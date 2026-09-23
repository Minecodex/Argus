//go:build m4e2e

package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
)

func TestReplayPlanRoundTripsNativeProviderMessages(t *testing.T) {
	for _, protocol := range []modelprovider.Protocol{modelprovider.ProtocolChatCompletions, modelprovider.ProtocolResponses} {
		t.Run(string(protocol), func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/v1/chat/completions", chatCompletions)
			mux.HandleFunc("/v1/responses", responses)
			server := httptest.NewServer(mux)
			defer server.Close()
			provider := modelprovider.Provider{Protocol: protocol, BaseURL: server.URL + "/v1", Client: server.Client()}
			plan := []replayStep{{Tool: "tool.search", Arguments: map[string]any{"category": "host"}}, {Tool: "tool.describe", Arguments: map[string]any{"category": "host", "name": "list"}}, {Tool: "tool.invoke", Arguments: map[string]any{"category": "host", "name": "list", "arguments": map[string]any{}}}}
			data, _ := json.Marshal(plan)
			messages := []modelprovider.Message{{Role: "user", Content: "Execute argus_e2e_plan_b64:" + base64.RawURLEncoding.EncodeToString(data)}}
			var tools []modelprovider.Tool
			for _, step := range plan {
				tools = append(tools, modelprovider.Tool{Name: step.Tool, Schema: map[string]any{"type": "object"}})
			}
			for index := 0; index <= len(plan); index++ {
				var call modelprovider.ToolCall
				completed := false
				err := provider.Stream(t.Context(), modelprovider.Request{Model: "replay", Messages: messages, Tools: tools, MaxTokens: 1024}, func(event modelprovider.Event) error {
					if event.Type == "tool_call_done" {
						if event.ToolCallID != "" {
							call.ID = event.ToolCallID
						}
						if event.ToolName != "" {
							call.Name = event.ToolName
						}
						call.Arguments += event.Arguments
					}
					if event.Type == "completed" {
						completed = true
					}
					return nil
				})
				if err != nil || !completed {
					t.Fatalf("provider turn failed: %v", err)
				}
				if index == len(plan) {
					if call.ID != "" {
						t.Fatal("replay repeated a completed plan")
					}
					break
				}
				if call.Name != plan[index].Tool || call.ID == "" || !json.Valid([]byte(call.Arguments)) {
					t.Fatalf("native turn %d returned invalid call: %+v", index, call)
				}
				messages = append(messages, modelprovider.Message{Role: "assistant", ToolCalls: []modelprovider.ToolCall{call}}, modelprovider.Message{Role: "tool", ToolCallID: call.ID, Content: `{"summary":"completed"}`})
			}
		})
	}
}
