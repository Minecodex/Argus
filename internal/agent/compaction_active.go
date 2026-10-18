package agent

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// A long-running user turn can outgrow the hard budget even after older turns
// have been summarized. Only complete native tool groups may be moved, and the
// current Run's original user messages are reloaded separately from the ledger.
func (compactor Compactor) activeTurnBoundary(ctx context.Context, run db.Run, events []db.ConversationEvent) (int64, int64, bool, error) {
	actions, err := compactor.Store.Queries.ListRunPendingActions(ctx, db.ListRunPendingActionsParams{RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, EnterpriseID: run.EnterpriseID, CreatorSubjectID: run.ActorUserID})
	if err != nil {
		return 0, 0, false, err
	}
	protected := map[string]bool{}
	for _, action := range actions {
		switch action.Status {
		case "prepared", "awaiting_confirmation", "awaiting_approval", "ready", "executing", "result_unknown":
			protected[action.ActionRef] = true
		}
	}
	through, kept, ok := chooseActiveTurnBoundary(events, protected)
	return through, kept, ok, nil
}

func chooseActiveTurnBoundary(events []db.ConversationEvent, protected map[string]bool) (through, firstKept int64, ok bool) {
	if len(events) < 3 {
		return 0, 0, false
	}
	total := 0
	for _, event := range events {
		total += max(len(event.Payload), 1)
	}
	pending := map[string]bool{}
	consumed := 0
	for index, event := range events {
		consumed += max(len(event.Payload), 1)
		var payload struct {
			Calls     []modelprovider.ToolCall `json:"tool_calls"`
			ID        string                   `json:"tool_call_id"`
			ActionRef string                   `json:"action_ref"`
		}
		if len(event.Payload) > 0 && json.Unmarshal(event.Payload, &payload) != nil {
			return 0, 0, false
		}
		if event.EventType == "pending_action_created" && protected[payload.ActionRef] {
			break
		}
		if event.EventType == "assistant_message" {
			for _, call := range payload.Calls {
				if call.ID == "" || pending[call.ID] {
					return 0, 0, false
				}
				pending[call.ID] = true
			}
		}
		if event.EventType == "tool_call_result" {
			if payload.ID == "" || !pending[payload.ID] {
				return 0, 0, false
			}
			delete(pending, payload.ID)
		}
		if index+1 >= len(events) || len(pending) != 0 {
			continue
		}
		complete := event.EventType == "tool_call_result" || event.EventType == "model_usage" || event.EventType == "assistant_message" && len(payload.Calls) == 0
		if complete {
			through, firstKept, ok = event.Sequence, events[index+1].Sequence, true
			if consumed >= total/2 {
				return through, firstKept, true
			}
		}
	}
	return
}
