package agent

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/conversation"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

const unavailableContext = "Earlier output is unavailable under the current authorization."

type scopedCheckpoint struct {
	Workspace          *conversation.WorkspaceContext `json:"workspace,omitempty"`
	AuthorizationScope string                         `json:"authorization_scope"`
	Execution          json.RawMessage                `json:"execution_checkpoint"`
}

func snapshotInScope(snapshot db.ContextSnapshot, scope string) bool {
	var checkpoint scopedCheckpoint
	return scope != "" && json.Unmarshal(snapshot.TypedCheckpoint, &checkpoint) == nil && checkpoint.AuthorizationScope == scope
}

// Keep call IDs so native provider messages remain paired, while removing any
// output or arguments derived from data the current principal can no longer use.
func contextEventPayload(kind string, raw []byte, scope string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if kind != "assistant_message" && kind != "tool_call_result" || scope != "" && payload["authorization_scope"] == scope {
		return payload, nil
	}
	if kind == "tool_call_result" {
		return map[string]any{"tool_call_id": payload["tool_call_id"], "authorization_scope": scope, "projection": map[string]any{"error_code": "TOOL_RESULT_FORBIDDEN", "summary": unavailableContext}}, nil
	}
	result := map[string]any{"content": unavailableContext, "authorization_scope": scope}
	if calls, ok := payload["tool_calls"].([]any); ok {
		paired := make([]any, 0, len(calls))
		for _, value := range calls {
			if call, ok := value.(map[string]any); ok {
				paired = append(paired, map[string]any{"id": call["id"], "name": call["name"], "arguments": "{}"})
			}
		}
		result["tool_calls"] = paired
	}
	return result, nil
}
