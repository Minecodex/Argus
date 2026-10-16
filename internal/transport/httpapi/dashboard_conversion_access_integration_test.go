package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/dashboard"
	api "github.com/kakj-go/Argus/internal/gen/openapi/dashboardapi"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresResourceReaderConvertsQueriesWithoutDashboardWrites(t *testing.T) {
	address := os.Getenv("ARGUS_DASHBOARD_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires disposable PostgreSQL")
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
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.Pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	enterprise, department, user, role := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Resource query UI',$2,'UTC')", enterprise, enterprise.String())
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Operations')", department, enterprise)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Resource reader')", user, enterprise, department, user.String())
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Host reader only')", role, enterprise)
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'host.read')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), enterprise, user, role)
	token, csrf := uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	stamp := func(value time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: value, Valid: true} }
	_, err = store.Queries.CreateSession(ctx, db.CreateSessionParams{
		ID: uuid.New(), TokenHash: identity.TokenHash(token), CsrfHash: identity.TokenHash(csrf), Audience: "enterprise",
		UserID: user, EnterpriseID: uuid.NullUUID{UUID: enterprise, Valid: true}, DepartmentID: uuid.NullUUID{UUID: department, Valid: true},
		AuthorizationVersion: pgtype.Int8{Int64: 1, Valid: true}, Locale: "zh-CN", IdleExpiresAt: stamp(now.Add(time.Hour)),
		AbsoluteExpiresAt: stamp(now.Add(time.Hour)), LastSeenAt: stamp(now), AuthenticatedAt: stamp(now), Amr: []string{"password"},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := DashboardHandler{Identity: EnterpriseIdentityHandler{Auth: SetupHandler{
		Config: config.Server{AllowedOrigins: []string{"http://argus.test"}}, Identity: identity.Service{Store: store},
	}}}
	body := dashboardConvert[api.DashboardConvertPanelInput](dashboard.ConvertPanelInput{Mode: "dsl", Panel: dashboard.Panel{
		ID: "quick", Signal: "metrics", AuthoringMode: "builder",
		Targets: []dashboard.Target{{ID: "main", Language: "promql", QueryMode: "range", RangeStepPolicy: dashboard.StepPolicy{Kind: "fixed", Seconds: 60}, SourceDefinition: dashboard.Definition{Builder: &dashboard.Builder{
			Metric: "system_cpu_utilization", Operation: "avg", Filters: []dashboard.Filter{}, GroupBy: []string{},
		}}}},
	}})
	for _, scenario := range []struct {
		name, cookie, csrf string
		allowed            bool
	}{
		{"resource reader", token, csrf, true}, {"missing session", "", csrf, false}, {"wrong csrf", token, "wrong", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://argus.test/api/v1/dashboard-spec/convert-panel", nil)
			request.AddCookie(&http.Cookie{Name: sessionCookieName("enterprise"), Value: scenario.cookie})
			request.AddCookie(&http.Cookie{Name: csrfCookieName("enterprise"), Value: csrf})
			request.Header.Set("Origin", "http://argus.test")
			request.Header.Set("X-CSRF-Token", scenario.csrf)
			requestCtx := WithRequestContext(ctx, httptest.NewRecorder(), request)
			result, err := handler.ConvertDashboardPanel(requestCtx, api.ConvertDashboardPanelRequestObject{
				Body: &body, Params: api.ConvertDashboardPanelParams{XCSRFToken: scenario.csrf},
			})
			if err != nil {
				t.Fatal(err)
			}
			converted, ok := result.(api.ConvertDashboardPanel200JSONResponse)
			if ok != scenario.allowed || ok && !converted.Converted {
				t.Fatalf("unexpected conversion response: %#v", result)
			}
		})
	}
	for _, table := range []string{"dashboard_drafts", "dashboard_revisions", "pending_actions"} {
		var count int
		if err := store.Pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE enterprise_id=$1", enterprise).Scan(&count); err != nil || count != 0 {
			t.Fatalf("conversion persisted %s: %d %v", table, count, err)
		}
	}
}
