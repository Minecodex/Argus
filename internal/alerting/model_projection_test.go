package alerting

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/mcp"
)

func TestTimelineProjectionPreservesFactsAndFullUIWithoutRepeatedSnapshots(t *testing.T) {
	snapshot := map[string]any{"rules": []any{map[string]any{"expression": strings.Repeat("metric ", 2000)}}}
	events := []any{}
	for i := range 8 {
		events = append(events, map[string]any{"id": i, "event_type": "muted", "actor_id": "actor", "payload": map[string]any{"id": "instance", "policy_id": "policy", "policy_version": 3, "resource_ids": []any{"host"}, "source_ids": []any{"source"}, "state": map[string]any{"value": 12.5, "noise_reasons": []any{"silence:v1"}, "policy_snapshot": snapshot, "source_snapshot": []any{"source"}}}})
	}
	events = append(events, map[string]any{"event_type": "comment", "payload": map[string]any{"comment": "kept"}})
	full := map[string]any{"instance": map[string]any{"state": map[string]any{"policy_snapshot": snapshot}}, "events": events}
	before, _ := json.Marshal(full)
	view := projectInstanceTimeline(mcp.Result{Structured: full})
	encoded, _ := json.Marshal(view)
	after, _ := json.Marshal(full)
	if string(before) != string(after) || len(encoded) >= len(before)/2 {
		t.Fatal("projection mutated full data or retained duplicated snapshots")
	}
	if view["instance"].(map[string]any)["state"].(map[string]any)["policy_snapshot"] == nil {
		t.Fatal("firing snapshot omitted")
	}
	first := view["events"].([]any)[0].(map[string]any)
	state := first["payload"].(map[string]any)["state"].(map[string]any)
	if first["actor_id"] != "actor" || state["value"] != float64(12.5) || state["snapshot_ref"] == nil || state["noise_reasons"] == nil || state["policy_snapshot"] != nil {
		t.Fatal("timeline facts or immutable snapshot reference lost")
	}
	last := view["events"].([]any)[8].(map[string]any)
	if last["payload"].(map[string]any)["comment"] != "kept" {
		t.Fatal("manual handling payload omitted")
	}
}
