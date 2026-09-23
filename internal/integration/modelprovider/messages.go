package modelprovider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// Message is the provider-neutral, durable representation of a model turn.
// Tool calls and results remain protocol data, never simulated user text.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

var functionName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// WireToolName preserves logical identities while satisfying both function APIs.
// External connections receive a namespaced logical identity before this layer.
func WireToolName(name string) string {
	switch name {
	case "tool.search":
		return "tool_search"
	case "tool.describe":
		return "tool_describe"
	case "tool.invoke":
		return "tool_invoke"
	}
	if functionName.MatchString(name) {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	return "argus_" + hex.EncodeToString(sum[:24])
}

func toolAliases(tools []Tool) (map[string]string, error) {
	aliases := make(map[string]string, len(tools))
	for _, tool := range tools {
		if tool.Name == "" || tool.Schema == nil {
			return nil, errors.New("model tool name and input schema are required")
		}
		wire := WireToolName(tool.Name)
		if _, exists := aliases[wire]; exists {
			return nil, fmt.Errorf("duplicate model tool identity: %s", wire)
		}
		aliases[wire] = tool.Name
	}
	return aliases, nil
}

func encodeMessages(protocol Protocol, messages []Message) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(messages))
	pending := map[string]bool{}
	for _, message := range messages {
		if message.Role != "system" && message.Role != "user" && message.Role != "assistant" && message.Role != "tool" {
			return nil, errors.New("unsupported model message role")
		}
		if message.Role == "tool" {
			if message.ToolCallID == "" || !pending[message.ToolCallID] || len(message.ToolCalls) != 0 {
				return nil, errors.New("unpaired model tool result")
			}
			delete(pending, message.ToolCallID)
			if protocol == ProtocolResponses {
				result = append(result, map[string]any{"type": "function_call_output", "call_id": message.ToolCallID, "output": message.Content})
			} else {
				result = append(result, map[string]any{"role": "tool", "tool_call_id": message.ToolCallID, "content": message.Content})
			}
			continue
		}
		if len(pending) != 0 {
			return nil, errors.New("model tool batch lacks results")
		}
		if len(message.ToolCalls) != 0 && message.Role != "assistant" {
			return nil, errors.New("only assistant messages may contain tool calls")
		}
		var calls []map[string]any
		for _, call := range message.ToolCalls {
			if call.ID == "" || call.Name == "" || !json.Valid([]byte(call.Arguments)) || pending[call.ID] {
				return nil, errors.New("invalid model tool call")
			}
			pending[call.ID] = true
			calls = append(calls, map[string]any{"id": call.ID, "type": "function", "function": map[string]any{
				"name": WireToolName(call.Name), "arguments": call.Arguments,
			}})
		}
		if protocol == ProtocolResponses {
			if message.Content != "" || len(calls) == 0 {
				contentType := "input_text"
				if message.Role == "assistant" {
					contentType = "output_text"
				}
				result = append(result, map[string]any{"type": "message", "role": message.Role,
					"content": []map[string]any{{"type": contentType, "text": message.Content}}})
			}
			for _, call := range message.ToolCalls {
				result = append(result, map[string]any{"type": "function_call", "call_id": call.ID,
					"name": WireToolName(call.Name), "arguments": call.Arguments})
			}
		} else {
			item := map[string]any{"role": message.Role, "content": message.Content}
			if len(calls) != 0 {
				item["tool_calls"] = calls
			}
			result = append(result, item)
		}
	}
	if len(pending) != 0 {
		return nil, errors.New("model input ends with an incomplete tool batch")
	}
	return result, nil
}
