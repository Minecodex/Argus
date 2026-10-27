package connector

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"time"
)

func updateKubernetesRuntimeObservation(ctx context.Context, q *db.Queries, identity TrustedIdentity, epoch int64, observation *connectorv1.KubernetesRuntimeObservation) error {
	if observation == nil {
		return nil
	}
	if observation.GetObservedAt() == nil || !observation.GetObservedAt().IsValid() || time.Since(observation.GetObservedAt().AsTime()) > 5*time.Minute || observation.GetObservedAt().AsTime().After(time.Now().Add(time.Minute)) || observation.GetServerVersion() == "" || len(observation.GetServerVersion()) > 128 || observation.GetNodeCount() > 10000 || observation.GetReadyNodeCount() > observation.GetNodeCount() {
		return ErrCommandState
	}
	rows, err := q.UpdateKubernetesRuntimeObservation(ctx, db.UpdateKubernetesRuntimeObservationParams{ConnectorID: uuid.NullUUID{UUID: identity.ConnectorID, Valid: true}, EnterpriseID: identity.EnterpriseID, ConnectionEpoch: epoch, NodeCount: int32(observation.GetNodeCount()), ReadyNodeCount: int32(observation.GetReadyNodeCount()), KubernetesVersion: observation.GetServerVersion(), StatisticsObservedAt: pgtype.Timestamptz{Time: observation.GetObservedAt().AsTime(), Valid: true}})
	if err != nil || rows != 1 {
		return ErrCommandState
	}
	return nil
}
