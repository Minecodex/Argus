package collectormanager_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	promengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"github.com/kakj-go/Argus/internal/transport/httpapi"
	traceapi "go.opentelemetry.io/otel/trace"
)

type observationPipelineBackend struct {
	sdkQueryBackend
	catalog telemetry.ClickHouseQuery
}

func (b observationPipelineBackend) DiscoverData(ctx context.Context, in telemetry.DataCatalogRequest) (telemetry.DataCatalogResult, error) {
	return b.catalog.DiscoverData(ctx, in)
}

// Real SDK -> native Collector -> authenticated Ingest -> SASL Kafka -> Writer ->
// ClickHouse -> resource-authorized HTTP -> portal. Identity enrollment is fixture
// provisioned. PostgreSQL and ClickHouse must belong to the disposable namespace.
func TestNativeObservationQueryPipelineAndPortal(t *testing.T) {
	if os.Getenv("ARGUS_OBSERVE_PIPELINE") != "1" {
		t.Skip("requires explicitly owned Plan10 dependencies")
	}
	f := newNativeSDKProtocolPipelineWithFaults(t, "otlp", nativeSDKFaultOptions{Duration: 10 * time.Minute})
	f.private = "observe-private-" + uuid.NewString()
	app := newActualSDKService(t, f, f.apps[0], "observe")
	ctx, span := app.start(f.ctx, "GET /observe-acceptance", traceapi.SpanKindServer, f.private)
	traceID := span.SpanContext().TraceID().String()
	app.log(ctx, "observe-first-event", f.private)
	app.log(ctx, "observe-second-event", f.private)
	span.End()
	if err := app.flushMetrics(f.ctx); err != nil {
		t.Fatal(err)
	}
	verifySDKMetricSum(t, f, f.apps[0], 2)
	f.waitRows(1, 2, 0, 0)
	department, user, role, grant := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f.exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Observation readers')", department, f.enterprise)
	f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Observation reader')", user, f.enterprise, department, user.String())
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Observation application reader')", role, f.enterprise)
	f.exec("INSERT INTO permissions(id,description,registry_version) VALUES('application.read','Read applications',$1) ON CONFLICT(id) DO NOTHING", authorization.PermissionRegistryVersion)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'application.read')", role)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), f.enterprise, user, role)
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'application',$4)", grant, f.enterprise, user, f.apps[0].app)
	token, csrf := uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	stamp := func(v time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: v, Valid: true} }
	_, err := f.store.Queries.CreateSession(f.ctx, db.CreateSessionParams{ID: uuid.New(), TokenHash: identity.TokenHash(token), CsrfHash: identity.TokenHash(csrf), Audience: "enterprise", UserID: user, EnterpriseID: uuid.NullUUID{UUID: f.enterprise, Valid: true}, DepartmentID: uuid.NullUUID{UUID: department, Valid: true}, AuthorizationVersion: pgtype.Int8{Int64: 1, Valid: true}, Locale: "zh-CN", IdleExpiresAt: stamp(now.Add(time.Hour)), AbsoluteExpiresAt: stamp(now.Add(time.Hour)), LastSeenAt: stamp(now), AuthenticatedAt: stamp(now), Amr: []string{"password"}})
	if err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{9}, 32)
	origins := []string{"http://127.0.0.1:4278"}
	identityHandler := httpapi.EnterpriseIdentityHandler{Auth: httpapi.SetupHandler{Config: config.Server{AllowedOrigins: origins}, Identity: identity.Service{Store: f.store}}, Service: identity.EnterpriseService{Store: f.store}, Cursor: pagination.Signer{Key: key}}
	coordinator := &queryengine.Coordinator{Trace: queryengine.TraceEngine{Engine: skywalking.Engine{Conn: f.conn, Router: f.router}}, KQL: queryengine.KQLEngine{Conn: f.conn, Router: f.router}, PromQL: queryengine.PromQLEngine{Engine: promengine.NewEngine(f.conn, f.router, nil)}}
	backend := observationPipelineBackend{sdkQueryBackend{coordinator}, telemetry.ClickHouseQuery{Conn: f.conn, Router: f.router}}
	service := telemetry.Service{Store: f.store, Engine: backend, Actions: resource.PendingActionService{Store: f.store, Key: key}}
	handler := httpapi.TelemetryHandler{Identity: identityHandler, Service: service}
	dashboards := httpapi.DashboardHandler{Identity: identityHandler, Service: dashboard.Service{Store: f.store}}
	server := httptest.NewServer(httpapi.NewRouterWithOptions(httpapi.RouterOptions{Setup: &identityHandler.Auth, EnterpriseIdentity: &identityHandler, Telemetry: &handler, Dashboard: &dashboards, AllowedOrigins: origins}))
	t.Cleanup(server.Close)
	call := func(method, path string, body any) (int, map[string]any) {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		r, _ := http.NewRequestWithContext(f.ctx, method, server.URL+"/api/v1"+path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origins[0])
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: "argus_enterprise_session", Value: token})
		r.AddCookie(&http.Cookie{Name: "argus_enterprise_csrf", Value: csrf})
		response, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		payload, _ := io.ReadAll(response.Body)
		if bytes.Contains(payload, []byte(f.private)) {
			t.Fatal("private telemetry attribute escaped projection")
		}
		out := map[string]any{}
		_ = json.Unmarshal(payload, &out)
		return response.StatusCode, out
	}
	if status, _ := call("GET", "/enterprise/telemetry/query/resources?kind=application&limit=30", nil); status != 200 {
		t.Fatalf("resource directory without dashboard permission: %d", status)
	}
	if status, _ := call("GET", "/dashboard-scope-resources?kind=application", nil); status != 403 {
		t.Fatalf("dashboard directory permission widened: %d", status)
	}
	scope := map[string]any{"mode": "explicit", "resources": []map[string]any{{"type": "application", "id": f.apps[0].app.String()}}, "source_type": "otlp"}
	catalog := map[string]any{"scope": scope, "signal": "metrics", "kind": "metrics", "from": now.Add(-time.Hour), "to": time.Now().Add(time.Second), "limit": 30}
	status, result := call("POST", "/enterprise/telemetry/query/catalog", catalog)
	if status != 200 {
		t.Fatalf("actual metric catalog: %d %v", status, result)
	}
	entries, _ := result["metrics"].([]any)
	found := false
	for _, entry := range entries {
		m := entry.(map[string]any)
		found = found || m["name"] == "plan8_sdk_request_count" && m["observed"] == true && m["type"] == "counter"
	}
	if !found {
		t.Fatal("actual collected metric missing from catalog")
	}
	query := map[string]any{"scope": scope, "query": "sum(plan8_sdk_request_count)", "time_range": map[string]any{"from": now.Add(-time.Hour), "to": time.Now().Add(time.Second)}}
	if status, result = call("POST", "/enterprise/metrics/query", query); status != 200 {
		t.Fatalf("structured query: %d %v", status, result)
	}
	scope["source_keys"] = []string{f.apps[1].source + ":1"}
	if status, _ = call("POST", "/enterprise/metrics/query", query); status != 403 {
		t.Fatalf("unauthorized source version: %d", status)
	}
	scope["source_keys"] = []string{}
	if status, result = call("POST", "/enterprise/telemetry/query/catalog", catalog); status != 200 || len(result["metrics"].([]any)) != 0 {
		t.Fatal("explicit empty source selection expanded")
	}
	delete(scope, "source_keys")
	query["resource_ids"] = []string{f.apps[0].app.String()}
	if status, _ = call("POST", "/enterprise/metrics/query", query); status != 400 {
		t.Fatalf("conflicting legacy and typed scopes accepted: %d", status)
	}
	delete(query, "resource_ids")
	scope["resources"] = []map[string]any{{"type": "host", "id": f.apps[0].app.String()}}
	if status, _ = call("POST", "/enterprise/metrics/query", query); status != 403 {
		t.Fatalf("forged resource type: %d", status)
	}
	scope["resources"] = []map[string]any{{"type": "application", "id": f.apps[1].app.String()}}
	if status, _ = call("POST", "/enterprise/telemetry/query/catalog", catalog); status != 403 {
		t.Fatalf("unauthorized application catalog: %d", status)
	}
	scope["resources"] = []map[string]any{}
	if status, result = call("POST", "/enterprise/telemetry/query/catalog", catalog); status != 200 || len(result["metrics"].([]any)) != 0 {
		t.Fatal("explicit empty catalog expanded")
	}
	scope["resources"] = []map[string]any{{"type": "application", "id": f.apps[0].app.String()}}
	runObservationPortal(t, f, server.URL, token, csrf, traceID)
	f.exec("DELETE FROM data_authorization_grants WHERE id=$1", grant)
	if status, _ = call("POST", "/enterprise/telemetry/query/catalog", catalog); status != 403 {
		t.Fatalf("catalog retained revoked grant: %d", status)
	}
	t.Log("actual Collector/Ingest/Kafka/Writer data, resource scope, catalog, query, conversion, portal and revocation passed; enrollment was fixture provisioned")
}

