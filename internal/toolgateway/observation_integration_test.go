package toolgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/resourceref"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestPostgresObservationDiscoveryScopeAndCapabilityRevocation(t *testing.T) {
	address := os.Getenv("ARGUS_PLAN11_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires owned isolated Plan11 PostgreSQL")
	}
	ctx := t.Context()
	if err := postgres.RunMigrations(ctx, address, filepath.Join("..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.InReadCommittedTx(ctx, func(q *db.Queries) error { _, e := authorization.SyncPermissionCatalog(ctx, q); return e }); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := store.Pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	enterprise, department, user, role := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Plan11 discovery',$2,'UTC')", enterprise, enterprise.String())
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Readers')", department, enterprise)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Reader')", user, enterprise, department, user.String())
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Plan11 reader')", role, enterprise)
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'monitoring.read'),($1,'application.read')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), enterprise, user, role)
	targets := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	for i, id := range targets {
		engine, kind := "mysql", "database"
		if i == 2 {
			engine, kind = "redis", "cache"
		}
		exec("INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by) VALUES($1,$2,'same-name',$3,$4,'{}',$5)", id, enterprise, kind, engine, user)
		if i != 3 {
			exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'monitoring_target',$4)", uuid.New(), enterprise, user, id)
		}
	}
	app := uuid.New()
	exec("INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by) VALUES($1,$2,$3,'orders','production',$4)", app, enterprise, app.String(), user)
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'application',$4)", uuid.New(), enterprise, user, app)
	actor := authorization.Actor{EnterpriseID: enterprise, SubjectID: user, SubjectType: "user", AuthorizationVersion: 1}
	key := bytes.Repeat([]byte{11}, 32)
	resolver := observationscope.Resolver{Queries: store.Queries}
	exec("INSERT INTO monitoring_ingest_endpoints(id,enterprise_id,application_id,name,protocol,signals,created_by) VALUES($1,$2,$3,'SDK','otlp',ARRAY['metrics','logs'],$4)", uuid.New(), enterprise, app, user)
	apps, err := resolver.Discover(ctx, actor, key, observationscope.DiscoveryInput{ResourceTypes: []string{"application"}, Families: []string{"ingestion"}, Integrations: []string{"otlp"}})
	if err != nil || len(apps.Items) != 1 || apps.Items[0].ID != app {
		t.Fatalf("SDK integration discovery: %+v %v", apps, err)
	}
	input := observationscope.DiscoveryInput{ResourceTypes: []string{"monitoring_target"}, Families: []string{"database"}, Integrations: []string{"mysql", "redis"}, Search: "same-name", Limit: 1}
	firstInput := input
	first, err := resolver.Discover(ctx, actor, key, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Complete || first.NextCursor == "" || first.Items[0].Ref.ID == targets[3] || first.Items[0].Family != "database" {
		t.Fatalf("first filtered page: %+v", first)
	}
	input.Cursor = first.NextCursor
	second, err := resolver.Discover(ctx, actor, key, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || !second.Complete || second.Items[0].Ref.ID == first.Items[0].Ref.ID {
		t.Fatalf("second page: %+v", second)
	}
	for _, page := range []observationscope.DiscoveryPage{first, second} {
		item := page.Items[0]
		if item.Ref.ID == targets[3] || item.Family != "database" || item.Name != "same-name" || len(item.Integrations) != 1 || item.Integrations[0] != "mysql" {
			t.Fatal("paging leaked a hidden, nonmatching name/family/integration resource")
		}
	}
	nameChanged := input
	nameChanged.Search = "other-name"
	if _, err = resolver.Discover(ctx, actor, key, nameChanged); err == nil {
		t.Fatal("cursor accepted with changed name search")
	}
	input.Integrations = []string{"redis"}
	if _, err = resolver.Discover(ctx, actor, key, input); err == nil {
		t.Fatal("cursor accepted with changed filters")
	}
	input.Cursor = ""
	empty, err := resolver.Discover(ctx, actor, key, input)
	if err != nil || len(empty.Items) != 0 {
		t.Fatal("empty filter intersection expanded", err)
	}
	writeDiscoveryFilterEvidence(t, firstInput, first, second, input, empty)
	reg := mcp.NewRegistry()
	observe := ObservationTools{Base: ResourceTools{Store: store}, Monitoring: monitoring.Service{Store: store, CursorKey: key}, Telemetry: telemetry.Service{Store: store}, Dashboard: dashboard.Service{Store: store, ProvideDefaults: true}, CursorKey: key}
	if err = observe.Register(reg); err != nil {
		t.Fatal(err)
	}
	gateway, err := New(reg, "plan11-postgres")
	if err != nil {
		t.Fatal(err)
	}
	principal := toolruntime.Principal{EnterpriseID: enterprise, UserID: user, AuthorizationVersion: 1, Permissions: []string{"monitoring.read", "application.read"}}
	call := toolruntime.Invocation{Principal: principal, Arguments: map[string]any{"category": "observe"}}
	search, err := gateway.CoreTools()[0].Invoke(ctx, call)
	if err != nil || len(search.Data["items"].([]map[string]any)) != 3 {
		t.Fatal("observe discovery", err)
	}
	native := mcp.Call{Enterprise: enterprise.String(), Subject: user.String(), SubjectType: "user", ToolID: "observe.capabilities.get", Input: map[string]any{"resources": []any{map[string]any{"type": "monitoring_target", "id": targets[0].String()}}}}
	result, err := reg.Call(ctx, native)
	if err != nil {
		t.Fatal(err)
	}
	if sampled, ok := result.Structured["sampling_performed"].(bool); !ok || sampled || result.Structured["data_state"] != "not_checked" || !strings.Contains(result.Structured["data_state_reason"].(string), "this request's activity") {
		t.Fatal("metadata-only request omitted its own explicit sampling activity or confused it with collection state")
	}
	rows := result.Structured["items"].([]any)
	sections := rows[0].(map[string]any)["sections"].(map[string]any)
	if sections["dashboards"].(map[string]any)["status"] != "not_permitted" {
		t.Fatal("dashboard permission not separated")
	}
	if sections["sources"].(map[string]any)["status"] != "available" {
		t.Fatal("source absence mislabeled")
	}
	verifyCollectionMetadataActivityAndSDKCoverage(t, ctx, store, actor, targets[0], app, reg)
	raw, _ := json.Marshal(result.Structured)
	if strings.Contains(string(raw), "identity_config") || strings.Contains(string(raw), "credential") {
		t.Fatal("private config leaked")
	}
	bad := native
	bad.Input = map[string]any{"resources": []any{map[string]any{"type": "monitoring_target", "id": targets[3].String()}}}
	if _, err = reg.Call(ctx, bad); err == nil {
		t.Fatal("ungranted target returned capabilities")
	}
	bad.Input = map[string]any{"resources": []any{map[string]any{"type": "application", "id": targets[0].String()}}}
	if _, err = reg.Call(ctx, bad); err == nil {
		t.Fatal("cross-type ID accepted")
	}
	verifyCapabilityBackendFailureIsPartial(t, address, store, actor, observe, role, targets[0], targets[3])
	exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND resource_id=$2", enterprise, targets[0])
	if _, err = reg.Call(ctx, native); err == nil {
		t.Fatal("revoked capability still readable")
	}
	_, err = resolver.Sources(context.Background(), actor, observationscope.SourceSelection{Resources: []resourceref.Ref{{Type: "monitoring_target", ID: targets[0]}}, Signal: "catalog"})
	if err == nil {
		t.Fatal("revoked source scope accepted")
	}
}
