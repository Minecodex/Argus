package argusdev

import (
	"encoding/json"
	"testing"
)

func TestPlan6EmptyTargetRequiresCompleteAuthorizedPage(t *testing.T) {
	page := map[string]any{"resource_ids": []any{"cluster"}, "complete": true, "total": 1, "next_cursor": nil, "evaluations": []any{map[string]any{"policy_id": "target", "health": "normal", "last_evaluated_at": "2026-10-11T00:00:00Z"}}, "items": []any{map[string]any{"policy_id": "unrelated"}}}
	check := func() error {
		raw, _ := json.Marshal([]any{map[string]any{"Tool": "tool.invoke", "Status": "succeeded", "Output": map[string]any{"page": page}}})
		return verifyPlan6StateProjection(string(raw), "target", "normal", false, true, "cluster")
	}
	if err := check(); err != nil {
		t.Fatal("unrelated events invalidate the zero-event target:", err)
	}
	page["complete"] = false
	if check() == nil {
		t.Fatal("partial page falsely proved target absence")
	}
	page["complete"] = true
	page["next_cursor"] = "more"
	if check() == nil {
		t.Fatal("unread page falsely proved target absence")
	}
	page["next_cursor"] = nil
	page["items"] = []any{map[string]any{"policy_id": "target"}}
	if check() == nil {
		t.Fatal("target event falsely reported empty")
	}
	page["items"] = []any{}
	page["resource_ids"] = []any{"other"}
	if check() == nil {
		t.Fatal("different authorized scope accepted")
	}
}
