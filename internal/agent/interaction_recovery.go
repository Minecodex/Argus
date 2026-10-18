package agent

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Recover a control barrier before leasing model credentials or making model
// calls. A committed plan may precede its ToolResult if the worker dies.
func (loop Loop) reconcileInteractions(ctx context.Context, run db.Run) (bool, error) {
	pending, err := loop.Store.Queries.GetPendingRunInteraction(ctx, db.GetPendingRunInteractionParams{RunID: run.ID, EnterpriseID: run.EnterpriseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if pending.Kind == "user_input" {
		return true, nil
	}
	calls, err := loop.Store.Queries.ListUnfinishedRunToolCalls(ctx, db.ListUnfinishedRunToolCallsParams{RunID: run.ID, EnterpriseID: run.EnterpriseID})
	if err != nil {
		return true, err
	}
	steps := map[uuid.UUID]bool{}
	for _, call := range calls {
		var result toolruntime.Result
		var outcome error
		if call.ID == pending.ToolCallID {
			result = toolruntime.Result{ToolID: "workflow.plan.propose", Data: map[string]any{"plan_id": pending.ID, "revision": pending.Revision, "status": "pending", "summary": "Waiting for human confirmation of this immutable plan document and its listed commands."}}
		} else {
			outcome = toolruntime.Error{Kind: "TOOL_CANCELLED"}
		}
		if _, err = loop.persistToolOutcome(ctx, run, call, result, outcome); err != nil {
			return true, err
		}
		steps[call.StepID] = true
	}
	for id := range steps {
		if err = persistStepStatus(ctx, loop.Store, db.FinishRunStepParams{ID: id, EnterpriseID: run.EnterpriseID, Status: "succeeded"}); err != nil {
			return true, err
		}
	}
	return true, nil
}

// Answers can change the intended target or scope. Complete the original native
// message group without dispatching decisions made before the answer, then let
// ReAct consume the real answer and construct the next operation. This also
// recovers a crash between recording an answer's ToolResult and closing its tail.
func (loop Loop) reconsiderQuestionTails(ctx context.Context, run db.Run) error {
	calls, err := loop.Store.Queries.ListAnsweredQuestionTails(ctx, db.ListAnsweredQuestionTailsParams{RunID: run.ID, EnterpriseID: run.EnterpriseID})
	if err != nil {
		return err
	}
	steps := map[uuid.UUID]bool{}
	for _, call := range calls {
		if _, err = loop.persistToolOutcome(ctx, run, call, toolruntime.Result{Data: map[string]any{"deferred": true, "reason": "New user input must be considered before choosing this operation again."}}, toolruntime.Error{Kind: "TOOL_CANCELLED"}); err != nil {
			return err
		}
		steps[call.StepID] = true
	}
	for id := range steps {
		unfinished, e := loop.Store.Queries.HasUnfinishedStepTools(ctx, db.HasUnfinishedStepToolsParams{StepID: id, EnterpriseID: run.EnterpriseID})
		if e != nil {
			return e
		}
		if !unfinished {
			if e = persistStepStatus(ctx, loop.Store, db.FinishRunStepParams{ID: id, EnterpriseID: run.EnterpriseID, Status: "succeeded"}); e != nil && !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
		}
	}
	return nil
}
