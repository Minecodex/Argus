// Package collectorstate records frozen Collector command outcomes consistently
// for direct execution and Connector delivery.
package collectorstate

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Apply returns false for a superseded command. A late result must never label
// a newer desired revision effective, failed, or healthy.
func Apply(ctx context.Context, q *db.Queries, enterprise, collector uuid.UUID, revision int64, operation string, succeeded bool, code string) (bool, error) {
	if revision < 1 {
		return false, errors.New("invalid Collector outcome revision")
	}
	// User changes lock an instance before its Collector. Follow the same
	// ordering here so a config callback cannot deadlock a concurrent edit.
	if operation != "uninstall" {
		frozen, err := q.GetCollectorConfigRevision(ctx, db.GetCollectorConfigRevisionParams{CollectorID: collector, Revision: revision})
		if err != nil {
			return false, err
		}
		var inputs configbundle.Bundle
		if json.Unmarshal(frozen.RenderedConfig, &inputs) != nil {
			return false, errors.New("invalid frozen Collector bundle")
		}
		ids := []uuid.UUID{}
		for _, item := range inputs.Collections {
			id, err := uuid.Parse(item.ID)
			if err != nil {
				return false, err
			}
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
		for _, id := range ids {
			if _, err := q.LockMonitoringCollection(ctx, db.LockMonitoringCollectionParams{EnterpriseID: enterprise, ID: id}); err != nil {
				return false, err
			}
		}
	}
	status := "degraded"
	if OutcomeUnknown(code) {
		status = "result_unknown"
	}
	if succeeded {
		status = "converged"
		if operation == "uninstall" {
			status = "uninstalled"
		}
	}
	updated, err := q.ApplyCollectorRevisionOutcome(ctx, db.ApplyCollectorRevisionOutcomeParams{EnterpriseID: enterprise, CollectorID: collector, Revision: revision, Status: status, Applied: succeeded && operation != "uninstall"})
	if err != nil || updated == 0 {
		return false, err
	}
	if operation == "uninstall" {
		return true, nil
	}
	config, err := q.GetCollectorConfigRevision(ctx, db.GetCollectorConfigRevisionParams{CollectorID: collector, Revision: revision})
	if err != nil {
		return false, err
	}
	if succeeded {
		var count int64
		count, err = q.MarkCollectorConfigEffective(ctx, db.MarkCollectorConfigEffectiveParams{CollectorID: collector, Revision: revision, Reconciled: OutcomeUnknown(config.FailureCode.String)})
		if err == nil && count != 1 {
			err = errors.New("Collector configuration outcome is no longer recoverable")
		}
	} else {
		_, err = q.MarkCollectorConfigFailed(ctx, db.MarkCollectorConfigFailedParams{CollectorID: collector, Revision: revision, FailureCode: pgtype.Text{String: code, Valid: code != ""}})
	}
	if err != nil {
		return false, err
	}
	var bundle configbundle.Bundle
	if json.Unmarshal(config.RenderedConfig, &bundle) != nil {
		return false, errors.New("invalid frozen Collector bundle")
	}
	// No telemetry sample or health is fabricated by deployment success.
	enabledIDs := []uuid.UUID{}
	for _, item := range bundle.Collections {
		id, err := uuid.Parse(item.ID)
		if err != nil {
			return false, err
		}
		if item.Enabled {
			enabledIDs = append(enabledIDs, id)
		}
		before, err := q.LockMonitoringCollection(ctx, db.LockMonitoringCollectionParams{EnterpriseID: enterprise, ID: id})
		if err != nil {
			return false, err
		}
		count, err := q.ApplyMonitoringCollectionRevision(ctx, db.ApplyMonitoringCollectionRevisionParams{EnterpriseID: enterprise, CollectionID: id, CollectorID: uuid.NullUUID{UUID: collector, Valid: true}, Revision: item.Revision, AssignmentEpoch: item.AssignmentEpoch, Applied: succeeded, OutcomeUnknown: !succeeded && OutcomeUnknown(code), ErrorCode: code})
		if err != nil {
			return false, err
		}
		if succeeded && count == 1 {
			if err := applyAssignment(ctx, q, before, collector, item.Revision, item.AssignmentEpoch); err != nil {
				return false, err
			}
			if err := secret.RetireUnusedMonitoringCredentials(ctx, q, enterprise, id); err != nil {
				return false, err
			}
		}
		if succeeded && count == 0 && before.CollectorID.UUID != collector && before.EffectiveCollectorID.UUID != collector {
			assignment, e := q.GetMonitoringAssignment(ctx, db.GetMonitoringAssignmentParams{EnterpriseID: enterprise, CollectionID: id, Epoch: item.AssignmentEpoch})
			if e == nil && assignment.ApprovedActionID.Valid {
				if err := enqueueRetirement(ctx, q, enterprise, id, before.AssignmentEpoch, collector, assignment.ApprovedActionID.UUID); err != nil {
					return false, err
				}
			}
		}
	}
	if succeeded {
		if err := q.CompleteMonitoringAssignmentRetirements(ctx, db.CompleteMonitoringAssignmentRetirementsParams{EnterpriseID: enterprise, PreviousCollectorID: collector, Column3: enabledIDs}); err != nil {
			return false, err
		}
		if err := q.ClearRetiredCollectorCollectionExecution(ctx, db.ClearRetiredCollectorCollectionExecutionParams{EnterpriseID: enterprise, EffectiveCollectorID: uuid.NullUUID{UUID: collector, Valid: true}, Column3: enabledIDs}); err != nil {
			return false, err
		}
	}
	return true, nil
}
