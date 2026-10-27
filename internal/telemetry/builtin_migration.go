package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// A confirmed Collector configuration materializes old fixed profile choices
// as real collection intents in the same transaction. No samples or healthy
// state are synthesized; the normal bundle acknowledgement owns apply state.
func materializeBuiltinProfiles(ctx context.Context, q *db.Queries, action db.PendingAction, collector db.CollectorInstance, plan collectorActionPlan) error {
	for _, key := range plan.ProfileKeys {
		profile, _, _ := strings.Cut(key, "@")
		for _, definition := range builtincollection.All() {
			if definition.LegacyProfile != profile || !slices.Contains(definition.ResourceTypes, collector.ResourceType) || !slices.Contains(definition.Platforms, collector.Platform) {
				continue
			}
			id := builtincollection.StableID(collector.ResourceType, collector.ResourceID, definition.Key)
			if _, err := q.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: action.EnterpriseID, ID: id}); err == nil {
				continue
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if _, err := q.CreateMonitoringCollection(ctx, db.CreateMonitoringCollectionParams{ID: id, EnterpriseID: action.EnterpriseID, Name: definition.Name, IntegrationKey: definition.Key, IntegrationVersion: builtincollection.Version, TargetType: collector.ResourceType, TargetID: collector.ResourceID, CollectorID: uuid.NullUUID{UUID: collector.ID, Valid: true}, Config: json.RawMessage(`{"interval":"30s"}`), DesiredEnabled: true, CreatedBy: action.CreatorSubjectID}); err != nil {
				return err
			}
			if _, err := q.SnapshotMonitoringCollection(ctx, db.SnapshotMonitoringCollectionParams{EnterpriseID: action.EnterpriseID, CollectionID: id, ActorID: action.CreatorSubjectID}); err != nil {
				return err
			}
			if err := q.CreateMonitoringAssignment(ctx, db.CreateMonitoringAssignmentParams{EnterpriseID: action.EnterpriseID, CollectionID: id, CollectorID: collector.ID, Epoch: 1}); err != nil {
				return err
			}
		}
	}
	return nil
}
