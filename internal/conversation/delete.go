package conversation

import (
	"context"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Delete permanently withdraws product access and schedules owned file cleanup.
// Confirmed execution/audit facts remain immutable governance records.
func (service Service) Delete(ctx context.Context, enterprise, owner, id uuid.UUID, key string) (db.Conversation, error) {
	return postgres.ExecuteIdempotent(ctx, service.Store, service.Idempotency, "enterprise", owner.String(), "conversation.delete", key, id, 202, func(q *db.Queries) (db.Conversation, error) {
		value, err := q.MarkConversationDeleted(ctx, db.MarkConversationDeletedParams{ID: id, EnterpriseID: enterprise, OwnerUserID: owner})
		if err != nil {
			return value, err
		}
		if err := q.MarkConversationWorkspacesDeleting(ctx, db.MarkConversationWorkspacesDeletingParams{ConversationID: id, EnterpriseID: enterprise}); err != nil {
			return value, err
		}
		runs, err := q.CancelConversationAgentRuns(ctx, db.CancelConversationAgentRunsParams{ConversationID: id, EnterpriseID: enterprise})
		if err != nil {
			return value, err
		}
		for _, run := range runs {
			if _, err := AppendEvent(ctx, q, EventInput{EnterpriseID: enterprise, ConversationID: id, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Type: "run_state_changed", ActorType: "user", ActorID: owner.String(), Payload: map[string]any{"status": "cancelled", "stop_reason": "conversation_deleted"}, Classification: "internal"}); err != nil {
				return value, err
			}
		}
		return value, nil
	})
}
