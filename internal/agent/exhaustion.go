package agent

import (
	"context"
	"encoding/json"

	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (loop Loop) HandleExhausted(ctx context.Context, task runtime.Task, cause error) error {
	var payload conversation.AgentTask
	if json.Unmarshal(task.Payload, &payload) != nil || !task.RunID.Valid {
		return nil
	}
	ctx, cancel := withTaskLease(ctx, loop.Store, task)
	defer cancel()
	if err := assertTaskLease(ctx); err != nil {
		return err
	}
	run, err := loop.Store.Queries.GetRun(ctx, db.GetRunParams{ID: task.RunID.UUID, EnterpriseID: payload.EnterpriseID})
	if err != nil {
		return err
	}
	if !terminalRun(run.Status) {
		if err := fencedTransaction(ctx, loop.Store, func(q *db.Queries) error {
			if err := q.ReconcileInterruptedModelCalls(ctx, db.ReconcileInterruptedModelCallsParams{RunID: run.ID, EnterpriseID: run.EnterpriseID, CallKind: "inference"}); err != nil {
				return err
			}
			if run.CurrentStepID.Valid {
				// A failed provider attempt already closes its step. Exhaustion
				// must still close the Run without rewriting terminal step facts.
				return q.FailActiveExhaustedRunStep(ctx, db.FailActiveExhaustedRunStepParams{ID: run.CurrentStepID.UUID, EnterpriseID: run.EnterpriseID, RunID: run.ID})
			}
			return nil
		}); err != nil {
			return err
		}
		if err := loop.finishRun(ctx, run, "failed", "task_exhausted", toolErrorCode(cause)); err != nil {
			return err
		}
	}
	return loop.reconcileTerminalToolCalls(ctx, run)
}

func (compactor Compactor) HandleExhausted(ctx context.Context, task runtime.Task, _ error) error {
	var payload conversation.AgentTask
	if json.Unmarshal(task.Payload, &payload) != nil || !requiresCompactionResume(payload.Reason) {
		return nil
	}
	ctx, cancel := withTaskLease(ctx, compactor.Store, task)
	defer cancel()
	if err := assertTaskLease(ctx); err != nil {
		return err
	}
	run, err := compactor.Store.Queries.GetRun(ctx, db.GetRunParams{ID: payload.RunID, EnterpriseID: payload.EnterpriseID})
	if err != nil {
		return err
	}
	if terminalRun(run.Status) {
		return nil
	}
	return compactor.fail(ctx, run, "CONTEXT_COMPACTION_FAILED")
}