func runObservationPortal(t *testing.T, f *nativeSDKPipeline, api, session, csrf, trace string) {
	t.Helper()
	if os.Getenv("ARGUS_OBSERVE_BROWSER") != "1" {
		return
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "observation-browser.json")
	payload, _ := json.Marshal(map[string]string{"api": api, "session": session, "csrf": csrf, "application": f.apps[0].app.String(), "trace": trace})
	if err = os.WriteFile(file, payload, 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:4278")
	if err != nil {
		t.Fatal("owned browser port unavailable")
	}
	listener.Close()
	cmd := exec.CommandContext(f.ctx, "node", filepath.Join(root, "node_modules", "@playwright", "test", "cli.js"), "test", "--config=playwright.observe.config.ts", "--workers=1")
	cmd.Dir = filepath.Join(root, "web", "apps", "enterprise")
	cmd.Env = append(os.Environ(), "ARGUS_OBSERVE_BROWSER_FIXTURE="+file, "VITE_API_BASE_URL="+api)
	hideNativeSDKWindow(cmd)
	output, err := cmd.CombinedOutput()
	safe := string(output)
	for _, secret := range []string{session, csrf, f.private, f.apps[0].token, f.apps[1].token} {
		safe = strings.ReplaceAll(safe, secret, "[REDACTED]")
	}
	if err != nil {
		t.Fatalf("observation real browser: %v\n%s", err, safe)
	}
	t.Log(safe)
}
