package connector

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type collectorReconnectFixture struct {
	t                             *testing.T
	store                         *postgres.Store
	identity                      TrustedIdentity
	collector, collection, target uuid.UUID
	command                       string
}

func newCollectorReconnectFixture(t *testing.T) *collectorReconnectFixture {
	t.Helper()
	store := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "collector_replay", filepath.Join("..", "..", "migrations", "postgresql"))
	f := &collectorReconnectFixture{t: t, store: store, identity: TrustedIdentity{EnterpriseID: uuid.New(), ConnectorID: uuid.New(), SerialNumber: "01"}, collector: uuid.New(), collection: uuid.New(), target: uuid.New(), command: "receipt-" + uuid.NewString()}
	e, h, d, creator := f.identity.EnterpriseID, uuid.New(), uuid.New(), uuid.New()
	f.exec("INSERT INTO enterprises(id,name,code,timezone)VALUES($1,'Owned reconnect receipt',$2,'UTC')", e, e.String())
	if err := audit.InitializeChain(t.Context(), store.Queries, "enterprise", uuid.NullUUID{UUID: e, Valid: true}); err != nil {
		t.Fatal(err)
	}
	f.exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash)VALUES($1,$2,'Owned reconnect host','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", h, e)
	f.exec("INSERT INTO connectors(id,enterprise_id,role,name,host_id,instance_id,device_fingerprint_hash,public_key_hash,status,connection_epoch,certificate_expires_at)VALUES($1,$2,'host','Owned reconnect',$3,$4,decode(repeat('00',32),'hex'),decode(repeat('00',32),'hex'),'online',1,now()+interval '1 hour')", f.identity.ConnectorID, e, h, f.identity.ConnectorID.String())
	f.exec("UPDATE hosts SET connector_id=$2 WHERE id=$1", h, f.identity.ConnectorID)
	f.exec("INSERT INTO connector_certificates(id,connector_id,enterprise_id,serial_number,issuer_generation,certificate_request_name,certificate_pem,ca_bundle_pem,not_before,not_after)VALUES($1,$2,$3,'01',1,'owned-receipt','controlled fixture','controlled fixture',now()-interval '1 minute',now()+interval '1 hour')", uuid.New(), f.identity.ConnectorID, e)
	f.exec("INSERT INTO connector_sessions(connector_id,enterprise_id,gateway_instance_id,connection_epoch,capabilities,connected_at,last_heartbeat_at)VALUES($1,$2,'owned-receipt',1,'{}',now(),now())", f.identity.ConnectorID, e)
	f.exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision)VALUES($1,$2,'fixture','fixture','fixture','supported','[]',1)", d, d.String())
	f.exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,desired_revision,effective_revision)VALUES($1,$2,'host',$3,$4,'linux_amd64','leaf','installing',2,1)", f.collector, e, h, d)
	f.exec("INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by)VALUES($1,$2,'Owned reconnect target','probe','http','{}',$3)", f.target, e, creator)
	f.exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,effective_collector_id,desired_revision,effective_revision,assignment_epoch,effective_assignment_epoch,created_by,apply_status,desired_enabled)VALUES($1,$2,'Owned disabled collection','http','monitoring_target',$3,$4,$4,2,1,1,1,$5,'pending',false)", f.collection, e, f.target, f.collector, creator)
	f.exec("INSERT INTO monitoring_collection_assignments(collection_id,enterprise_id,epoch,collector_id,activated_at)VALUES($1,$2,1,$3,now())", f.collection, e, f.collector)
	f.config(1, true, "effective")
	f.config(2, false, "applying")
	payload, _ := protojson.Marshal(&connectorv1.CollectorManagementCommand{CollectorId: f.collector.String(), ResourceId: h.String(), ResourceType: "host", Operation: "configure", DesiredRevision: 2, Transport: "direct", ConfigSha256: strings.Repeat("00", 32)})
	f.exec("INSERT INTO connector_commands(id,command_id,enterprise_id,connector_id,connection_epoch,operation_ref,command_type,payload_schema_version,payload,payload_hash,idempotency_key,expires_at,status)VALUES($1,$2,$3,$4,1,$2,'collector_management','fixture',$5,decode(repeat('00',32),'hex'),$2,now()+interval '1 hour','running')", uuid.New(), f.command, e, f.identity.ConnectorID, payload)
	f.exec("INSERT INTO telemetry_routes(id,enterprise_id,collector_id,kind,transport,status)VALUES($1,$2,$3,'direct_argus','direct','active')", uuid.New(), e, f.collector)
	return f
}
func (f *collectorReconnectFixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.store.Pool.Exec(f.t.Context(), query, args...); err != nil {
		f.t.Fatal(err)
	}
}
func (f *collectorReconnectFixture) config(revision int64, enabled bool, status string) {
	f.t.Helper()
	bundle, _ := json.Marshal(configbundle.Bundle{SchemaVersion: configbundle.SchemaVersion, CollectorID: f.collector.String(), Collections: []configbundle.Collection{{ID: f.collection.String(), Integration: "http", TargetID: f.target.String(), TargetType: "monitoring_target", Revision: revision, AssignmentEpoch: 1, Enabled: enabled}}})
	f.exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status)VALUES($1,$2,$3,'{}',$4,decode(repeat('00',32),'hex'),$5)", uuid.New(), f.collector, revision, bundle, status)
}
func (f *collectorReconnectFixture) receipt(epoch int64) *connectorv1.CommandResult {
	value, _ := anypb.New(&connectorv1.CollectorManagementResult{CollectorId: f.collector.String(), EffectiveRevision: 2, AppliedConfigSha256: strings.Repeat("00", 32), Status: "converged"})
	raw, _ := proto.Marshal(value)
	digest := sha256.Sum256(raw)
	return &connectorv1.CommandResult{CommandId: f.command, ConnectionEpoch: uint64(epoch), Status: "succeeded", TypedResult: value, ResultHash: hex.EncodeToString(digest[:]), ResultSchemaVersion: "argus.connector_result/v1"}
}
func (f *collectorReconnectFixture) deliver(identity TrustedIdentity, epoch int64, receipt *connectorv1.CommandResult) error {
	return (Gateway{Service: Service{Store: f.store}}).handleFrame(f.t.Context(), identity, epoch, &connectorv1.ConnectRequest{Frame: &connectorv1.ConnectRequest_CommandResult{CommandResult: receipt}}, nil, nil)
}
func (f *collectorReconnectFixture) assertState(desired, effective int64, apply, health string) {
	f.t.Helper()
	var d, e int64
	var a, h string
	if err := f.store.Pool.QueryRow(f.t.Context(), "SELECT desired_revision,effective_revision,apply_status,health FROM monitoring_collections WHERE id=$1", f.collection).Scan(&d, &e, &a, &h); err != nil || d != desired || e != effective || a != apply || h != health {
		f.t.Fatalf("collection state got %d/%d %s/%s want %d/%d %s/%s err=%v", d, e, a, h, desired, effective, apply, health, err)
	}
}

