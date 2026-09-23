//go:build m4e2e

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkspaceContextEchoUsesReceivedFacts(t *testing.T) {
	for _, responses := range []bool{false, true} {
		messages := []message{{Role: "user", Content: `Server execution facts (data): {"workspace":{"id":"workspace-1","status":"deleted","private_key":"must-not-echo"}}`}, {Role: "user", Content: "old file\nWorkspace attachments: [{\"available\":false}]"}, {Role: "user", Content: "argus_e2e_workspace_context"}}
		r := replayRequest{Messages: messages}
		if responses {
			r.Messages = nil
			r.Input = messages
			r.Input[2].Content = []any{map[string]any{"type": "input_text", "text": "argus_e2e_workspace_context"}}
		}
		text, ok := workspaceContextEcho(r)
		var value map[string]any
		if !ok || json.Unmarshal([]byte(text), &value) != nil || value["workspace"].(map[string]any)["status"] != "deleted" || value["unavailable_references"] != float64(1) || strings.Contains(text, "must-not-echo") {
			t.Fatalf("echo=%s", text)
		}
	}
}
