package connector

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/hostinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestPostgresHeartbeatFencesEveryProjectionAndRollsBackInvalidFrames(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	e, h, b, c := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Owned heartbeat fence',$2,'UTC')", e, e.String())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tx, err := store.Pool.Begin(ctx)
		if err != nil {
			t.Error(err)
			return
		}
		defer tx.Rollback(ctx)
		for _, query := range []string{
			"UPDATE hosts SET connector_id=NULL WHERE enterprise_id=$1",
			"UPDATE bastion_scopes SET active_connector_id=NULL,connector_host_id=NULL WHERE enterprise_id=$1",
			"DELETE FROM host_runtime_observations WHERE enterprise_id=$1",
			"DELETE FROM connector_sessions WHERE enterprise_id=$1",
			"DELETE FROM connectors WHERE enterprise_id=$1",
			"DELETE FROM hosts WHERE enterprise_id=$1",
			"DELETE FROM bastion_scopes WHERE enterprise_id=$1",
			"DELETE FROM enterprises WHERE id=$1",
		} {
			if _, err = tx.Exec(ctx, query, e); err != nil {
				t.Error(err)
				return
			}
		}
		if err = tx.Commit(ctx); err != nil {
			t.Error(err)
		}
	})
	exec("INSERT INTO bastion_scopes(id,enterprise_id,name,environment,labels_hash,onboarding_mode,status,relay_status) VALUES($1,$2,'Owned heartbeat scope','development',decode(repeat('00',32),'hex'),'command','active','ready')", b, e)
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,bastion_scope_id,labels_hash,connection_status) VALUES($1,$2,'Owned heartbeat host','',0,'development','linux','bastion','direct',$3,decode(repeat('00',32),'hex'),'online')", h, e, b)
	exec("INSERT INTO connectors(id,enterprise_id,role,name,host_id,bastion_scope_id,instance_id,device_fingerprint_hash,public_key_hash,status,connection_epoch,certificate_expires_at) VALUES($1,$2,'bastion','Owned heartbeat connector',$3,$4,$5,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),'online',2,now()+interval '1 hour')", c, e, h, b, c.String())
	exec("UPDATE hosts SET connector_id=$2 WHERE id=$1", h, c)
	exec("UPDATE bastion_scopes SET connector_host_id=$2,active_connector_id=$3 WHERE id=$1", b, h, c)
	_, err = store.Queries.UpsertConnectorSession(t.Context(), db.UpsertConnectorSessionParams{ConnectorID: c, EnterpriseID: e, GatewayInstanceID: "current", ConnectionEpoch: 2, Capabilities: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	gateway := Gateway{Service: Service{Store: store}}
	identity := TrustedIdentity{EnterpriseID: e, ConnectorID: c}
	frame := func(epoch uint64, hostname string, offset time.Duration) *connectorv1.ConnectRequest {
		at := time.Now().UTC().Add(offset)
		inventory, _ := json.Marshal(hostinventory.Snapshot{SchemaVersion: hostinventory.SchemaVersion, CollectedAt: at, Hostname: hostname, Scope: "machine", Disks: []hostinventory.Disk{}, Networks: []hostinventory.Network{}, GPUs: []hostinventory.GPU{}, Issues: []hostinventory.Issue{}, Cloud: hostinventory.Cloud{Status: "not_detected"}})
		return &connectorv1.ConnectRequest{Frame: &connectorv1.ConnectRequest_Heartbeat{Heartbeat: &connectorv1.ConnectorHeartbeat{ConnectionEpoch: epoch,
			HostRuntime:  &connectorv1.HostRuntimeObservation{ObservedAt: timestamppb.New(at), Platform: "linux", OpensshStatus: "available", RdpStatus: "unavailable", InventoryJson: inventory},
			BastionRelay: &connectorv1.BastionRelayStatus{Status: "ready", Generation: epoch, AdvertiseAddress: "10.0.0.8", Listeners: []*connectorv1.BastionRelayListener{{Kind: "https", Port: 8447, Status: "ready"}, {Kind: "gateway", Port: 9448, Status: "ready"}}},
		}}}
	}
	if err = gateway.handleFrame(t.Context(), identity, 2, frame(2, "current", 0), nil, nil); err != nil {
		t.Fatal(err)
	}
	stale := frame(1, "stale", time.Millisecond)
	stale.GetHeartbeat().BastionRelay.AdvertiseAddress = "10.0.0.99"
	if err = gateway.handleFrame(t.Context(), identity, 1, stale, nil, nil); !errors.Is(err, ErrConnectorFenced) {
		t.Fatalf("old epoch was not fenced: %v", err)
	}
	assertCurrent := func() {
		t.Helper()
		var hostname, address string
		var generation int64
		if err := store.Pool.QueryRow(t.Context(), "SELECT o.inventory_json->>'hostname',b.relay_address,b.relay_port_generation FROM host_runtime_observations o JOIN bastion_scopes b ON b.enterprise_id=o.enterprise_id WHERE o.host_id=$1", h).Scan(&hostname, &address, &generation); err != nil {
			t.Fatal(err)
		}
		if hostname != "current" || address != "10.0.0.8" || generation != 2 {
			t.Errorf("rejected frame changed current projection: hostname=%s address=%s generation=%d", hostname, address, generation)
		}
	}
	assertCurrent()
	invalid := frame(2, "invalid-partial", 2*time.Millisecond)
	invalid.GetHeartbeat().BastionRelay.Listeners = nil
	if err = gateway.handleFrame(t.Context(), identity, 2, invalid, nil, nil); err == nil {
		t.Fatal("invalid relay frame accepted")
	}
	assertCurrent()
}
