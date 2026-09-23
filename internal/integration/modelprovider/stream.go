package modelprovider

import (
	"encoding/json"
	"errors"
	"fmt"
)

var ErrIncompleteStream = errors.New("model stream ended without a terminal response")

type streamState struct {
	calls     map[string]Event
	order     []string
	completed bool
}

func newStreamState() *streamState { return &streamState{calls: make(map[string]Event)} }

func (provider Provider) decodeEvent(raw []byte, sink func(Event) error) error {
	return newStreamState().decode(provider.Protocol, raw, sink)
}

func (state *streamState) decode(protocol Protocol, raw []byte, sink func(Event) error) error {
	if protocol == ProtocolResponses {
		return state.responses(raw, sink)
	}
	return state.chat(raw, sink)
}

func (state *streamState) remember(key string, value Event) error {
	if _, exists := state.calls[key]; !exists {
		if len(state.calls) >= 128 {
			return errors.New("model tool batch exceeds limit")
		}
		state.order = append(state.order, key)
	}
	if len(value.Arguments) > 1<<20 || len(value.ToolName) > 256 {
		return errors.New("model tool call exceeds size limit")
	}
	state.calls[key] = value
	return nil
}

func (state *streamState) finishCalls(sink func(Event) error) error {
	for index, key := range state.order {
		call := state.calls[key]
		if call.ToolCallID == "" || call.ToolName == "" || !json.Valid([]byte(call.Arguments)) {
			return errors.New("model returned an incomplete tool call")
		}
		call.Type, call.Index = "tool_call_done", index
		if err := sink(call); err != nil {
			return err
		}
	}
	clear(state.calls)
	state.order = nil
	return nil
}

