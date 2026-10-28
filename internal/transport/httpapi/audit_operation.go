package httpapi

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	auditapi "github.com/kakj-go/Argus/internal/gen/openapi/audit"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Projection is resolved only after the audit event passes domain/scope checks.
// Immutable details and hashes remain untouched; no preview or secret is read.
func (resolver *auditPresentationResolver) resolveOperation(ctx context.Context, row db.AuditEvent) (*auditapi.AuditOperation, error) {
	if row.Domain != "enterprise" || !row.EnterpriseID.Valid {
		return nil, nil
	}
	var detail struct {
		ActionRef        string `json:"action_ref"`
		PendingActionRef string `json:"pending_action_ref"`
		ResourceName     string `json:"resource_name"`
	}
	_ = json.Unmarshal(row.Details, &detail)
	ref := detail.ActionRef
	if ref == "" {
		ref = detail.PendingActionRef
	}
	var action db.PendingAction
	var err error
	if ref != "" {
		if value, found := resolver.operations[ref]; found {
			return value, nil
		}
		action, err = resolver.queries.GetPendingAction(ctx, db.GetPendingActionParams{ActionRef: ref, EnterpriseID: row.EnterpriseID.UUID})
	} else if row.ResourceType.String == "execution" {
		id, parseErr := uuid.Parse(row.ResourceID.String)
		if parseErr != nil {
			return nil, nil
		}
		execution, queryErr := resolver.queries.GetExecution(ctx, db.GetExecutionParams{ID: id, EnterpriseID: row.EnterpriseID.UUID})
		if errors.Is(queryErr, pgx.ErrNoRows) {
			return nil, nil
		}
		if queryErr != nil {
			return nil, queryErr
		}
		action, err = resolver.queries.GetPendingActionByID(ctx, db.GetPendingActionByIDParams{ID: execution.PendingActionID, EnterpriseID: row.EnterpriseID.UUID})
	} else {
		return nil, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value := &auditapi.AuditOperation{ActionRef: action.ActionRef, ActionType: action.ActionType, ActionStatus: action.Status,
		CreatedAt: action.CreatedAt.Time, UpdatedAt: action.UpdatedAt.Time}
	if action.ErrorCode.Valid {
		value.ErrorCode = &action.ErrorCode.String
	}
	resourceType, resourceID := action.ResourceType, action.ResourceID
	if !resourceID.Valid && action.ResultResourceID.Valid {
		resourceType, resourceID = action.ResultResourceType.String, action.ResultResourceID
	}
	if resourceID.Valid && resourceType != "" {
		value.ResourceType, value.ResourceId = &resourceType, &resourceID.UUID
		name := resolver.resolveResource(ctx, db.AuditEvent{EnterpriseID: row.EnterpriseID, ResourceType: pgtype.Text{String: resourceType, Valid: true}, ResourceID: pgtype.Text{String: resourceID.UUID.String(), Valid: true}})
		if name != "" {
			value.ResourceName = &name
		}
	}
	if value.ResourceName == nil && detail.ResourceName != "" && len(detail.ResourceName) <= 256 {
		value.ResourceName = &detail.ResourceName
	}
	execution, err := resolver.queries.GetExecutionByAction(ctx, db.GetExecutionByActionParams{ActionRef: action.ActionRef, EnterpriseID: action.EnterpriseID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		value.ExecutionId, value.ExecutionStatus = &execution.ID, &execution.Status
		if execution.ErrorCode.Valid {
			value.ErrorCode = &execution.ErrorCode.String
		}
		if execution.UpdatedAt.Time.After(value.UpdatedAt) {
			value.UpdatedAt = execution.UpdatedAt.Time
		}
	}
	if resolver.operations == nil {
		resolver.operations = make(map[string]*auditapi.AuditOperation)
	}
	resolver.operations[action.ActionRef] = value
	return value, nil
}
