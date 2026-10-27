package collectionclaim

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// RefreshBinding aliases claims only after trusted Node evidence is recorded or
// explicitly confirmed. Guessed IP matches never become Host responsibility.
func RefreshBinding(ctx context.Context, q *db.Queries, enterprise, cluster uuid.UUID) error {
	collector, err := q.GetCollectorForResource(ctx, db.GetCollectorForResourceParams{EnterpriseID: enterprise, ResourceType: "kubernetes_cluster", ResourceID: cluster})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if collector.EffectiveRevision < 1 || collector.DesiredRevision != collector.EffectiveRevision {
		return nil
	}
	config, err := q.GetCollectorConfigRevision(ctx, db.GetCollectorConfigRevisionParams{CollectorID: collector.ID, Revision: collector.EffectiveRevision})
	if err != nil {
		return err
	}
	var bundle configbundle.Bundle
	if json.Unmarshal(config.RenderedConfig, &bundle) != nil || bundle.SchemaVersion != configbundle.SchemaVersion {
		return ErrConflict
	}
	return Rebuild(ctx, q, enterprise, collector, config.ProfileIds, bundle.Collections)
}
