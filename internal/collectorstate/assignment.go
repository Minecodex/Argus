package collectorstate

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Cutover is atomic with recording the successful configuration. The previous
// execution remains authoritative on failure; a retired receiver is removed by
// a separate command only after the new configuration has actually converged.
func applyAssignment(ctx context.Context, q *db.Queries, before db.MonitoringCollection, collector uuid.UUID, revision, epoch int64) error {
	assignment, err := q.GetMonitoringAssignment(ctx, db.GetMonitoringAssignmentParams{EnterpriseID: before.EnterpriseID, CollectionID: before.ID, Epoch: epoch})
	if err != nil {
		return err
	}
	if err := q.EndOtherActiveMonitoringAssignments(ctx, db.EndOtherActiveMonitoringAssignmentsParams{EnterpriseID: before.EnterpriseID, CollectionID: before.ID, Epoch: epoch}); err != nil {
		return err
	}
	count, err := q.ActivateMonitoringAssignment(ctx, db.ActivateMonitoringAssignmentParams{EnterpriseID: before.EnterpriseID, CollectionID: before.ID, Epoch: epoch, CollectorID: collector})
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("collection assignment activation fence changed")
	}
	if before.EffectiveCollectorID.Valid && before.EffectiveCollectorID.UUID != collector && assignment.ApprovedActionID.Valid {
		return enqueueRetirement(ctx, q, before.EnterpriseID, before.ID, epoch, before.EffectiveCollectorID.UUID, assignment.ApprovedActionID.UUID)
	}
	return nil
}

func enqueueRetirement(ctx context.Context, q *db.Queries, enterprise, collection uuid.UUID, epoch int64, previous, approval uuid.UUID) error {
	row, err := q.CreateMonitoringAssignmentRetirement(ctx, db.CreateMonitoringAssignmentRetirementParams{ID: uuid.New(), EnterpriseID: enterprise, CollectionID: collection, AssignmentEpoch: epoch, PreviousCollectorID: previous, ApprovedActionID: approval})
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]uuid.UUID{"enterprise_id": enterprise, "id": row.ID})
	return q.EnqueueMonitoringAssignmentRetirement(ctx, db.EnqueueMonitoringAssignmentRetirementParams{ID: row.ID, EnterpriseID: uuid.NullUUID{UUID: enterprise, Valid: true}, Payload: payload})
}
