package connector

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestPostgresGatewayMonitoringDispatchUsesConfiguredPrivateCredentialChannel(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx := t.Context()
	store, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	e, host, collector, distribution, target, collection, creator, connector, draft := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Owned credential dispatch',$2,'UTC')", e, e.String())
	if err = audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: e, Valid: true}); err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,'Credential dispatch fixture','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", host, e)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,'Dispatch fixture',$2,'fixture','fixture','supported','[]',1)", distribution, distribution.String())
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,desired_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','leaf','installing',1)", collector, e, host, distribution)
	exec("INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by) VALUES($1,$2,'Owned dispatch DB','database','mysql','{}',$3)", target, e, creator)
	exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,created_by) VALUES($1,$2,'Owned private receiver','mysql','monitoring_target',$3,$4,$5)", collection, e, target, collector, creator)
	keyPath := filepath.Join(t.TempDir(), "keyring.json")
	keyJSON, _ := json.Marshal(map[string]any{"current_version": 1, "keys": map[string]string{"1": base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32))}})
	if err = os.WriteFile(keyPath, keyJSON, 0600); err != nil {
		t.Fatal(err)
	}
	keyring, err := secret.LoadKeyring(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	credentials := secret.Service{Store: store, Keyring: keyring}
	private := []byte(`{"username":"monitor","password":"dispatch-private-canary"}`)
	defer clear(private)
	secretID, err := credentials.StageMonitoringWithQueries(ctx, store.Queries, e, creator, draft, private)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := credentials.BindMonitoringWithQueries(ctx, store.Queries, e, draft, secretID, collection)
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE monitoring_collections SET credential_id=$2 WHERE id=$1", collection, bound.ID)
	if _, err = store.Queries.SnapshotMonitoringCollection(ctx, db.SnapshotMonitoringCollectionParams{EnterpriseID: e, CollectionID: collection, ActorID: creator}); err != nil {
		t.Fatal(err)
	}
	bundle, _ := json.Marshal(configbundle.Bundle{SchemaVersion: configbundle.SchemaVersion, CollectorID: collector.String(), Collections: []configbundle.Collection{{ID: collection.String(), Integration: "mysql", TargetID: target.String(), TargetType: "monitoring_target", Revision: 1, AssignmentEpoch: 1, Enabled: true, HasCredential: true}}})
	exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}',$3,decode(repeat('00',32),'hex'),'applying')", uuid.New(), collector, bundle)
	payload, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(&connectorv1.CollectorManagementCommand{CollectorId: collector.String(), ResourceId: host.String(), ResourceType: "host", Operation: "configure", DesiredRevision: 1, RenderedConfig: bundle, Artifact: &connectorv1.CollectorArtifact{}})
	if err != nil {
		t.Fatal(err)
	}
	// This matches the executable's actual dependency boundary: execution
	// credentials live on Gateway; the control Service is not a fallback.
	gateway := Gateway{Service: Service{Store: store}, Credentials: credentials}
	command := db.ConnectorCommand{EnterpriseID: e, ConnectorID: connector, CommandID: "owned-dispatch", CommandType: "collector_management", OperationRef: "owned-dispatch", Payload: payload}
	frame, err := gateway.typedCommand(ctx, command)
	if err != nil {
		t.Fatal("configured execution-channel credentials were not used", err)
	}
	var delivered connectorv1.CollectorManagementCommand
	if frame.GetTypedPayload().UnmarshalTo(&delivered) != nil || !bytes.Contains(delivered.PrivateMonitoringConfig, []byte("dispatch-private-canary")) {
		t.Fatal("authorized private overlay missing from the execution frame")
	}
	if bytes.Contains(payload, []byte("dispatch-private-canary")) {
		t.Fatal("private overlay persisted in the public command")
	}
}
