package agent

import (
	"encoding/json"
	"testing"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestActiveTurnCompactionKeepsCompleteToolGroups(t *testing.T) {
	event := func(sequence int64, kind string, payload any) db.ConversationEvent {
		raw, _ := json.Marshal(payload)
		return db.ConversationEvent{Sequence: sequence, EventType: kind, Payload: raw}
	}
	events := []db.ConversationEvent{
		event(1, "user_message", map[string]any{"content": "repair only the approved test service"}),
		event(2, "assistant_message", map[string]any{"tool_calls": []map[string]any{{"id": "call-a", "name": "read", "arguments": `{}`}, {"id": "call-b", "name": "read", "arguments": `{}`}}}),
		event(3, "tool_call_result", map[string]any{"tool_call_id": "call-a", "projection": "first complete result"}),
		event(4, "tool_call_result", map[string]any{"tool_call_id": "call-b", "projection": "second complete result"}),
		event(5, "assistant_message", map[string]any{"tool_calls": []map[string]any{{"id": "call-c", "name": "bash", "arguments": `{}`}}}),
		event(6, "tool_call_result", map[string]any{"tool_call_id": "call-c", "projection": "keep the recent pair"}),
	}
	if _, _, ok := ChooseCompactionBoundary(events); ok {
		t.Fatal("soft compaction must still preserve the current turn")
	}
	through, kept, ok := chooseActiveTurnBoundary(events, nil)
	if !ok || through != 4 || kept != 5 {
		t.Fatalf("complete tool group boundary = %d/%d, %v", through, kept, ok)
	}
	for name, incomplete := range map[string][]db.ConversationEvent{
		"partial_parallel_batch": events[:3],
		"user_only":              events[:1],
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, ok := chooseActiveTurnBoundary(incomplete, nil); ok {
				t.Fatal("an incomplete group was compacted")
			}
		})
	}
	protected := []db.ConversationEvent{events[0], event(2, "pending_action_created", map[string]any{"action_ref": "act-live"})}
	for _, item := range events[1:] {
		item.Sequence++
		protected = append(protected, item)
	}
	if _, _, ok := chooseActiveTurnBoundary(protected, map[string]bool{"act-live": true}); ok {
		t.Fatal("active approval was moved into a narrative summary")
	}
}
