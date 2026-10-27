package toolgateway

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestPostgresMonitoringNativeDomainsIsolationPreviewAndPresentationRevocation(t *testing.T) {
	address := os.Getenv("ARGUS_MONITORING_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires owned disposable migrated Plan8 PostgreSQL")
	}
	store, err := postgres.Open(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := store.Pool.Exec(t.Context(), sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	e, d, u, role, app, hidden, grant := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Plan8 native domain',$2,'UTC')", e, e.String())
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Domain readers')", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Domain reader')", u, e, d, u.String())
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Domain role')", role, e)
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'application.read'),($1,'application.manage'),($1,'monitoring.read')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), e, u, role)
	for _, id := range []uuid.UUID{app, hidden} {
		exec("INSERT INTO monitoring_applications(id,enterprise_id,application_key,name,environment,created_by) VALUES($1,$2,$3,$3,'production',$4)", id, e, id.String(), u)
	}
	exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'application',$4)", grant, e, u, app)
	if err = audit.InitializeChain(t.Context(), store.Queries, "enterprise", uuid.NullUUID{UUID: e, Valid: true}); err != nil {
		t.Fatal(err)
	}
	actions := resource.PendingActionService{Store: store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{4}, 32)}, Key: bytes.Repeat([]byte{5}, 32)}
	domain := monitoring.Service{Store: store, Actions: actions, CursorKey: bytes.Repeat([]byte{6}, 32)}
	reg := mcp.NewRegistry()
	if err = (MonitoringTools{Base: ResourceTools{Store: store}, Monitoring: domain, Kubernetes: kubernetesworkbench.Service{Store: store}}).Register(reg); err != nil {
		t.Fatal(err)
	}
	call := mcp.Call{Enterprise: e.String(), Subject: u.String(), SubjectType: "user", ToolID: "application.list", Input: map[string]any{"limit": float64(1)}}
	result, err := reg.Call(t.Context(), call)
	if err != nil {
		t.Fatal(err)
	}
	rows := result.Structured["items"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != app.String() {
		t.Fatal("application directory crossed grant")
	}
	meta, _ := reg.Lookup(call.ToolID)
	view, err := meta.Present(t.Context(), call, result)
	if err != nil {
		t.Fatal(err)
	}
	if view == nil || len(view.Resources) != 1 || view.Resources[0].Type != "application" || view.Resources[0].ID != app.String() {
		t.Fatal("public result lost typed application provenance")
	}
	before := view.AuthorizationScope
	denied := call
	denied.ToolID = "application.get"
	denied.Input = map[string]any{"id": hidden.String()}
	if _, err = reg.Call(t.Context(), denied); err == nil {
		t.Fatal("ungranted application read")
	}
	preview := call
	preview.ToolID = "application.change.preview"
	preview.InvocationID = uuid.NewString()
	preview.Input = map[string]any{"kind": "application", "operation": "create", "application": map[string]any{"application_key": "new-native-app", "name": "Native app", "environment": "testing"}}
	prepared, err := reg.Call(t.Context(), preview)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Structured["action_ref"] == nil || prepared.Structured["preview"] == nil {
		t.Fatal("missing immutable public action")
	}
	encoded, _ := json.Marshal(prepared)
	for _, s := range []string{"immutable_plan", "commit_token", "credential_draft_id"} {
		if strings.Contains(string(encoded), s) {
			t.Fatal("private action material leaked")
		}
	}
	var count int
	if err = store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM monitoring_applications WHERE enterprise_id=$1 AND application_key='new-native-app'", e).Scan(&count); err != nil || count != 0 {
		t.Fatal("preview mutated application")
	}
	exec("DELETE FROM data_authorization_grants WHERE id=$1", grant)
	after, err := presentation.Scope(t.Context(), store, e, u)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("application revocation did not invalidate sealed result")
	}
	form := call
	form.ToolID = "monitoring.collection.form"
	form.Input = map[string]any{"integration": "mysql", "name": "Requested monitor", "endpoint": "mysql.example.test:3306"}
	if _, err := reg.Call(t.Context(), form); err == nil {
		t.Fatal("requesting a form bypassed monitoring management permission")
	}
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'monitoring.manage')", role)
	hostForm, err := reg.Call(t.Context(), form)
	if err != nil || hostForm.Structured["schema_version"] != "argus.monitoring_host_form/v1" || hostForm.Structured["status"] != "requires_user_input" {
		t.Fatal("trusted form request unavailable", err)
	}
	formMeta, _ := reg.Lookup(form.ToolID)
	formView, err := formMeta.Present(t.Context(), form, hostForm)
	if err != nil || formView.Data["tool"] != form.ToolID || formView.Data["schema_version"] != "argus.monitoring_host_form/v1" {
		t.Fatal("sealed form descriptor lost its native identity", err)
	}
	if err := store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM monitoring_collections WHERE enterprise_id=$1", e).Scan(&count); err != nil || count != 0 {
		t.Fatal("form request created monitoring state", err)
	}
	for _, input := range []map[string]any{{"integration": "clickhouse"}, {"integration": "mysql", "endpoint": "mysql://user:password@db:3306"}, {"integration": "mysql", "collection_id": uuid.NewString()}} {
		bad := form
		bad.Input = input
		if _, err := reg.Call(t.Context(), bad); err == nil {
			t.Fatal("deferred, unsafe or unowned form request accepted")
		}
	}
	call.ToolID = "application.get"
	call.Input = map[string]any{"id": app.String()}
	if _, err = reg.Call(t.Context(), call); err == nil {
		t.Fatal("revoked native application remained readable")
	}
	result, err = reg.Call(t.Context(), mcp.Call{Enterprise: e.String(), Subject: u.String(), SubjectType: "user", ToolID: "application.list", Input: map[string]any{}})
	if err != nil || len(result.Structured["items"].([]any)) != 0 {
		t.Fatal("empty grant expanded")
	}
	exec("UPDATE role_bindings SET status='disabled' WHERE role_id=$1", role)
	if _, err = reg.Call(t.Context(), preview); err == nil {
		t.Fatal("revoked functional permission prepared action")
	}
}
