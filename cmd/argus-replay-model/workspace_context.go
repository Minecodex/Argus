//go:build m4e2e

package main

import (
	"encoding/json"
	"strings"
)

// Return only public facts from the actual provider request, never prompts or
// private runtime data. This fixture is not part of a production model server.
func workspaceContextEcho(request replayRequest) (string, bool) {
	messages := append(request.Messages, request.Input...)
	lastUser := ""
	for _, m := range messages {
		if m.Role == "user" {
			lastUser = replayContentText(m.Content)
		}
	}
	if !strings.Contains(lastUser, "argus_e2e_workspace_context") {
		return "", false
	}
	var workspace struct {
		ID     string `json:"id,omitempty"`
		Status string `json:"status"`
	}
	workspace.Status = "missing"
	available, unavailable := 0, 0
	for _, m := range messages {
		text := replayContentText(m.Content)
		if raw, ok := strings.CutPrefix(text, "Server execution facts (data): "); ok {
			var value struct {
				Workspace json.RawMessage `json:"workspace"`
			}
			if json.Unmarshal([]byte(raw), &value) == nil && len(value.Workspace) > 0 {
				_ = json.Unmarshal(value.Workspace, &workspace)
			}
		}
		if _, raw, ok := strings.Cut(text, "\nWorkspace attachments: "); ok {
			var refs []struct {
				Available *bool `json:"available"`
			}
			if json.Unmarshal([]byte(raw), &refs) == nil {
				for _, ref := range refs {
					if ref.Available != nil {
						if *ref.Available {
							available++
						} else {
							unavailable++
						}
					}
				}
			}
		}
	}
	data, _ := json.Marshal(map[string]any{"workspace": workspace, "available_references": available, "unavailable_references": unavailable})
	return string(data), true
}
