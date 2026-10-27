package monitoring

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Real storage invariants; no live tunnel or installed Collector is claimed.
func TestPostgresReceiverChangesPreserveExistingRouteAndTunnel(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	collector := f.collector()
	f.exec("UPDATE hosts SET name='Owned transport leaf' WHERE id=(SELECT resource_id FROM collector_instances WHERE id=$1)", collector)
	gateway := f.collector()
	var host, root uuid.UUID
	if err := f.s.Store.Pool.QueryRow(ctx, "SELECT resource_id FROM collector_instances WHERE id=$1", collector).Scan(&host); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Store.Pool.QueryRow(ctx, "SELECT resource_id FROM collector_instances WHERE id=$1", gateway).Scan(&root); err != nil {
		t.Fatal(err)
	}
	scope, connector, secretID, credential := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f.exec("INSERT INTO bastion_scopes(id,enterprise_id,name,environment,labels_hash,onboarding_mode,status) VALUES($1,$2,'Owned transport scope','development',decode(repeat('00',32),'hex'),'command','active')", scope, f.a.EnterpriseID)
	f.exec("UPDATE hosts SET role='bastion',bastion_scope_id=$2 WHERE id=$1", root, scope)
	f.exec("UPDATE collector_instances SET role='edge_gateway' WHERE id=$1", gateway)
	f.exec("INSERT INTO connectors(id,enterprise_id,role,name,host_id,bastion_scope_id,instance_id,device_fingerprint_hash,public_key_hash,status,connection_epoch,certificate_expires_at) VALUES($1,$2,'bastion','Owned transport connector',$3,$4,$5,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),'online',3,now()+interval '1 hour')", connector, f.a.EnterpriseID, root, scope, connector.String())
	f.exec("INSERT INTO secrets(id,enterprise_id,name,type,created_by) VALUES($1,$2,'Transport metadata fixture','ssh_password',$3)", secretID, f.a.EnterpriseID, f.a.SubjectID)
	f.exec("INSERT INTO credentials(id,enterprise_id,name,protocol,username,secret_id) VALUES($1,$2,'Transport metadata fixture','ssh','root',$3)", credential, f.a.EnterpriseID, secretID)
	q := f.s.Store.Queries
	routeInput := db.UpsertTelemetryRouteParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, CollectorID: collector, Kind: "bastion_gateway", GatewayCollectorID: nullID(gateway), Transport: "bastion_tunnel", LoopbackPort: pgtype.Int4{Int32: 14318, Valid: true}}
	first, err := q.UpsertTelemetryRoute(ctx, routeInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.MarkTelemetryRouteActive(ctx, db.MarkTelemetryRouteActiveParams{CollectorID: collector, EnterpriseID: f.a.EnterpriseID}); err != nil {
		t.Fatal(err)
	}
	active, err := q.GetTelemetryRouteByCollector(ctx, db.GetTelemetryRouteByCollectorParams{CollectorID: collector, EnterpriseID: f.a.EnterpriseID})
	if err != nil {
		t.Fatal(err)
	}
	routeInput.ID = uuid.New()
	same, err := q.UpsertTelemetryRoute(ctx, routeInput)
	if err != nil || same.ID != first.ID || same.Status != "active" || same.Version != active.Version {
		t.Fatalf("receiver-only update reset route: status=%s version=%d expected=%d err=%v", same.Status, same.Version, active.Version, err)
	}
	input := db.CreateTelemetryTunnelParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, HostID: host, CollectorID: collector, ConnectorID: nullID(connector), CredentialID: credential, CredentialVersion: 1, TargetAddress: "192.0.2.10", TargetPort: 22, TargetUsername: "root", PinnedHostKey: "SHA256:owned-storage-fixture", Initiator: "connector", Transport: "bastion_tunnel", LoopbackPort: 14318, ForwardTarget: "gateway"}
	tunnel, err := q.CreateTelemetryTunnel(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"established", "degraded"} {
		f.exec("UPDATE telemetry_tunnels SET status=$2,epoch=7,fence=4,lease_owner=$3,owner_connection_epoch=3,lease_expires_at=now()+interval '90 seconds',reconnect_attempt=2,next_claim_at=now()+interval '30 seconds' WHERE id=$1", tunnel.ID, state, "connector:"+connector.String()+":3")
		prior, err := q.GetTelemetryTunnel(ctx, db.GetTelemetryTunnelParams{ID: tunnel.ID, EnterpriseID: f.a.EnterpriseID})
		if err != nil {
			t.Fatal(err)
		}
		input.ID = uuid.New()
		current, err := q.CreateTelemetryTunnel(ctx, input)
		if err != nil || current.ID != tunnel.ID || current.Status != prior.Status || current.Epoch != prior.Epoch || current.Fence != prior.Fence || current.LeaseOwner != prior.LeaseOwner || current.OwnerConnectionEpoch != prior.OwnerConnectionEpoch || current.LeaseExpiresAt != prior.LeaseExpiresAt || current.NextClaimAt != prior.NextClaimAt || current.ReconnectAttempt != prior.ReconnectAttempt {
			t.Fatalf("receiver-only update reset %s transport: epoch=%d state=%s owner_epoch=%d err=%v", state, current.Epoch, current.Status, current.OwnerConnectionEpoch, err)
		}
	}
	input.TargetPort = 2222
	changed, err := q.CreateTelemetryTunnel(ctx, input)
	if err != nil || changed.ID != tunnel.ID || changed.Status != "desired" || changed.Epoch != 8 || changed.LeaseOwner != "" || changed.OwnerConnectionEpoch != 0 || changed.LeaseExpiresAt.Valid || changed.NextClaimAt.Valid || changed.ReconnectAttempt != 0 {
		t.Fatal("changed connection did not fence and rebuild its transport", err)
	}
	routeInput.LoopbackPort.Int32 = 14320
	changedRoute, err := q.UpsertTelemetryRoute(ctx, routeInput)
	if err != nil || changedRoute.Status != "pending" || changedRoute.Version != active.Version+1 {
		t.Fatal("changed route reused the previous active proof", err)
	}
}
