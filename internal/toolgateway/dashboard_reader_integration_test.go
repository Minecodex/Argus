package toolgateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/modelmetrics"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

// Run against the existing current-schema fixture, with an actual owned Run.
// This exercises the native adapter used by Worker, not a paid model response.
func assertReadOnlyDefaultDashboardParity(t *testing.T, store *postgres.Store, service dashboard.Service, enterprise, user, role, run uuid.UUID) {
	t.Helper()
	ctx := t.Context()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("DELETE FROM role_permissions WHERE role_id=$1 AND permission_id='telemetry.dashboard.manage'", role)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := store.Pool.Exec(cleanup, "INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'telemetry.dashboard.manage') ON CONFLICT DO NOTHING", role); err != nil {
			t.Error("restore fixture management permission", err)
		}
	})
	host, collector, distribution, grant := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO collector_distribution_versions(id,name,version,collector_version,config_schema_version,support_status,artifact_manifest,catalog_revision) VALUES($1,'Read-only default',$2,'fixture','v1','supported','[]',1)", distribution, distribution.String())
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels_hash) VALUES($1,$2,'Read-only Host','127.0.0.1',22,'linux','managed_host','direct','development',decode(repeat('00',32),'hex'))", host, enterprise)
	exec("INSERT INTO collector_instances(id,enterprise_id,resource_type,resource_id,distribution_version_id,platform,role,status,source_generation,desired_revision,effective_revision) VALUES($1,$2,'host',$3,$4,'linux_amd64','direct','converged',$5,1,1)", collector, enterprise, host, distribution, uuid.New())
	exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,target_type,target_id,collector_id,created_by) VALUES($1,$2,'Read-only processes','host_process_metrics','host',$3,$4,$5)", uuid.New(), enterprise, host, collector, user)
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", grant, enterprise, user, host)
	private := uuid.New()
	exec("INSERT INTO dashboards(id,enterprise_id,name,created_by,updated_by) VALUES($1,$2,'Private reader dashboard',$3,$3)", private, enterprise, user)
	service.ProvideDefaults = true
	if err := service.SyncTemplates(ctx, dashboard.BuiltinTemplates()); err != nil {
		t.Fatal(err)
	}
	registry := mcp.NewRegistry()
	if err := (DashboardTools{Service: service}).Register(registry); err != nil {
		t.Fatal(err)
	}
	actor := dashboard.Actor{EnterpriseID: enterprise, SubjectID: user, SubjectType: "user", AuthorizationVersion: 1}
	call := mcp.Call{Enterprise: enterprise.String(), Subject: user.String(), SubjectType: "user", Caller: "model", RunID: run.String(), InvocationID: uuid.NewString(), Input: map[string]any{}}
	listNative := func() map[uuid.UUID]string {
		t.Helper()
		call.ToolID, call.Input = "telemetry.dashboard.list", map[string]any{}
		result, err := registry.Call(ctx, call)
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Items []struct {
				ID   uuid.UUID `json:"id"`
				Name string    `json:"name"`
			} `json:"items"`
		}
		raw, err := json.Marshal(result.Structured)
		if err != nil || json.Unmarshal(raw, &page) != nil {
			t.Fatal("native directory result unavailable", err)
		}
		items := map[uuid.UUID]string{}
		for _, item := range page.Items {
			items[item.ID] = item.Name
		}
		return items
	}
	assertParity := func(wantDefault bool) uuid.UUID {
		t.Helper()
		native := listNative()
		portal, err := service.List(dashboard.Interactive(ctx), actor)
		if err != nil || len(native) != len(portal) {
			t.Fatalf("native/interactive directory differs: %v %d/%d", err, len(native), len(portal))
		}
		var managed uuid.UUID
		for _, item := range portal {
			if native[item.ID] != item.Name || item.ID == private || item.ID == modelmetrics.DashboardID(enterprise) {
				t.Fatal("read-only native directory changed identity or leaked private data")
			}
			if item.Name == "Host processes" {
				managed = item.ID
			}
		}
		if (managed != uuid.Nil) != wantDefault {
			t.Fatal("default discovery ignored current Host authorization")
		}
		return managed
	}
	managed := assertParity(true)
	call.ToolID, call.Input = "telemetry.dashboard.get", map[string]any{"dashboard_id": managed.String()}
	if result, err := registry.Call(ctx, call); err != nil || result.Structured["name"] != "Host processes" {
		t.Fatal("read-only native definition unavailable", err)
	}
	for _, denied := range []uuid.UUID{private, modelmetrics.DashboardID(enterprise)} {
		call.Input = map[string]any{"dashboard_id": denied.String()}
		if _, err := registry.Call(ctx, call); err == nil {
			t.Fatal("read-only native get exposed a private dashboard")
		}
	}
	if _, err := service.CreateDraft(ctx, actor, dashboard.DraftInput{Name: "Denied reader draft"}); !errors.Is(err, dashboard.ErrDenied) {
		t.Fatal("native discovery granted management", err)
	}
	exec("DELETE FROM data_authorization_grants WHERE id=$1", grant)
	assertParity(false)
}
