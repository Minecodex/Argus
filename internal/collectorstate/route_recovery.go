package collectorstate

import (
	"context"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// A physical reconnect can follow a verified configuration receipt after its
// action has already settled. Derive route readiness from both current facts;
// never infer a configuration result or actual data samples from a tunnel.
func ActivateVerifiedTunnelRoute(ctx context.Context, q *db.Queries, tunnel db.TelemetryTunnel) error {
	collectors, err := q.ActivateVerifiedTelemetryTunnelRoute(ctx, db.ActivateVerifiedTelemetryTunnelRouteParams{TunnelID: tunnel.ID, EnterpriseID: tunnel.EnterpriseID, Epoch: tunnel.Epoch, Fence: tunnel.Fence, LeaseOwner: tunnel.LeaseOwner})
	if err != nil {
		return err
	}
	for _, collector := range collectors {
		if _, err = q.FinalizeCollectorClaimMigrations(ctx, db.FinalizeCollectorClaimMigrationsParams{EnterpriseID: tunnel.EnterpriseID, CollectorID: collector}); err != nil {
			return err
		}
	}
	return nil
}