func (state *streamState) chat(raw []byte, sink func(Event) error) error {
	var chunk struct {
		Error   json.RawMessage `json:"error"`
		Choices []struct {
			Index int `json:"index"`
			Delta struct {
				Content   string `json:"content"`
				Refusal   string `json:"refusal"`
				ToolCalls []struct {
					Index    int                              `json:"index"`
					ID       string                           `json:"id"`
					Function struct{ Name, Arguments string } `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     *int64 `json:"prompt_tokens"`
			CompletionTokens *int64 `json:"completion_tokens"`
			InputDetails     struct {
				Cached *int64 `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &chunk); err != nil {
		return err
	}
	if len(chunk.Error) != 0 && string(chunk.Error) != "null" {
		return errors.New("model provider reported an error")
	}
	if len(chunk.Choices) > 1 {
		return errors.New("multiple model choices are unsupported")
	}
	for _, choice := range chunk.Choices {
		if choice.Index != 0 {
			return errors.New("unexpected model choice index")
		}
		if state.completed && (choice.Delta.Content != "" || len(choice.Delta.ToolCalls) != 0) {
			return errors.New("model content after terminal response")
		}
		if choice.Delta.Refusal != "" {
			return errors.New("model refused the request")
		}
		if choice.Delta.Content != "" {
			if err := sink(Event{Type: "text_delta", Text: choice.Delta.Content}); err != nil {
				return err
			}
		}
		for _, part := range choice.Delta.ToolCalls {
			if part.Index < 0 {
				return errors.New("negative model tool index")
			}
			key := fmt.Sprint(part.Index)
			call := state.calls[key]
			if part.ID != "" {
				if call.ToolCallID != "" && call.ToolCallID != part.ID {
					return errors.New("model tool identity changed")
				}
				call.ToolCallID = part.ID
			}
			call.ToolName += part.Function.Name
			call.Arguments += part.Function.Arguments
			if err := state.remember(key, call); err != nil {
				return err
			}
		}
		if choice.FinishReason != "" {
			if state.completed {
				return errors.New("duplicate model terminal response")
			}
			if choice.FinishReason == "tool_calls" {
				if err := state.finishCalls(sink); err != nil {
					return err
				}
			} else if choice.FinishReason == "stop" && len(state.calls) > 0 {
				return errors.New("model tool calls lack tool_calls finish reason")
			}
			state.completed = true
			if err := sink(Event{Type: "completed", StopReason: choice.FinishReason}); err != nil {
				return err
			}
		}
	}
	if chunk.Usage.PromptTokens != nil || chunk.Usage.CompletionTokens != nil || chunk.Usage.InputDetails.Cached != nil {
		return emitUsage(sink, "usage", "", chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens, chunk.Usage.InputDetails.Cached)
	}
	return nil
}

type responseItem struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (state *streamState) responses(raw []byte, sink func(Event) error) error {
	var event struct {
		Type      string       `json:"type"`
		Delta     string       `json:"delta"`
		ItemID    string       `json:"item_id"`
		CallID    string       `json:"call_id"`
		Name      string       `json:"name"`
		Arguments string       `json:"arguments"`
		Item      responseItem `json:"item"`
		Response  struct {
			Status string         `json:"status"`
			Output []responseItem `json:"output"`
			Usage  struct {
				Input        *int64 `json:"input_tokens"`
				Output       *int64 `json:"output_tokens"`
				InputDetails struct {
					Cached *int64 `json:"cached_tokens"`
				} `json:"input_tokens_details"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return err
	}
	if state.completed {
		return errors.New("model event after terminal response")
	}
	switch event.Type {
	case "response.created", "response.in_progress", "response.queued", "response.output_text.done", "response.content_part.added", "response.content_part.done":
		return nil
	case "response.output_text.delta":
		return sink(Event{Type: "text_delta", Text: event.Delta})
	case "response.output_item.added", "response.output_item.done":
		switch event.Item.Type {
		case "function_call":
			key := event.Item.ID
			if key == "" {
				key = event.Item.CallID
			}
			call := state.calls[key]
			call.ToolCallID, call.ToolName = event.Item.CallID, event.Item.Name
			if event.Item.Arguments != "" {
				call.Arguments = event.Item.Arguments
			}
			return state.remember(key, call)
		case "message":
			return nil
		default:
			return errors.New("unsupported model response output item")
		}
	case "response.function_call_arguments.delta", "response.function_call_arguments.done":
		key := event.ItemID
		if key == "" {
			key = event.CallID
		}
		if key == "" {
			return errors.New("model tool delta lacks identity")
		}
		call := state.calls[key]
		if event.CallID != "" {
			call.ToolCallID = event.CallID
		}
		if event.Name != "" {
			call.ToolName = event.Name
		}
		if event.Type == "response.function_call_arguments.done" {
			call.Arguments = event.Arguments
		} else {
			call.Arguments += event.Delta
		}
		return state.remember(key, call)
	case "response.completed":
		if event.Response.Status != "completed" {
			return errors.New("invalid completed model status")
		}
		// Completed responses may include the authoritative calls even when a
		// provider omits intermediate item events. Their order is preserved.
		for _, item := range event.Response.Output {
			if item.Type == "function_call" {
				key := item.ID
				if key == "" {
					key = item.CallID
				}
				if err := state.remember(key, Event{ToolCallID: item.CallID, ToolName: item.Name, Arguments: item.Arguments}); err != nil {
					return err
				}
			} else if item.Type != "message" {
				return errors.New("unsupported model response output item")
			}
		}
		if err := state.finishCalls(sink); err != nil {
			return err
		}
		state.completed = true
		return emitUsage(sink, "completed", "completed", event.Response.Usage.Input, event.Response.Usage.Output, event.Response.Usage.InputDetails.Cached)
	case "response.incomplete":
		state.completed = true
		return emitUsage(sink, "completed", "incomplete", event.Response.Usage.Input, event.Response.Usage.Output, event.Response.Usage.InputDetails.Cached)
	case "response.failed", "error", "response.refusal.delta", "response.refusal.done":
		return errors.New("model response failed or was refused")
	default:
		return errors.New("unsupported model response event")
	}
}
