package collectorstate

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// A validation failure happens before activation. It must not disable the
// unchanged, active route of an already verified effective configuration.
// Failed activation without verified rollback continues to fail closed.
func RouteAfterFailure(ctx context.Context, q *db.Queries, enterprise, collector uuid.UUID, operation, code string) error {
	if operation == "configure" && (code == "COLLECTOR_CONFIG_INVALID" || code == "CONNECTOR_COMMAND_EXPIRED" || code == "COLLECTOR_OPERATION_EXPIRED") {
		route, err := q.GetTelemetryRouteByCollector(ctx, db.GetTelemetryRouteByCollectorParams{EnterpriseID: enterprise, CollectorID: collector})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil && route.Status == "active" {
			current, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: enterprise, ID: collector})
			if err != nil {
				return err
			}
			if current.EffectiveRevision > 0 && current.EffectiveRevision < current.DesiredRevision {
				effective, err := q.GetCollectorConfigRevision(ctx, db.GetCollectorConfigRevisionParams{CollectorID: collector, Revision: current.EffectiveRevision})
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
				if err == nil && effective.Status == "effective" && len(effective.ConfigHash) == 32 {
					return nil
				}
			}
		}
	}
	_, err := q.MarkTelemetryRouteDegraded(ctx, db.MarkTelemetryRouteDegradedParams{EnterpriseID: enterprise, CollectorID: collector})
	return err
}
