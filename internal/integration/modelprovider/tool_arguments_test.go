package modelprovider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMalformedToolArgumentsRetainUsageAndCorrectionHistory(t *testing.T) {
	for _, protocol := range []Protocol{ProtocolChatCompletions, ProtocolResponses} {
		for _, args := range []string{`{"value":`, "", `[]`, `null`} {
			t.Run(string(protocol)+"/"+args, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					var frames []any
					if protocol == ProtocolChatCompletions {
						frames = []any{map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call-1", "function": map[string]any{"name": "inspect", "arguments": args}}}}, "finish_reason": "tool_calls"}}}, map[string]any{"choices": []any{}, "usage": map[string]any{"prompt_tokens": 12, "completion_tokens": 4}}}
					} else {
						frames = []any{map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": []any{map[string]any{"type": "function_call", "id": "item-1", "call_id": "call-1", "name": "inspect", "arguments": args}}, "usage": map[string]any{"input_tokens": 12, "output_tokens": 4}}}}
					}
					for _, frame := range frames {
						raw, _ := json.Marshal(frame)
						_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
					}
					_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
				}))
				defer server.Close()
				provider := Provider{Protocol: protocol, BaseURL: server.URL, Client: server.Client()}
				var usage TokenUsage
				var calls []Event
				if err := provider.Stream(t.Context(), Request{Model: "argument-validation", MaxTokens: 32}, func(e Event) error {
					usage.Observe(e)
					if e.Type == "tool_call_done" {
						calls = append(calls, e)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if !usage.Complete() || usage.Input != 12 || usage.Output != 4 || len(calls) != 1 || calls[0].Arguments != args {
					t.Fatal("transport changed arguments or lost provider usage")
				}
				messages := []Message{{Role: "assistant", ToolCalls: []ToolCall{{ID: "call-1", Name: "inspect", Arguments: args}}}, {Role: "tool", ToolCallID: "call-1", Content: `{"error_code":"TOOL_INPUT_INVALID"}`}}
				encoded, err := encodeMessages(protocol, messages)
				if err != nil || len(encoded) != 2 {
					t.Fatal("correction history cannot be sent", err)
				}
			})
		}
	}
}

func TestMissingToolIdentityRemainsProtocolFailure(t *testing.T) {
	for _, call := range []string{`{"index":0,"function":{"name":"inspect","arguments":"{}"}}`, `{"index":0,"id":"c","function":{"arguments":"{}"}}`} {
		stream := `data: {"choices":[{"delta":{"tool_calls":[` + call + `]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"
		if err := (Provider{Protocol: ProtocolChatCompletions}).consumeSSE(strings.NewReader(stream), func(Event) error { return nil }); err == nil {
			t.Fatal("missing identity accepted")
		}
	}
}
