package agent

import (
	"context"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// A preview can commit before its ToolResult or before the rest of a model
// batch. Recovery must finish that batch without introducing an external write
// while the human confirmation is pending. Native confirmation previews keep
// using the existing verification policy; they never execute their changes.
func (loop Loop) actionBatchReadOnly(ctx context.Context, run db.Run) (bool, error) {
	actions, err := loop.Store.Queries.ListRunPendingActions(ctx, db.ListRunPendingActionsParams{RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, EnterpriseID: run.EnterpriseID, CreatorSubjectID: run.ActorUserID})
	if err != nil {
		return false, err
	}
	for _, action := range actions {
		if action.ErrorCode.String != "ACTION_SUPERSEDED" {
			return true, nil
		}
	}
	return false, nil
}

func (loop Loop) finishToolBatchSteps(ctx context.Context, run db.Run, calls []db.ToolCall, unknown bool) error {
	return fencedTransaction(ctx, loop.Store, func(q *db.Queries) error {
		seen := map[uuid.UUID]bool{}
		status := "succeeded"
		if unknown {
			status = "failed"
		}
		for _, call := range calls {
			if seen[call.StepID] {
				continue
			}
			seen[call.StepID] = true
			if _, err := q.FinishRunStep(ctx, db.FinishRunStepParams{ID: call.StepID, EnterpriseID: run.EnterpriseID, Status: status}); err != nil {
				return err
			}
		}
		return nil
	})
}
