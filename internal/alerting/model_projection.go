package alerting

import (
	"encoding/json"

	"github.com/kakj-go/Argus/internal/mcp"
)

// Keep the instance snapshot once and project each timeline event's observed
// state and actor. Complete per-event snapshots remain in the artifact/UI data.
func projectInstanceTimeline(result mcp.Result) map[string]any {
	raw, err := json.Marshal(result.Structured)
	if err != nil {
		return nil
	}
	var view map[string]any
	if json.Unmarshal(raw, &view) != nil {
		return nil
	}
	events, _ := view["events"].([]any)
	for _, rawEvent := range events {
		event, _ := rawEvent.(map[string]any)
		payload, _ := event["payload"].(map[string]any)
		state, _ := payload["state"].(map[string]any)
		if state == nil {
			continue // Manual comments/assignments retain their entire payload.
		}
		for _, key := range []string{"policy_snapshot", "source_snapshot", "latest_source_snapshot"} {
			delete(state, key)
		}
		state["snapshot_ref"] = map[string]any{"event_id": event["id"], "instance_id": payload["id"], "policy_id": payload["policy_id"], "policy_version": payload["policy_version"], "resource_ids": payload["resource_ids"], "source_ids": payload["source_ids"]}
	}
	view["projection_notes"] = "Instance retains its firing policy/source snapshot. Timeline preserves actors, handling, observed values, noise reasons, lifecycle, versions and scopes; complete per-event snapshots remain in the immutable result_ref and presentation."
	return view
}
