package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
)

// A second stage consumes the owned tenant left by the telemetry native metric
// gate. This executes the production dashboard runtime and Data Catalog against
// real stored receiver observations; PostgreSQL setup remains a controlled
// authorization fixture rather than a claim of deployed platform acceptance.
func TestPostgresClickHouseManagedNativeTemplateViews(t *testing.T) {
	path := os.Getenv("ARGUS_MONITORING_NATIVE_VIEW_RECEIPT")
	database, address := os.Getenv("ARGUS_DASHBOARD_TEST_DATABASE_URL"), os.Getenv("ARGUS_CLICKHOUSE_TEST_ADDRESS")
	if path == "" || database == "" || address == "" {
		t.Skip("requires owned native metric receipt, PostgreSQL and ClickHouse")
	}
	var receipt struct {
		Owner        string                `json:"owner"`
		EnterpriseID uuid.UUID             `json:"enterprise_id"`
		CapturedAt   time.Time             `json:"captured_at"`
		CollectorID  uuid.UUID             `json:"collector_id"`
		HostID       uuid.UUID             `json:"host_id"`
		Sources      []configbundle.Source `json:"sources"`
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.EnterpriseID == uuid.Nil || receipt.Owner != "plan8-native-template-view/"+receipt.EnterpriseID.String() || time.Since(receipt.CapturedAt) > 45*time.Minute || len(receipt.Sources) != 4 {
		t.Fatal("invalid or stale owned native receipt")
	}
	ctx := t.Context()
	if err = postgres.RunMigrations(ctx, database, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	conn, err := telemetry.OpenClickHouse(address, "argus_telemetry", "argus", os.Getenv("ARGUS_CLICKHOUSE_TEST_PASSWORD"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	e, user, department, role, distribution := receipt.EnterpriseID, uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	ownedCode := "nativeview-" + e.String()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Native template view',$2,'UTC')", e, ownedCode)
	t.Cleanup(func() {
		var code string
		if err := store.Pool.QueryRow(context.Background(), "SELECT code FROM enterprises WHERE id=$1", e).Scan(&code); err != nil || code != ownedCode {
			t.Error("owned native tenant cleanup refused")
			return
		}
		if err := (telemetry.ClickHouseTenantSchemaManager{Conn: conn, Router: telemetry.TenantTableRouter{}}).DropTenant(context.Background(), e); err != nil {
			t.Error(err)
		}
	})
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Native view')", department, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Native view')", user, e, department, user.String())
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Native view')", role, e)
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'telemetry.dashboard.read')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), e, user, role)
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,environment,platform,role,control_path,labels_hash) VALUES($1,$2,'Captured Collector','127.0.0.1',22,'development','linux','managed_host','direct',decode(repeat('00',32),'hex'))", receipt.HostID, e)
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,$2,'test',$3,'argus.otelcol/v3','supported','[]',1)", distribution, distribution.String(), configbundle.CollectorVersion)
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", receipt.CollectorID, e, receipt.HostID, distribution, uuid.MustParse(receipt.Sources[0].Generation))
	exec("INSERT INTO collector_config_revisions(id,collector_id,revision,profile_ids,rendered_config,config_hash,status) VALUES($1,$2,1,'{}','{}',decode(repeat('00',32),'hex'),'effective')", uuid.New(), receipt.CollectorID)
	if err = audit.InitializeChain(ctx, store.Queries, "enterprise", nullID(e)); err != nil {
		t.Fatal(err)
	}
	for _, source := range receipt.Sources {
		target, collection := uuid.MustParse(source.ResourceID), uuid.MustParse(source.CollectionID)
		entry, ok := monitoringcatalog.Lookup(source.Type)
		if !ok {
			t.Fatal("native target catalog entry missing")
		}
		exec("INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by) VALUES($1,$2,$3,$4,$3,'{}',$5)", target, e, source.Type, entry.Family, user)
		exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,created_by) VALUES($1,$2,$3,$3,'monitoring_target',$4,$5,$6)", collection, e, source.Type, target, receipt.CollectorID, user)
		if _, err = store.Queries.SnapshotMonitoringCollection(ctx, db.SnapshotMonitoringCollectionParams{EnterpriseID: e, CollectionID: collection, ActorID: user}); err != nil {
			t.Fatal(err)
		}
		exec("INSERT INTO monitoring_collection_assignments(collection_id,enterprise_id,epoch,collector_id) VALUES($1,$2,1,$3)", collection, e, receipt.CollectorID)
		exec("INSERT INTO telemetry_sources(id,enterprise_id,collector_id,generation,resource_type,resource_id,source_key,source_type,signals,config_revision,config_hash,capability_version,collection_id,collection_revision,assignment_epoch) VALUES($1,$2,$3,$4,'monitoring_target',$5,$6,$7,ARRAY['metrics'],1,decode(repeat('00',32),'hex'),'v1',$8,1,1)", uuid.MustParse(source.ID), e, receipt.CollectorID, uuid.MustParse(source.Generation), target, source.Key, source.Type, collection)
	}
	router := telemetry.TenantTableRouter{}
	backend := integrationRuntimeBackend{engine: &queryengine.Coordinator{PromQL: queryengine.PromQLEngine{Engine: promqlengine.NewEngine(conn, router, nil)}}, catalog: telemetry.ClickHouseQuery{Conn: conn, Router: router}}
	runtime := Runtime{Store: store, Backend: backend, ContextKey: make([]byte, 32)}
	service := Service{Store: store}
	actor := Actor{EnterpriseID: e, SubjectType: "user", SubjectID: user, AuthorizationVersion: 1}
	from, to := receipt.CapturedAt.Add(-time.Minute), receipt.CapturedAt.Add(time.Second)
	definitions := BuiltinTemplates()
	for i := range definitions {
		definitions[i].Key = "test.native." + uuid.NewString()
	}
	if err = service.SyncTemplates(ctx, definitions); err != nil {
		t.Fatal(err)
	}
	panels := 0
	for _, source := range receipt.Sources {
		target := uuid.MustParse(source.ResourceID)
		var definition TemplateDefinition
		for _, item := range definitions {
			if item.IntegrationKeys[0] == source.Type {
				definition = item
			}
		}
		managed, err := service.ApplyManagedTemplate(ctx, e, definition.Key)
		if err != nil {
			t.Fatal(err)
		}
		input := ExecutionInput{ResourceMode: "explicit", ResourceTypes: []string{"monitoring_target"}, ResourceIDs: []uuid.UUID{target}, From: &from, To: &to}
		if _, err = runtime.Execute(ctx, actor, managed.ID, input); !errors.Is(err, ErrDenied) {
			t.Fatalf("ungranted native target readable: %v", err)
		}
		exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'monitoring_target',$4)", uuid.New(), e, user, target)
		result, err := runtime.Execute(ctx, actor, managed.ID, input)
		if err != nil {
			t.Fatal(err)
		}
		for _, panel := range result.Panels {
			if panel.Status != "success" || len(panel.Sources) != 1 || panel.Sources[0].ID != uuid.MustParse(source.ID) || len(panel.Targets) != 1 || panel.Targets[0].Status != "success" {
				t.Fatalf("native template %s/%s: %+v", source.Type, panel.ID, panel)
			}
			panels++
		}
		// A relation created after collection exposes the same historical resource
		// observations; two applications do not duplicate a shared target.
		apps := []uuid.UUID{uuid.New(), uuid.New()}
		for _, app := range apps {
			exec("INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by) VALUES($1,$2,$3,'Linked after capture','production',$4)", app, e, app.String(), user)
			exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'application',$4)", uuid.New(), e, user, app)
			exec("INSERT INTO monitoring_application_bindings(id,enterprise_id,application_id,target_type,target_id) VALUES($1,$2,$3,'monitoring_target',$4)", uuid.New(), e, app, target)
		}
		input.ApplicationMode, input.ApplicationIDs = "explicit", apps
		linked, err := runtime.Execute(ctx, actor, managed.ID, input)
		if err != nil || len(linked.Resources) != 1 || len(linked.Panels) != len(result.Panels) {
			t.Fatalf("historical shared target: %v", err)
		}
		for i, panel := range linked.Panels {
			if panel.Status != "success" || len(panel.Sources) != 1 || panel.Targets[0].Meta.ReturnedRows != result.Panels[i].Targets[0].Meta.ReturnedRows {
				t.Fatalf("linked resource doubled or lost native samples: %+v", panel)
			}
		}
		input.ResourceIDs = []uuid.UUID{}
		empty, err := runtime.Execute(ctx, actor, managed.ID, input)
		if err != nil || !empty.ScopeEmpty {
			t.Fatalf("explicit empty target widened: %+v %v", empty, err)
		}
	}
	if panels != 22 {
		t.Fatalf("real dashboard panel coverage=%d", panels)
	}
	t.Logf("native dashboard views passed: panels=%d, targets=%d; source and explicit-empty isolation verified", panels, len(receipt.Sources))
}
