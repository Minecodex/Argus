package resource

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"strings"
)

func pendingAuditDetails(action db.PendingAction, details map[string]any) map[string]any {
	return PendingAuditDetails(action, details)
}

// PendingAuditDetails carries the same authorized scope through preview,
// host confirmation, cancellation and execution audit records.
func PendingAuditDetails(action db.PendingAction, details map[string]any) map[string]any {
	if details == nil {
		details = map[string]any{}
	}
	details["action_ref"] = action.ActionRef
	details["action_type"] = action.ActionType
	details["pending_action_id"] = action.ID
	var preview map[string]any
	_ = json.Unmarshal(action.Preview, &preview)
	if name, ok := preview["name"].(string); ok && name != "" && len(name) <= 256 {
		details["resource_name"] = name
	}
	if action.RunID.Valid {
		details["run_id"] = action.RunID.UUID
	}
	if !strings.HasPrefix(action.ActionType, "alert.") {
		return details
	}
	scope := preview["resource_ids"]
	if scope == nil {
		scope = []any{}
	}
	details["resource_ids"] = scope
	details["pending_action_ref"] = action.ActionRef
	details["pending_action_id"] = action.ID
	if action.RunID.Valid {
		details["run_id"] = action.RunID.UUID
		if id, ok := preview["tool_call_id"].(string); ok && id != "" {
			details["tool_call_id"] = id
		}
	}
	return details
}

func PendingExecutionAuditDetails(ctx context.Context, q *db.Queries, action db.PendingAction, details map[string]any) (map[string]any, error) {
	details = PendingAuditDetails(action, details)
	execution, err := q.GetExecutionByAction(ctx, db.GetExecutionByActionParams{ActionRef: action.ActionRef, EnterpriseID: action.EnterpriseID})
	if errors.Is(err, pgx.ErrNoRows) {
		return details, nil
	}
	if err != nil {
		return nil, err
	}
	details["execution_id"] = execution.ID
	return details, nil
}
