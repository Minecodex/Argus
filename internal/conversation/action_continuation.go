package conversation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Called in the transaction changing the action. A running task cannot replace
// a pending continuation: it may already have read the previous action state.
func ScheduleActionRun(ctx context.Context, q *db.Queries, action db.PendingAction, verification bool) error {
	if !action.RunID.Valid || action.CreatorSubjectType != "user" {
		return nil
	}
	run, err := q.GetRunForUpdate(ctx, db.GetRunForUpdateParams{ID: action.RunID.UUID, EnterpriseID: action.EnterpriseID})
	if err != nil {
		return err
	}
	if run.ActorUserID != action.CreatorSubjectID || TerminalRun(run.Status) {
		return nil
	}
	if verification && !run.VerificationOnly {
		if err := q.MarkRunVerificationOnly(ctx, db.MarkRunVerificationOnlyParams{ID: run.ID, EnterpriseID: run.EnterpriseID}); err != nil {
			return err
		}
	}
	pending, err := q.HasPendingAgentTask(ctx, db.HasPendingAgentTaskParams{RunID: action.RunID, EnterpriseID: uuid.NullUUID{UUID: action.EnterpriseID, Valid: true}})
	if err != nil || pending {
		return err
	}
	payload, _ := json.Marshal(AgentTask{RunID: run.ID, EnterpriseID: run.EnterpriseID, Reason: "action_state_changed"})
	_, err = q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: newID(), EnterpriseID: uuid.NullUUID{UUID: run.EnterpriseID, Valid: true}, Queue: "agent", RunID: action.RunID, Payload: payload, MaxAttempts: 5, AvailableAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}})
	return err
}
