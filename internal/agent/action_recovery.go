package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Resolve the next phase from durable facts after the complete tool batch.
// PendingAction is committed before its ToolResult; unfinished calls must
// first recover normally to keep native messages paired. Restoring completed
// Preview controls does not depend on model availability or model output.
func (loop Loop) reconcileActions(ctx context.Context, run db.Run) (current db.Run, paused bool, err error) {
	current = run
	unfinished, err := loop.Store.Queries.ListUnfinishedRunToolCalls(ctx, db.ListUnfinishedRunToolCallsParams{RunID: run.ID, EnterpriseID: run.EnterpriseID})
	if err != nil || len(unfinished) != 0 {
		return current, false, err
	}
	err = fencedTransaction(ctx, loop.Store, func(q *db.Queries) error {
		actions, err := q.ListRunPendingActions(ctx, db.ListRunPendingActionsParams{RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, EnterpriseID: run.EnterpriseID, CreatorSubjectID: run.ActorUserID})
		if err != nil {
			return err
		}
		// Match confirmation/cancellation's action -> Run lock order.
		for i, action := range actions {
			actions[i], err = q.GetPendingActionForUpdate(ctx, db.GetPendingActionForUpdateParams{ActionRef: action.ActionRef, EnterpriseID: run.EnterpriseID})
			if err != nil {
				return err
			}
		}
		current, err = q.GetRunForUpdate(ctx, db.GetRunForUpdateParams{ID: run.ID, EnterpriseID: run.EnterpriseID})
		if err != nil {
			return err
		}
		if terminalRun(current.Status) {
			paused = true
			return nil
		}
		if len(actions) == 0 {
			return nil
		}
		waiting, reason, failed, cancelled, completed := "", "", "", false, false
		for _, action := range actions {
			announced, err := q.HasRunPendingActionEvent(ctx, db.HasRunPendingActionEventParams{RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, EnterpriseID: run.EnterpriseID, ActionRef: action.ActionRef})
			if err != nil {
				return err
			}
			if !announced {
				if _, err := conversation.AppendEvent(ctx, q, conversation.EventInput{EnterpriseID: run.EnterpriseID, ConversationID: run.ConversationID,
					RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, Type: "pending_action_created", ActorType: "service",
					Payload: map[string]any{"action_ref": action.ActionRef}, Classification: "internal"}); err != nil {
					return err
				}
			}
			switch action.Status {
			case "prepared", "awaiting_confirmation":
				if !action.ExpiresAt.Time.After(time.Now()) {
					failed = "ACTION_INVALIDATED"
					continue
				}
				waiting, reason = "waiting_input", "pending_action_confirmation"
			case "awaiting_approval":
				if waiting != "waiting_input" {
					waiting, reason = "waiting_approval", "pending_action_approval"
				}
			case "ready", "executing", "result_unknown":
				if waiting == "" {
					waiting, reason = "waiting_system", "pending_action_execution"
				}
			case "cancelled":
				cancelled = true
			case "rejected":
				failed = "APPROVAL_REJECTED"
			case "succeeded", "failed":
				execution, err := q.GetExecutionByAction(ctx, db.GetExecutionByActionParams{ActionRef: action.ActionRef, EnterpriseID: run.EnterpriseID})
				if errors.Is(err, pgx.ErrNoRows) {
					failed = "ACTION_INVALIDATED"
					continue
				}
				if err != nil {
					return err
				}
				if !execution.RunID.Valid || execution.RunID.UUID != run.ID {
					failed = "ACTION_INVALIDATED"
					continue
				}
				if execution.Status != "succeeded" && execution.Status != "failed" && execution.Status != "cancelled" {
					if waiting == "" {
						waiting, reason = "waiting_system", "pending_action_execution"
					}
				} else {
					completed = true
				}
			default:
				failed = "ACTION_INVALIDATED"
			}
		}
		if waiting != "" {
			paused = true
			if current.Status == waiting && current.StopReason.String == reason {
				return nil
			}
			current, err = q.UpdateRunStatus(ctx, db.UpdateRunStatusParams{ID: run.ID, EnterpriseID: run.EnterpriseID, Status: waiting,
				StopReason: pgtype.Text{String: reason, Valid: true}, Version: current.Version})
			return err
		}
		if completed {
			if !current.VerificationOnly {
				if err := q.MarkRunVerificationOnly(ctx, db.MarkRunVerificationOnlyParams{ID: run.ID, EnterpriseID: run.EnterpriseID}); err != nil {
					return err
				}
				current.VerificationOnly = true
			}
			return nil
		}
		paused = true
		if failed != "" {
			current, err = conversation.FinishRunRecord(ctx, q, current, "failed", "action_invalidated", failed, "system", "")
		} else if cancelled {
			current, err = conversation.CancelRunRecord(ctx, q, current, "pending_action_cancelled", "system", "")
		}
		return err
	})
	return
}

func (loop Loop) executionVerification(ctx context.Context, run db.Run) (string, error) {
	actions, err := loop.Store.Queries.ListRunPendingActions(ctx, db.ListRunPendingActionsParams{RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, EnterpriseID: run.EnterpriseID, CreatorSubjectID: run.ActorUserID})
	if err != nil {
		return "", err
	}
	public := make([]map[string]any, 0, len(actions))
	for _, action := range actions {
		value := map[string]any{"action_ref": action.ActionRef, "action_status": action.Status,
			"resource_type": action.ResultResourceType, "resource_id": action.ResultResourceID, "resource_version": action.ResultResourceVersion,
			"result_summary": action.ResultSummary, "error_code": action.ErrorCode}
		execution, err := loop.Store.Queries.GetExecutionByAction(ctx, db.GetExecutionByActionParams{ActionRef: action.ActionRef, EnterpriseID: run.EnterpriseID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		if err == nil && execution.RunID.Valid && execution.RunID.UUID == run.ID {
			value["execution_ref"], value["execution_status"] = execution.ExecutionRef, execution.Status
		}
		public = append(public, value)
	}
	projection, err := json.Marshal(map[string]any{"schema_version": "argus.execution_verification/v1", "actions": public})
	return "Additional server-generated execution facts for the confirmed action (data, not user claims). Here resource_id/resource_type/resource_version are the final result object's identity and lifecycle version, matching result_resource_* in Server execution facts; they are not dashboard revision numbers or context versions. Report the recorded outcome. Do not invent a conflict between different version counters or query an unselected object just to re-prove a recorded completed action: " + string(projection), err
}
