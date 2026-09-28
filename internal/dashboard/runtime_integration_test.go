package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	actionservice "github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
)

type integrationRuntimeBackend struct {
	engine  *queryengine.Coordinator
	catalog telemetry.ClickHouseQuery
}

func (backend integrationRuntimeBackend) ExecuteEngineQuery(ctx context.Context, request queryengine.Request) (queryengine.Result, error) {
	return backend.engine.Execute(ctx, request)
}
func (backend integrationRuntimeBackend) DiscoverData(ctx context.Context, request telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
	return backend.catalog.DiscoverData(ctx, request)
}

func TestPublishedDashboardRunsAgainstRegisteredAuthorizedSources(t *testing.T) {
	url, address := os.Getenv("ARGUS_DASHBOARD_TEST_DATABASE_URL"), os.Getenv("ARGUS_CLICKHOUSE_TEST_ADDRESS")
	if url == "" || address == "" {
		t.Skip("PostgreSQL and ClickHouse integration endpoints are required")
	}
	ctx := context.Background()
	if err := postgres.RunMigrations(ctx, url, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	conn, err := telemetry.OpenClickHouse(address, "argus_telemetry", "argus", os.Getenv("ARGUS_CLICKHOUSE_TEST_PASSWORD"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	enterprise, department, user, role := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	host, otherHost, hiddenHost, distribution := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Runtime test',$2,'UTC')`, enterprise, "runtime-"+enterprise.String())
	exec(`INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Runtime')`, department, enterprise)
	exec(`INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Editor')`, user, enterprise, department, "runtime-"+user.String())
	exec(`INSERT INTO roles(id,enterprise_id,name,description,builtin) VALUES($1,$2,'Runtime editor','',false)`, role, enterprise)
	exec(`INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'telemetry.dashboard.read'),($1,'telemetry.dashboard.manage')`, role)
	exec(`INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)`, uuid.New(), enterprise, user, role)
	exec(`INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,$2,'test','0.133.0','argus.otelcol/v2','supported','[]',1)`, distribution, "runtime-"+distribution.String())
	var ownSource, ownCollector, ownGeneration, otherSource, hiddenSource uuid.UUID
	for _, id := range []uuid.UUID{host, otherHost, hiddenHost} {
		exec(`INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels_hash) VALUES($1,$2,$3,'127.0.0.1',22,'linux','managed_host','direct','development',decode(repeat('00',32),'hex'))`, id, enterprise, "host-"+id.String())
		collector, source, generation := uuid.New(), uuid.New(), uuid.New()
		if id == otherHost {
			otherSource = source
		}
		if id == hiddenHost {
			hiddenSource = source
		}
		if id == host {
			ownSource = source
			ownCollector, ownGeneration = collector, generation
		}
		exec(`INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,desired_revision,effective_revision,source_generation) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',1,1,$5)`, collector, enterprise, id, distribution, generation)
		exec(`INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,ARRAY[$3::uuid],'{}',decode(repeat('00',32),'hex'),'effective')`, uuid.New(), collector, uuid.New())
		exec(`INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version) VALUES($1,$2,$3,$4,'host',$5,'host/otlp','otlp',ARRAY['logs','traces'],1,decode(repeat('00',32),'hex'),'v1')`, source, enterprise, collector, generation, id)
	}
	exec(`INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)`, uuid.New(), enterprise, user, host)
	if err := audit.InitializeChain(ctx, store.Queries, "enterprise", nullID(enterprise)); err != nil {
		t.Fatal(err)
	}
	router := telemetry.TenantTableRouter{}
	manager := telemetry.ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, enterprise); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), enterprise) })
	tables, _ := router.Tables(enterprise)
	if err := conn.Exec(ctx, "INSERT INTO `"+tables.Logs+"` (resource_id,collector_id,timestamp,body,event_id,expires_at,source_id,source_revision,source_type) VALUES (?,?,?,'authorized evidence','event',now64(3)+INTERVAL 1 HOUR,?,1,'otlp')", host, uuid.New(), time.Now().UTC().Add(-time.Minute), ownSource); err != nil {
		t.Fatal(err)
	}
	backend := integrationRuntimeBackend{engine: &queryengine.Coordinator{Cache: queryengine.NewResultCache(8<<20, time.Minute), PromQL: queryengine.PromQLEngine{Engine: promqlengine.NewEngine(conn, router, nil)}, KQL: queryengine.KQLEngine{Conn: conn, Router: router}, Trace: queryengine.TraceEngine{Engine: skywalking.Engine{Conn: conn, Router: router}}}, catalog: telemetry.ClickHouseQuery{Conn: conn, Router: router}}
	runtime := Runtime{Store: store, Backend: backend}
	key := bytes.Repeat([]byte{6}, 32)
	runtime.ContextKey = key
	service := Service{Store: store, Verifier: runtime, Actions: resource.PendingActionService{Store: store, Key: key, Idempotency: postgres.Idempotency{Key: key}}}
	actor := Actor{EnterpriseID: enterprise, SubjectID: user, SubjectType: "user", AuthorizationVersion: 1}
	spec := EmptySpec()
	spec.Panels = []Panel{{ID: "logs", Title: "Logs", Signal: "logs", Type: "logs", AuthoringMode: "dsl", SourceBinding: SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, ApplicableResourceTypes: []string{"host"}, Layout: Rectangle{W: 12, H: 10, MinW: 2, MinH: 2}, Targets: []Target{{ID: "a", Language: queryengine.LanguageKQL, SourceDefinition: Definition{DSL: &DSL{Expression: "* | limit 5"}}}}}}
	encoded, _ := json.Marshal(spec)
	draft, err := service.CreateDraft(ctx, actor, DraftInput{Name: "Source-aware logs", Spec: encoded})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewPublish(ctx, actor, draft.ID, draft.DraftVersion, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Validation.Valid {
		t.Fatal(preview.Validation.Issues)
	}
	extension := ActionExtension{}
	workflow := actionservice.Service{Store: store, Idempotency: postgres.Idempotency{Key: key}, Resources: resource.Service{Store: store, Actions: service.Actions, Extension: extension}}
	confirmation, err := workflow.Confirm(ctx, user.String(), uuid.NewString(), enterprise, 1, false, preview.Action.ActionRef, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	var published resource.ActionCommitResult
	if err := store.InTx(ctx, func(q *db.Queries) error {
		var e error
		published, e = service.Actions.ExecuteReady(ctx, q, confirmation.PendingAction, extension.RevalidateAction, extension.CommitAction)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Execute(ctx, actor, published.ResourceID, ExecutionInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.RevisionID == uuid.Nil || result.ExecutionHash == "" || len(result.Resources) != 1 || result.Resources[0].ID != host || len(result.Panels) != 1 || result.Panels[0].Status != "success" || len(result.Panels[0].Sources) != 1 || result.Panels[0].Sources[0].ID != ownSource {
		t.Fatalf("runtime lost frozen scope: %+v", result)
	}
	// Same frozen published query reuses projected data, with original timestamps.
	fixedInput := ExecutionInput{From: &result.From, To: &result.To, ResourceIDs: []uuid.UUID{host}}
	cached, err := runtime.Execute(ctx, actor, published.ResourceID, fixedInput)
	if err != nil || !cached.Panels[0].Targets[0].Meta.CacheHit {
		t.Fatalf("published cache miss: %+v %v", cached, err)
	}
	if cached.Panels[0].Targets[0].Meta.LatestSampleAt == nil || cached.Panels[0].Targets[0].Meta.IngestionStatus != "unknown" {
		t.Fatal("original log sample freshness missing")
	}
	t.Run("cache source changes", func(t *testing.T) {
		added := uuid.New()
		exec(`INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version) SELECT $1,enterprise_id,collector_id,generation,resource_type,resource_id,source_key||'/new',source_type,signals,config_revision,config_hash,capability_version FROM telemetry_sources WHERE id=$2`, added, ownSource)
		changed, e := runtime.Execute(ctx, actor, published.ResourceID, fixedInput)
		if e != nil || len(changed.Panels[0].Sources) != 2 || changed.Panels[0].Targets[0].Meta.CacheHit {
			t.Fatalf("new dynamic source reused old cache: %v", e)
		}
		exec(`DELETE FROM telemetry_sources WHERE id=$1 AND enterprise_id=$2`, added, enterprise)
	})
	if _, err := runtime.Execute(ctx, actor, published.ResourceID, ExecutionInput{ResourceIDs: []uuid.UUID{otherHost}}); !errors.Is(err, ErrDenied) {
		t.Fatalf("unauthorized resource was accepted: %v", err)
	}
	t.Run("parameterized published queries", func(t *testing.T) {
		testPublishedParameters(t, ctx, service, runtime, actor, workflow, spec, host, ownSource)
	})
	t.Run("APM published sample contract", func(t *testing.T) {
		testPublishedAPM(t, ctx, service, runtime, actor, workflow, conn, tables.Traces, host, ownSource)
	})
	t.Run("published drilldown authorization", func(t *testing.T) {
		testPublishedDrilldowns(t, ctx, service, runtime, actor, workflow, conn, tables, host, otherHost, hiddenHost, ownSource, otherSource, hiddenSource)
	})
	t.Run("lossless conversion publication", func(t *testing.T) {
		testPublishedConversion(t, ctx, service, runtime, actor, workflow, conn, tables, host, ownSource)
	})
	t.Run("persistent query files", func(t *testing.T) {
		testPublishedQueryJobs(t, ctx, runtime, actor, published.ResourceID, host, ownSource, conn, tables.Logs)
	})
	data, err := runtime.Catalog(ctx, actor, CatalogInput{SourceBinding: SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, Signal: "logs", Kind: "values", Field: "body", Search: "not matching", SelectedValues: []string{"authorized evidence", "missing"}, From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Minute), Limit: 1})
	if err != nil || len(data.Values) != 0 || !data.Membership["authorized evidence"] || data.Membership["missing"] {
		t.Fatalf("search was confused with selected value absence: %+v %v", data, err)
	}
	control := telemetry.PostgresIngestControl{Queries: store.Queries}
	fields, err := runtime.Catalog(ctx, actor, CatalogInput{SourceBinding: SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, Signal: "logs", Kind: "fields", From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Minute), Limit: 100})
	if err != nil || len(fields.Fields) == 0 || !fields.Complete {
		t.Fatalf("authorized field catalog: %+v %v", fields, err)
	}
	producer := telemetry.TrustedIdentity{EnterpriseID: enterprise, ResourceID: host, ResourceType: "host", CollectorID: ownCollector}
	if _, err := control.ResolveSource(ctx, producer, ownSource, 1, "logs"); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE collector_instances SET status='uninstalled' WHERE id=$1`, ownCollector)
	reinstalled, err := store.Queries.UpsertCollectorForAction(ctx, db.UpsertCollectorForActionParams{ID: uuid.New(), EnterpriseID: enterprise, ResourceType: "host", ResourceID: host, DistributionVersionID: distribution, Platform: "linux_amd64", Role: "direct"})
	if err != nil {
		t.Fatal(err)
	}
	if reinstalled.ID != ownCollector || reinstalled.SourceGeneration == ownGeneration {
		t.Fatal("reinstall did not separate the new source generation")
	}
	if _, err := control.ResolveSource(ctx, producer, ownSource, 1, "logs"); err == nil {
		t.Fatal("new installation accepted a source from its previous generation")
	}
	if history, err := runtime.Execute(ctx, actor, published.ResourceID, ExecutionInput{}); err != nil || history.Panels[0].Status != "success" {
		t.Fatalf("reinstallation removed historical data: %+v %v", history, err)
	}
	// Permission changes are rechecked rather than trusting an old execution.
	exec(`UPDATE data_authorization_grants SET status='disabled' WHERE enterprise_id=$1 AND resource_type='host'`, enterprise)
	if _, err := runtime.Catalog(ctx, actor, CatalogInput{SourceBinding: SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, Signal: "logs", Kind: "fields", ResourceIDs: []uuid.UUID{host}, From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Minute), Limit: 100}); !errors.Is(err, ErrDenied) {
		t.Fatalf("revoked field catalog scope accepted: %v", err)
	}
	if _, err := runtime.Execute(ctx, actor, published.ResourceID, fixedInput); !errors.Is(err, ErrDenied) {
		t.Fatalf("revoked resource was accepted: %v", err)
	}
	// An expired role must not keep granting objects when another active role
	// still grants the dashboard feature itself.
	objectRole, binding := uuid.New(), uuid.New()
	exec(`INSERT INTO roles(id,enterprise_id,name,description,builtin) VALUES($1,$2,'Object reader','',false)`, objectRole, enterprise)
	exec(`INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id,valid_from,valid_until) VALUES($1,$2,'user',$3,$4,now()-INTERVAL '1 hour',now()+INTERVAL '1 hour')`, binding, enterprise, user, objectRole)
	exec(`INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'role',$3,'host',$4)`, uuid.New(), enterprise, objectRole, host)
	if _, err := runtime.Execute(ctx, actor, published.ResourceID, ExecutionInput{ResourceIDs: []uuid.UUID{host}}); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE role_bindings SET valid_until=now()-INTERVAL '1 minute' WHERE id=$1`, binding)
	if _, err := runtime.Execute(ctx, actor, published.ResourceID, ExecutionInput{ResourceIDs: []uuid.UUID{host}}); !errors.Is(err, ErrDenied) {
		t.Fatalf("expired role retained object access: %v", err)
	}
}
