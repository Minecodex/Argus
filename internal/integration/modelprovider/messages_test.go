package modelprovider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNativeToolMessagesForBothProtocols(t *testing.T) {
	for _, protocol := range []Protocol{ProtocolChatCompletions, ProtocolResponses} {
		t.Run(string(protocol), func(t *testing.T) {
			request := Request{Model: "test", MaxTokens: 128,
				Tools: []Tool{{Name: "tool.invoke", Schema: map[string]any{"type": "object"}}},
				Messages: []Message{
					{Role: "system", Content: "system"}, {Role: "user", Content: "query"},
					{Role: "assistant", ToolCalls: []ToolCall{{ID: "z", Name: "tool.invoke", Arguments: `{"name":"list"}`}, {ID: "a", Name: "tool.invoke", Arguments: `{"name":"get"}`}}},
					{Role: "tool", ToolCallID: "z", Content: `{"items":[]}`}, {Role: "tool", ToolCallID: "a", Content: `{"ok":true}`},
				},
			}
			_, body, err := (Provider{Protocol: protocol, BaseURL: "https://example.test/v1"}).buildRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(body, &value); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "Tool call:") || strings.Contains(string(body), "Tool result:") {
				t.Fatal("tools were converted to text")
			}
			tools := value["tools"].([]any)
			if protocol == ProtocolChatCompletions {
				messages := value["messages"].([]any)
				calls := messages[2].(map[string]any)["tool_calls"].([]any)
				if calls[0].(map[string]any)["id"] != "z" || messages[3].(map[string]any)["tool_call_id"] != "z" {
					t.Fatal("tool order or pairing changed")
				}
				if tools[0].(map[string]any)["function"].(map[string]any)["name"] != "tool_invoke" {
					t.Fatal("invalid function name")
				}
			} else {
				input := value["input"].([]any)
				if input[2].(map[string]any)["type"] != "function_call" || input[4].(map[string]any)["type"] != "function_call_output" {
					t.Fatal("invalid Responses items")
				}
				if tools[0].(map[string]any)["type"] != "function" || tools[0].(map[string]any)["name"] != "tool_invoke" {
					t.Fatal("invalid Responses schema")
				}
			}
		})
	}
}

func TestMalformedToolHistoryRejectedBeforeRequest(t *testing.T) {
	for _, messages := range [][]Message{
		{{Role: "tool", ToolCallID: "missing", Content: "{}"}},
		{{Role: "assistant", ToolCalls: []ToolCall{{ID: "x", Name: "tool.invoke", Arguments: "{}"}}}},
		{{Role: "user", ToolCalls: []ToolCall{{ID: "x", Name: "tool.invoke", Arguments: "{}"}}}},
		{{Role: "assistant", ToolCalls: []ToolCall{{ID: "x", Name: "tool.invoke", Arguments: "{"}}}, {Role: "tool", ToolCallID: "x", Content: "{}"}},
	} {
		if _, err := encodeMessages(ProtocolChatCompletions, messages); err == nil {
			t.Fatalf("malformed history accepted: %#v", messages)
		}
	}
}

func TestResponsesToolCallAssemblyUsesItemIdentity(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"item-z","call_id":"call-z","name":"tool_invoke","arguments":""}}`, "",
		`data: {"type":"response.function_call_arguments.delta","item_id":"item-z","delta":"{\"name\":"}`, "",
		`data: {"type":"response.function_call_arguments.delta","item_id":"item-z","delta":"\"list\"}"}`, "",
		`data: {"type":"response.function_call_arguments.done","item_id":"item-z","arguments":"{\"name\":\"list\"}"}`, "",
		`data: {"type":"response.completed","response":{"status":"completed"}}`, "",
	}, "\n")
	var calls []Event
	err := (Provider{Protocol: ProtocolResponses}).consumeSSE(strings.NewReader(stream), func(event Event) error {
		if event.Type == "tool_call_done" {
			calls = append(calls, event)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].ToolCallID != "call-z" || calls[0].Arguments != `{"name":"list"}` {
		t.Fatalf("bad calls: %#v", calls)
	}
}

func TestInterruptedAndUnsupportedResponsesFailClosed(t *testing.T) {
	for _, stream := range []string{
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n",
		"data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"computer_call\"}}\n\n",
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"incomplete\"}}\n\n",
	} {
		if err := (Provider{Protocol: ProtocolResponses}).consumeSSE(strings.NewReader(stream), func(Event) error { return nil }); err == nil {
			t.Fatal("incomplete/unsupported stream accepted")
		}
	}
}
