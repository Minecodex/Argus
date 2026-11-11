package directexecutor

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/audit"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/testsupport/postgrestest"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestPostgresCollectorOutcomeRetriesSerializationBeforeAdmittingSources(t *testing.T) {
	store := postgrestest.New(t, "ARGUS_MONITORING_TEST_DATABASE_URL", "outcome_retry", filepath.Join("..", "..", "migrations", "postgresql"))
	ctx := t.Context()
	enterprise, creator, host, distribution, collector, generation, action, operation, source := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("INSERT INTO enterprises(id,name,timezone) VALUES($1,'Owned outcome retry','UTC')", enterprise)
	if err := audit.InitializeChain(ctx, store.Queries, "enterprise", uuid.NullUUID{UUID: enterprise, Valid: true}); err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,'Outcome fixture','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", host, enterprise)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,$2,'fixture','fixture','fixture','supported','[]',1)", distribution, distribution.String())
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,desired_revision,effective_revision,source_generation) VALUES($1,$2,'host',$3,$4,'linux_amd64','leaf','installing',2,1,$5)", collector, enterprise, host, distribution, generation)
	bundle, _ := json.Marshal(configbundle.Bundle{SchemaVersion: configbundle.SchemaVersion, CollectorID: collector.String(), Collections: []configbundle.Collection{}})
	for revision := 1; revision <= 2; revision++ {
		state := "effective"
		if revision == 2 {
			state = "applying"
		}
		exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,$3,'{}',$4,decode(repeat('00',32),'hex'),$5)", uuid.New(), collector, revision, bundle, state)
	}
	exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version) VALUES($1,$2,$3,$4,'host',$5,'host/prometheus/collector_self','prometheus',ARRAY['metrics'],2,decode(repeat('00',32),'hex'),'fixture')", source, enterprise, collector, generation, host)
	exec("INSERT INTO telemetry_routes(id,enterprise_id,collector_id,kind,transport,status) VALUES($1,$2,$3,'direct_argus','direct','active')", uuid.New(), enterprise, collector)
	exec("INSERT INTO pending_actions(id,action_ref,enterprise_id,creator_subject_id,authorization_version,action_type,title,summary,risk,preview,status,resource_type,resource_id,impact_hash,expires_at) VALUES($1,$2,$3,$4,1,'collector.configure','Owned outcome','Owned outcome','write','{}','succeeded','collector_instance',$5,decode(repeat('00',32),'hex'),now()+interval '1 hour')", action, action.String(), enterprise, creator, collector)
	plan, _ := protojson.Marshal(&connectorv1.CollectorManagementCommand{CollectorId: collector.String(), Operation: "configure", DesiredRevision: 2})
	exec("INSERT INTO telemetry_collector_operations(id,enterprise_id,collector_id,pending_action_id,operation,executor_kind,status,plan,plan_hash,lease_owner,fence,lease_expires_at,expires_at) VALUES($1,$2,$3,$4,'configure','direct','running',$5,decode(repeat('00',32),'hex'),'owned-worker',1,now()+interval '1 hour',now()+interval '1 hour')", operation, enterprise, collector, action, plan)
	row, err := store.Queries.GetTelemetryCollectorOperation(ctx, db.GetTelemetryCollectorOperationParams{ID: operation, EnterpriseID: enterprise})
	if err != nil {
		t.Fatal(err)
	}
	params := db.ResolveTelemetrySourceParams{ID: source, ConfigRevision: 2, CollectorID: collector, EnterpriseID: enterprise}
	if _, err = store.Queries.ResolveTelemetrySource(ctx, params); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("unapplied source admitted", err)
	}
	// nextval is deliberately nontransactional: the first actual PostgreSQL
	// transaction aborts after its command row write, and the next must replay
	// only that database projection, not the already completed runtime action.
	exec("CREATE SEQUENCE owned_outcome_attempts")
	exec("CREATE FUNCTION owned_fail_first_outcome() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF nextval('owned_outcome_attempts')=1 THEN RAISE EXCEPTION 'owned concurrent outcome' USING ERRCODE='40001'; END IF; RETURN NEW; END $$")
	exec("CREATE TRIGGER owned_outcome_abort BEFORE UPDATE ON collector_instances FOR EACH ROW EXECUTE FUNCTION owned_fail_first_outcome()")
	(&Executor{Store: store}).finishCollectorOperation(ctx, row, "succeeded", nil, "")
	var attempts, revision, version int64
	var operationStatus, collectorStatus, configStatus string
	if err = store.Pool.QueryRow(ctx, "SELECT (SELECT last_value FROM owned_outcome_attempts),o.status,c.status,c.effective_revision,c.version,r.status FROM telemetry_collector_operations o JOIN collector_instances c ON c.id=o.collector_id JOIN collector_config_revisions r ON r.collector_id=c.id AND r.revision=2 WHERE o.id=$1", operation).Scan(&attempts, &operationStatus, &collectorStatus, &revision, &version, &configStatus); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || operationStatus != "succeeded" || collectorStatus != "converged" || revision != 2 || version != 2 || configStatus != "effective" {
		t.Fatal("known successful outcome was lost or partially committed", attempts, operationStatus, collectorStatus, revision, version, configStatus)
	}
	if _, err = store.Queries.ResolveTelemetrySource(ctx, params); err != nil {
		t.Fatal("verified source remained inadmissible", err)
	}
	t.Log("actual 40001 rollback retried; command/config/source cutover committed once without runtime reexecution")
}
