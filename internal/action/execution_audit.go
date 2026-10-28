package action

import (
	"context"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func executionAuditContext(ctx context.Context, action db.PendingAction, execution db.Execution) context.Context {
	return audit.WithOperation(ctx, audit.OperationContext{EnterpriseID: action.EnterpriseID, ActionID: action.ID,
		ActionRef: action.ActionRef, ActionType: action.ActionType, ExecutionID: execution.ID, RunID: action.RunID})
}

// Final state and its immutable audit event commit in the same transaction.
// The conditional FinishExecution update prevents replay from emitting twice.
func finishExecution(ctx context.Context, q *db.Queries, params db.FinishExecutionParams) (db.Execution, error) {
	execution, err := q.FinishExecution(ctx, params)
	if err != nil {
		return execution, err
	}
	action, err := q.GetPendingActionByID(ctx, db.GetPendingActionByIDParams{ID: execution.PendingActionID, EnterpriseID: execution.EnterpriseID})
	if err != nil {
		return execution, err
	}
	details := resource.PendingAuditDetails(action, map[string]any{"status": execution.Status, "execution_id": execution.ID})
	if execution.ErrorCode.Valid {
		details["error_code"] = execution.ErrorCode.String
	}
	result := "success"
	if execution.Status == "failed" {
		result = "failure"
	}
	resourceType, resourceID := executionAuditTarget(action, execution)
	_, err = audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: execution.EnterpriseID, Valid: true},
		ActorType: "system", ActorID: "action_executor", Action: "execution." + execution.Status,
		ResourceType: resourceType, ResourceID: resourceID.String(), Result: result, Details: details})
	return execution, err
}

func executionAuditTarget(action db.PendingAction, execution db.Execution) (string, uuid.UUID) {
	if action.ResourceID.Valid && action.ResourceType != "" {
		return action.ResourceType, action.ResourceID.UUID
	}
	if action.ResultResourceID.Valid && action.ResultResourceType.Valid {
		return action.ResultResourceType.String, action.ResultResourceID.UUID
	}
	return "execution", execution.ID
}
