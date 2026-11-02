//go:build m4e2e

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/integration/remotemcp"
)

func TestPlan11MCPBoundaryUsesRealProtocolPagesAndUntrustedData(t *testing.T) {
	for _, wire := range []string{"json", "sse"} {
		for _, tc := range []struct{ method, params, expected string }{
			{"tools/list", `{}`, "read_inventory"},
			{"tools/list", `{"cursor":"write"}`, "tool.invoke"},
			{"tools/call", `{"name":"read_inventory","arguments":{}}`, "plan11-customer-inventory-v1"},
		} {
			request := httptest.NewRequest("POST", "/mcp?scenario=plan11-origin-boundary&mode="+wire,
				strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"`+tc.method+`","params":`+tc.params+`}`))
			response := httptest.NewRecorder()
			remoteMCP(response, request)
			if response.Code != 200 || !strings.Contains(response.Body.String(), tc.expected) {
				t.Fatalf("actual protocol %s/%s omitted required boundary payload: %d", wire, tc.method, response.Code)
			}
			if wire == "json" && !json.Valid(response.Body.Bytes()) {
				t.Fatal("fixture returned malformed MCP JSON")
			}
		}
	}
}

func TestPlan11MCPBoundaryProductionClient(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(remoteMCP))
	defer server.Close()
	for _, wire := range []string{"json", "sse"} {
		client := remotemcp.Client{Endpoint: server.URL + "/mcp?auth=bearer&scenario=plan11-origin-boundary&mode=" + wire,
			HTTP: server.Client(), Authorization: "Bearer p5-token"}
		if err := client.Initialize(t.Context()); err != nil {
			t.Fatal(wire, err)
		}
		tools, hash, err := client.ListTools(t.Context())
		if err != nil || len(tools) != 2 || hash == "" || tools[0].Name != "read_inventory" || tools[1].Name != "tool.invoke" {
			t.Fatalf("%s: paged tools unavailable: %+v %v", wire, tools, err)
		}
		result, err := client.Call(t.Context(), "read_inventory", map[string]any{})
		data, ok := result["data"].(map[string]any)
		if err != nil || !ok || data["acceptance_marker"] != "plan11-customer-inventory-v1" || data["inventory_count"] != float64(7) || data["read_only_requested_tool"] != true {
			t.Fatalf("%s: actual client result lost structured evidence: %v", wire, err)
		}
		if result["display"] != nil || result["action_ref"] != nil || data["action_ref"] == nil {
			t.Fatal("untrusted display metadata was promoted out of customer data")
		}
		client.Close(t.Context())
	}
}
