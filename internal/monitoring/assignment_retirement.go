package monitoring

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

const AssignmentRetirementQueue = "monitoring_assignment_retire"

type AssignmentRetirementWorker struct{ Service Service }

func (w AssignmentRetirementWorker) Handle(ctx context.Context, task runtime.Task) error {
	var input struct {
		EnterpriseID uuid.UUID `json:"enterprise_id"`
		ID           uuid.UUID `json:"id"`
	}
	if json.Unmarshal(task.Payload, &input) != nil || input.EnterpriseID == uuid.Nil || input.ID == uuid.Nil {
		return ErrInvalid
	}
	if task.EnterpriseID.Valid && task.EnterpriseID.UUID != input.EnterpriseID {
		return ErrDenied
	}
	return w.Service.Store.InTx(ctx, func(q *db.Queries) error {
		row, err := q.GetMonitoringAssignmentRetirement(ctx, db.GetMonitoringAssignmentRetirementParams{EnterpriseID: input.EnterpriseID, ID: input.ID})
		if err != nil {
			return err
		}
		if row.Status != "pending" {
			return nil
		}
		collection, err := q.LockMonitoringCollection(ctx, db.LockMonitoringCollectionParams{EnterpriseID: input.EnterpriseID, ID: row.CollectionID})
		if err != nil {
			return err
		}
		// Configuration outcomes lock collections before settling retirements.
		// Follow the same order; locking retirement first can deadlock an ack.
		row, err = q.LockMonitoringAssignmentRetirement(ctx, db.LockMonitoringAssignmentRetirementParams{EnterpriseID: input.EnterpriseID, ID: input.ID})
		if err != nil || row.Status != "pending" {
			return err
		}
		if collection.CollectorID.UUID == row.PreviousCollectorID || collection.EffectiveCollectorID.UUID == row.PreviousCollectorID {
			// A later placement change made this executor current again. The old
			// task cannot remove it or rewrite another user's configuration.
			return q.UpdateMonitoringAssignmentRetirement(ctx, db.UpdateMonitoringAssignmentRetirementParams{EnterpriseID: row.EnterpriseID, ID: row.ID, Status: "completed"})
		}
		approval, err := q.GetMonitoringAssignmentApproval(ctx, db.GetMonitoringAssignmentApprovalParams{EnterpriseID: row.EnterpriseID, ID: row.ApprovedActionID})
		if err != nil {
			return err
		}
		a := Actor{EnterpriseID: row.EnterpriseID, SubjectType: approval.CreatorSubjectType, SubjectID: approval.CreatorSubjectID, AuthorizationVersion: approval.AuthorizationVersion, RunID: approval.RunID.UUID}
		collector, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: row.EnterpriseID, ID: row.PreviousCollectorID})
		if err != nil {
			return err
		}
		for _, permission := range []string{"monitoring.manage", "collector.use"} {
			if err = authorization.Authorize(ctx, q, a, permission); err != nil {
				break
			}
		}
		if err == nil {
			_, err = requireTarget(ctx, q, a, ResourceRef{Kind: collection.TargetType, ID: collection.TargetID})
		}
		if err == nil {
			err = authorization.RequireResource(ctx, q, a, collector.ResourceType, collector.ResourceID)
		}
		if err != nil {
			return q.UpdateMonitoringAssignmentRetirement(ctx, db.UpdateMonitoringAssignmentRetirementParams{EnterpriseID: row.EnterpriseID, ID: row.ID, Status: "blocked", ErrorCode: "COLLECTION_RETIRE_AUTHORIZATION_CHANGED"})
		}
		if collector.DesiredRevision != collector.EffectiveRevision {
			return ErrUnavailable
		}
		if w.Service.Runtime == nil {
			return ErrUnavailable
		}
		plan, err := w.Service.Runtime.Plan(ctx, q, row.EnterpriseID, row.PreviousCollectorID)
		if err != nil {
			return err
		}
		approval.ActionRef += "_retire_" + row.ID.String()
		result, err := w.Service.Runtime.Commit(ctx, q, approval, plan)
		if err != nil {
			if errors.Is(err, resource.ErrActionInvalidated) {
				return ErrUnavailable
			}
			return err
		}
		operation := result.TelemetryOperationID
		if result.ConnectorCommandID.Valid {
			operation = result.ConnectorCommandID
		}
		return q.UpdateMonitoringAssignmentRetirement(ctx, db.UpdateMonitoringAssignmentRetirementParams{EnterpriseID: row.EnterpriseID, ID: row.ID, Status: "dispatched", CollectorOperationID: operation})
	})
}

func (w AssignmentRetirementWorker) HandleExhausted(ctx context.Context, task runtime.Task, cause error) error {
	var input struct {
		EnterpriseID uuid.UUID `json:"enterprise_id"`
		ID           uuid.UUID `json:"id"`
	}
	if json.Unmarshal(task.Payload, &input) != nil || input.EnterpriseID == uuid.Nil || input.ID == uuid.Nil {
		return ErrInvalid
	}
	if task.EnterpriseID.Valid && task.EnterpriseID.UUID != input.EnterpriseID {
		return ErrDenied
	}
	return w.Service.Store.InTx(ctx, func(q *db.Queries) error {
		row, err := q.LockMonitoringAssignmentRetirement(ctx, db.LockMonitoringAssignmentRetirementParams{EnterpriseID: input.EnterpriseID, ID: input.ID})
		if err != nil || row.Status != "pending" {
			return err
		}
		return q.UpdateMonitoringAssignmentRetirement(ctx, db.UpdateMonitoringAssignmentRetirementParams{EnterpriseID: row.EnterpriseID, ID: row.ID, Status: "blocked", ErrorCode: "COLLECTION_RETIRE_RETRY_EXHAUSTED"})
	})
}
