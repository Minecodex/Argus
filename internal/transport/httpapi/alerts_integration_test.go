package httpapi

import (
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/alerting"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/config"
	api "github.com/kakj-go/Argus/internal/gen/openapi/alertsapi"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPostgresAlertHTTPPreviewUsesExplicitIAMPermission(t *testing.T) {
	address := os.Getenv("ARGUS_ALERT_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable Plan6 PostgreSQL")
	}
	ctx := t.Context()
	if err := postgres.RunMigrations(ctx, address, filepath.Join("..", "..", "..", "migrations", "postgresql"), postgres.MigrationUp); err != nil {
		t.Fatal(err)
	}
	store, err := postgres.Open(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := store.Pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	e, d, u, sa, role := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Plan6 HTTP',$2,'UTC')", e, e.String())
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Operations')", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'IAM operator')", u, e, d, u.String())
	exec("INSERT INTO service_accounts(id,enterprise_id,name) VALUES($1,$2,'Evaluator')", sa, e)
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'IAM only')", role, e)
	for id, description := range authorization.PermissionRegistry {
		exec("INSERT INTO permissions(id,description,registry_version) VALUES($1,$2,$3) ON CONFLICT(id) DO NOTHING", id, description, authorization.PermissionRegistryVersion)
	}
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'service_account.read'),($1,'service_account.manage')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), e, u, role)
	nullE := uuid.NullUUID{UUID: e, Valid: true}
	if err = audit.InitializeChain(ctx, store.Queries, "enterprise", nullE); err != nil {
		t.Fatal(err)
	}
	token := uuid.NewString()
	now := time.Now().UTC()
	stamp := func(v time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: v, Valid: true} }
	_, err = store.Queries.CreateSession(ctx, db.CreateSessionParams{ID: uuid.New(), TokenHash: identity.TokenHash(token), CsrfHash: identity.TokenHash("csrf"), Audience: "enterprise", UserID: u, EnterpriseID: nullE, DepartmentID: uuid.NullUUID{UUID: d, Valid: true}, AuthorizationVersion: pgtype.Int8{Int64: 1, Valid: true}, Locale: "zh-CN", IdleExpiresAt: stamp(now.Add(time.Hour)), AbsoluteExpiresAt: stamp(now.Add(time.Hour)), LastSeenAt: stamp(now), AuthenticatedAt: stamp(now), Amr: []string{"password"}})
	if err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{9}, 32)
	h := AlertHandler{Identity: EnterpriseIdentityHandler{Auth: SetupHandler{Config: config.Server{AllowedOrigins: []string{"http://argus.test"}}, Identity: identity.Service{Store: store}}}, Service: alerting.Service{Store: store, Actions: resource.PendingActionService{Store: store, Key: key, Idempotency: postgres.Idempotency{Key: key}}}}
	request := httptest.NewRequest(http.MethodPost, "http://argus.test/api/v1/alerts/configurations/preview", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName("enterprise"), Value: token})
	request.AddCookie(&http.Cookie{Name: csrfCookieName("enterprise"), Value: "csrf"})
	request.Header.Set("Origin", "http://argus.test")
	request.Header.Set("X-CSRF-Token", "csrf")
	requestCtx := WithRequestContext(ctx, httptest.NewRecorder(), request)

	raw, _ := json.Marshal(map[string]any{"kind": "settings", "operation": "create", "name": "Evaluator", "spec": map[string]any{"evaluator_id": sa}})
	var body api.AlertMutation
	if err = json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}

	response, err := h.PreviewAlertConfiguration(requestCtx, api.PreviewAlertConfigurationRequestObject{Body: &body, Params: api.PreviewAlertConfigurationParams{XCSRFToken: "csrf", IdempotencyKey: uuid.NewString()}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(api.PreviewAlertConfiguration202JSONResponse); !ok {
		t.Fatalf("IAM settings preview denied: %#v", response)
	}
	var count int
	if err = store.Pool.QueryRow(ctx, "SELECT count(*) FROM alert_configurations WHERE enterprise_id=$1", e).Scan(&count); err != nil || count != 0 {
		t.Fatal("preview wrote configuration", count, err)
	}
	exec("DELETE FROM role_permissions WHERE role_id=$1 AND permission_id='service_account.manage'", role)
	denied, err := h.PreviewAlertConfiguration(requestCtx, api.PreviewAlertConfigurationRequestObject{Body: &body, Params: api.PreviewAlertConfigurationParams{XCSRFToken: "csrf", IdempotencyKey: uuid.NewString()}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := denied.(api.PreviewAlertConfigurationdefaultJSONResponse); !ok {
		t.Fatal("settings mutation admitted after permission removal")
	}
	read, err := h.ListAlertConfigurations(requestCtx, api.ListAlertConfigurationsRequestObject{Params: api.ListAlertConfigurationsParams{Kind: "settings"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := read.(api.ListAlertConfigurations200JSONResponse); !ok {
		t.Fatalf("IAM read requires unrelated alert permission: %#v", read)
	}
}