func TestPostgresCollectorReconnectUsesVerifiedReceiptAndRejectsStaleOrForgedResults(t *testing.T) {
	for _, cleanup := range []bool{true, false} {
		t.Run(map[bool]string{true: "disconnect_projection", false: "missed_old_gateway_cleanup"}[cleanup], func(t *testing.T) {
			f := newCollectorReconnectFixture(t)
			service := Service{Store: f.store, GatewayInstance: "owned-receipt-new"}
			ctx := t.Context()
			if cleanup {
				if _, err := service.TransitionCommand(ctx, f.identity, 1, f.command, "result_unknown", nil, "CONNECTOR_DISCONNECTED"); err != nil {
					t.Fatal(err)
				}
				f.assertState(2, 1, "unknown", "unknown")
			}
			opened, err := service.OpenSession(ctx, f.identity, []string{"host.collector_manage"})
			if err != nil || opened.ConnectionEpoch != 2 {
				t.Fatal("current session did not open", err)
			}
			f.assertState(2, 1, "unknown", "unknown")
			if err = f.deliver(f.identity, 1, f.receipt(1)); err == nil {
				t.Fatal("stale connection settled the result")
			}
			bad := f.receipt(2)
			bad.ResultHash = strings.Repeat("f", 64)
			if err = f.deliver(f.identity, 2, bad); !errors.Is(err, ErrCommandState) {
				t.Fatal("corrupt saved receipt accepted", err)
			}
			other := f.identity
			other.EnterpriseID = uuid.New()
			if err = f.deliver(other, 2, f.receipt(2)); err == nil {
				t.Fatal("cross-enterprise receipt accepted")
			}
			bad = f.receipt(2)
			var outcome connectorv1.CollectorManagementResult
			_ = bad.TypedResult.UnmarshalTo(&outcome)
			outcome.CollectorId = uuid.NewString()
			bad.TypedResult, _ = anypb.New(&outcome)
			raw, _ := proto.Marshal(bad.TypedResult)
			hash := sha256.Sum256(raw)
			bad.ResultHash = hex.EncodeToString(hash[:])
			if err = f.deliver(f.identity, 2, bad); err == nil {
				t.Fatal("receipt changed its frozen Collector identity")
			}
			statusOnly := &connectorv1.ConnectRequest{Frame: &connectorv1.ConnectRequest_CommandReconcileResult{CommandReconcileResult: &connectorv1.CommandReconcileResult{ConnectionEpoch: 2, Commands: []*connectorv1.ReconciledCommand{{CommandId: f.command, Status: "succeeded"}}}}}
			if err = (Gateway{Service: service}).handleFrame(ctx, f.identity, 2, statusOnly, nil, nil); err != nil {
				t.Fatal(err)
			}
			f.assertState(2, 1, "unknown", "unknown")
			f.exec("CREATE FUNCTION fail_owned_receipt_projection() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'owned receipt projection failure';END $$")
			f.exec("CREATE TRIGGER fail_owned_receipt_projection BEFORE UPDATE ON monitoring_collections FOR EACH ROW EXECUTE FUNCTION fail_owned_receipt_projection()")
			if err = f.deliver(f.identity, 2, f.receipt(2)); err == nil {
				t.Fatal("partly committed receipt acknowledged")
			}
			var commandStatus string
			if err = f.store.Pool.QueryRow(ctx, "SELECT status FROM connector_commands WHERE command_id=$1", f.command).Scan(&commandStatus); err != nil || commandStatus != "result_unknown" {
				t.Fatal("projection failure committed command", commandStatus, err)
			}
			f.exec("DROP TRIGGER fail_owned_receipt_projection ON monitoring_collections")
			if err = f.deliver(f.identity, 2, f.receipt(2)); err != nil {
				t.Fatal(err)
			}
			f.assertState(2, 2, "effective", "stopped")
			var before, after time.Time
			if err = f.store.Pool.QueryRow(ctx, "SELECT updated_at FROM collector_instances WHERE id=$1", f.collector).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if err = f.deliver(f.identity, 2, f.receipt(2)); err != nil {
				t.Fatal("same typed receipt was not idempotent", err)
			}
			if err = f.store.Pool.QueryRow(ctx, "SELECT updated_at FROM collector_instances WHERE id=$1", f.collector).Scan(&after); err != nil || !before.Equal(after) {
				t.Fatal("same receipt changed effective state", err)
			}
		})
	}
}

func TestPostgresCollectorReconnectCannotOverwriteNewIntent(t *testing.T) {
	f := newCollectorReconnectFixture(t)
	service := Service{Store: f.store, GatewayInstance: "owned-receipt-new"}
	if _, err := service.OpenSession(t.Context(), f.identity, []string{"host.collector_manage"}); err != nil {
		t.Fatal(err)
	}
	f.config(3, true, "applying")
	f.exec("UPDATE collector_instances SET desired_revision=3,status='installing' WHERE id=$1", f.collector)
	f.exec("UPDATE monitoring_collections SET desired_revision=3,desired_enabled=true,apply_status='pending',health='unknown' WHERE id=$1", f.collection)
	if err := f.deliver(f.identity, 2, f.receipt(2)); err != nil {
		t.Fatal(err)
	}
	f.assertState(3, 1, "pending", "unknown")
	var effective int64
	if err := f.store.Pool.QueryRow(t.Context(), "SELECT effective_revision FROM collector_instances WHERE id=$1", f.collector).Scan(&effective); err != nil || effective != 1 {
		t.Fatal("old receipt replaced newer intent", effective, err)
	}
}
