//go:build m4e2e

package main

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNaturalReplayUsesCurrentCategoryOnlyDiscovery(t *testing.T) {
	gateway, err := toolgateway.New(mcp.NewRegistry(), "replay-contract")
	if err != nil {
		t.Fatal(err)
	}
	var tools []modelprovider.Tool
	for _, tool := range gateway.CoreTools() {
		tools = append(tools, tool.Definition.Model)
	}
	schema, err := toolruntime.CompileInputSchema(tools[0].Schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []modelprovider.Protocol{modelprovider.ProtocolChatCompletions, modelprovider.ProtocolResponses} {
		t.Run(string(protocol), func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/v1/chat/completions", chatCompletions)
			mux.HandleFunc("/v1/responses", responses)
			server := httptest.NewServer(mux)
			defer server.Close()
			provider := modelprovider.Provider{Protocol: protocol, BaseURL: server.URL + "/v1", Client: server.Client()}
			var call modelprovider.ToolCall
			err := provider.Stream(t.Context(), modelprovider.Request{Model: "replay", Messages: []modelprovider.Message{{Role: "user", Content: "Call host.list and summarize the visible hosts."}}, Tools: tools, MaxTokens: 1024}, func(e modelprovider.Event) error {
				if e.Type == "tool_call_done" {
					call.Name = e.ToolName
					call.Arguments += e.Arguments
				}
				return nil
			})
			if err != nil || call.Name != "tool.search" {
				t.Fatal("discovery call missing", call.Name, err)
			}
			var input any
			if err = json.Unmarshal([]byte(call.Arguments), &input); err != nil {
				t.Fatal(err)
			}
			if err = schema.Validate(input); err != nil {
				t.Fatal("replay violates production metadata schema", err)
			}
		})
	}
}
