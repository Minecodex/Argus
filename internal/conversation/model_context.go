package conversation

import (
	"time"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// WithModelRuntimeFacts adds the same fixed facts at message admission and in
// the worker. It copies the map so later context assembly cannot mutate a
// caller's authorization or file facts.
func WithModelRuntimeFacts(facts map[string]any, mode string, snapshot toolruntime.Snapshot, tools []modelprovider.Tool, now time.Time) map[string]any {
	result := make(map[string]any, len(facts)+3)
	for key, value := range facts {
		result[key] = value
	}
	result["current_time_utc"] = now.UTC().Format(time.RFC3339)
	result["mode"] = mode
	names := []string{}
	for _, tool := range tools {
		switch tool.Name {
		case "read", "write", "edit", "bash", "connect":
			names = append(names, tool.Name)
		}
	}
	result["builtin_tool_availability"] = map[string]any{"workspace_status_at_run_start": snapshot.SandboxStatus, "available": names}
	return result
}
