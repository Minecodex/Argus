package audit

import (
	"context"

	"github.com/google/uuid"
)

type operationContextKey struct{}

// OperationContext binds domain audit events to the server-owned action being
// executed. It carries identifiers only, never plans, commands or credentials.
type OperationContext struct {
	EnterpriseID uuid.UUID
	ActionID     uuid.UUID
	ActionRef    string
	ActionType   string
	ExecutionID  uuid.UUID
	RunID        uuid.NullUUID
}

func WithOperation(ctx context.Context, operation OperationContext) context.Context {
	return context.WithValue(ctx, operationContextKey{}, operation)
}

func operationDetails(ctx context.Context, entry Entry, details map[string]any) {
	operation, ok := ctx.Value(operationContextKey{}).(OperationContext)
	if !ok || entry.Domain != "enterprise" || !entry.EnterpriseID.Valid || entry.EnterpriseID.UUID != operation.EnterpriseID {
		return
	}
	details["pending_action_id"] = operation.ActionID
	details["action_ref"] = operation.ActionRef
	details["action_type"] = operation.ActionType
	details["execution_id"] = operation.ExecutionID
	if operation.RunID.Valid {
		details["run_id"] = operation.RunID.UUID
	}
}
