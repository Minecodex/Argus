package presentation

import (
	"github.com/kakj-go/Argus/internal/toolruntime"
	"testing"
)

func TestDisplayClassificationDoesNotGrantMutationOrGuessNames(t *testing.T) {
	for tool, mode := range map[string]string{"tool.search": "trace", "telemetry.dashboard.draft.validate": "trace", "pending_action.get": "action", "host.get": "template", "host.update.preview": "action", "workflow.publish_file": "file"} {
		got := ToolDisplayFor(tool, toolruntime.Result{Data: map[string]any{"id": "unknown-id"}})
		if got.Mode != mode || len(got.Facts) != 0 || got.ActionRef != "" {
			if tool != "telemetry.dashboard.draft.validate" {
				t.Fatal(tool, got)
			}
		}
	}
	got := ToolDisplayFor("pending_action.get", toolruntime.Result{Data: map[string]any{"action_ref": "existing", "name": "db-prod", "password": "private"}})
	if got.ActionRef != "existing" || len(got.Facts) != 1 || got.Facts[0].Value != "db-prod" {
		t.Fatal(got)
	}
	validation := ToolDisplayFor("telemetry.dashboard.draft.validate", toolruntime.Result{Data: map[string]any{"validation": map[string]any{"issues": []any{map[string]any{"path": "panels[0].targets", "message": "missing", "severity": "error"}}}}})
	if len(validation.Issues) != 1 || validation.Issues[0]["path"] != "panels[0].targets" {
		t.Fatal(validation)
	}
}
