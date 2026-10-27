package connector

import (
	"context"
	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	redisstore "github.com/kakj-go/Argus/internal/storage/redis"
	"github.com/kakj-go/Argus/internal/tunnelaudit"
)

type tunnelRuntimeEvent struct {
	tunnel         db.TelemetryTunnel
	event          tunnelaudit.Event
	status, reason string
}

// The connector lock precedes the session and all dependent projections, just
// as OpenSession and Disconnect do. A rejected frame has no partial writes.
func (gateway Gateway) withCurrentRuntimeSession(ctx context.Context, identity TrustedIdentity, epoch int64, fn func(*db.Queries) error) error {
	return gateway.Service.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		if _, err := q.LockConnectorRuntime(ctx, db.LockConnectorRuntimeParams{ID: identity.ConnectorID, EnterpriseID: identity.EnterpriseID, ConnectionEpoch: epoch}); err != nil {
			return ErrConnectorFenced
		}
		return fn(q)
	})
}

func (gateway Gateway) applyRuntimeHeartbeat(ctx context.Context, identity TrustedIdentity, epoch int64, frame *connectorv1.ConnectorHeartbeat) error {
	if frame.GetConnectionEpoch() != uint64(epoch) {
		return ErrConnectorFenced
	}
	var events []tunnelRuntimeEvent
	err := gateway.withCurrentRuntimeSession(ctx, identity, epoch, func(q *db.Queries) error {
		rows, err := q.HeartbeatConnectorSession(ctx, db.HeartbeatConnectorSessionParams{ConnectorID: identity.ConnectorID, EnterpriseID: identity.EnterpriseID, ConnectionEpoch: epoch})
		if err != nil {
			return err
		}
		if rows != 1 {
			return ErrConnectorFenced
		}
		if err = updateHostRuntimeObservation(ctx, q, identity, frame.GetHostRuntime()); err != nil {
			return err
		}
		if err = updateKubernetesRuntimeObservation(ctx, q, identity, epoch, frame.GetKubernetesRuntime()); err != nil {
			return err
		}
		if err = updateBastionRelayStatus(ctx, q, identity, frame.GetBastionRelay()); err != nil {
			return err
		}
		if events, err = gateway.applyTelemetryTunnelStatusesWithQueries(ctx, q, identity, epoch, frame.GetTelemetryTunnels()); err != nil {
			return err
		}
		_, err = q.RestoreHostConnectorOnline(ctx, db.RestoreHostConnectorOnlineParams{EnterpriseID: identity.EnterpriseID, ConnectorID: uuid.NullUUID{UUID: identity.ConnectorID, Valid: true}})
		return err
	})
	if err != nil {
		return err
	}
	gateway.auditRuntimeEvents(ctx, identity, events)
	if gateway.Service.Redis != nil {
		_ = gateway.Service.Redis.SetConnectorRegistry(ctx, identity.ConnectorID.String(), redisstore.ConnectorRegistryEntry{GatewayInstanceID: gateway.Service.GatewayInstance, ConnectionEpoch: epoch}, gateway.Service.registryTTL())
	}
	return nil
}

func (gateway Gateway) auditRuntimeEvents(ctx context.Context, identity TrustedIdentity, events []tunnelRuntimeEvent) {
	for _, event := range events {
		gateway.auditTelemetryTunnel(ctx, identity, event.event, event.tunnel, event.status, event.reason)
	}
}
