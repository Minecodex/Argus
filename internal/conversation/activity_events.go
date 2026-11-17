package conversation

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Before/after snapshots may contain complete file contents. Both ledger reads
// and SSE use this path, and must remove them if their authorization has changed.
func (service Service) scopeActivityEvents(ctx context.Context, enterprise, user uuid.UUID, events []db.ConversationEvent) ([]db.ConversationEvent, error) {
	current := ""
	for i := range events {
		if events[i].EventType != "tool_call_result" {
			continue
		}
		var payload map[string]json.RawMessage
		if json.Unmarshal(events[i].Payload, &payload) != nil || len(payload["activity"]) == 0 || string(payload["activity"]) == "null" {
			continue
		}
		if current == "" {
			var err error
			current, err = presentation.Scope(ctx, service.Store, enterprise, user)
			if err != nil {
				return nil, err
			}
		}
		var scope string
		_ = json.Unmarshal(payload["authorization_scope"], &scope)
		if scope != current {
			delete(payload, "activity")
			encoded, err := json.Marshal(payload)
			if err != nil {
				return nil, err
			}
			events[i].Payload = encoded
		}
	}
	return events, nil
}
