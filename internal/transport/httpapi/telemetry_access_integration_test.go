package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type objectPermissionEngine struct {
	scopes []queryengine.Scope
	during func()
}
type permissionQueryBackend struct{ coordinator *queryengine.Coordinator }

func (backend permissionQueryBackend) ExecuteEngineQuery(ctx context.Context, request queryengine.Request) (queryengine.Result, error) {
	return backend.coordinator.Execute(ctx, request)
}

func (engine *objectPermissionEngine) Execute(_ context.Context, request queryengine.Request) (queryengine.Result, error) {
	engine.scopes = append(engine.scopes, request.Scope)
	if engine.during != nil {
		engine.during()
	}
	data := any([]map[string]any{{"body": "ordinary request password=secret-value", "service_name": "api"}})
	if request.Language == queryengine.LanguageTrace {
		data = map[string]any{"trace": map[string]any{"attributes": map[string]any{"business": "ok", "api_key": "secret-value"}, "events": []any{map[string]any{"name": "work"}}, "links": []any{map[string]any{"traceId": "linked"}}}}
	}
	return queryengine.Result{Language: request.Language, ResultType: "table", Data: data}, nil
}
func TestPostgresTelemetryObjectReadParityAndRevocation(t *testing.T) {
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
	e, d, user, sa, role, host, cluster := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Access parity',$2,'UTC')", e, e.String())
	exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Access')", d, e)
	exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Reader')", user, e, d, user.String())
	exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Object reader')", role, e)
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'host.read')", role)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), e, user, role)
	exec("INSERT INTO service_accounts(id,enterprise_id,name,allowed_tool_ids) VALUES($1,$2,'Reader',ARRAY['telemetry.promql.query','telemetry.kql.query','telemetry.skywalking.trace'])", sa, e)
	exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'service_account',$3,$4)", uuid.New(), e, sa, role)
	exec("INSERT INTO hosts(id,enterprise_id,name,address,port,platform,role,control_path,environment,labels_hash) VALUES($1,$2,'Host','127.0.0.1',22,'linux','managed_host','direct','development',decode(repeat('00',32),'hex'))", host, e)
	exec("INSERT INTO kubernetes_clusters(id,enterprise_id,name,api_server,connection_mode,environment,labels_hash) VALUES($1,$2,'Cluster','https://cluster.invalid','in_cluster','development',decode(repeat('00',32),'hex'))", cluster, e)
	for _, subject := range []struct {
		kind string
		id   uuid.UUID
	}{{"user", user}, {"service_account", sa}} {
		for _, object := range []struct {
			kind string
			id   uuid.UUID
		}{{"host", host}, {"kubernetes_cluster", cluster}} {
			exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,$3,$4,$5,$6)", uuid.New(), e, subject.kind, subject.id, object.kind, object.id)
		}
	}
	token := uuid.NewString()
	now := time.Now().UTC()
	stamp := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
	_, err = store.Queries.CreateSession(ctx, db.CreateSessionParams{ID: uuid.New(), TokenHash: identity.TokenHash(token), CsrfHash: identity.TokenHash("csrf"), Audience: "enterprise", UserID: user, EnterpriseID: uuid.NullUUID{UUID: e, Valid: true}, DepartmentID: uuid.NullUUID{UUID: d, Valid: true}, AuthorizationVersion: pgtype.Int8{Int64: 1, Valid: true}, Locale: "zh-CN", IdleExpiresAt: stamp(now.Add(time.Hour)), AbsoluteExpiresAt: stamp(now.Add(time.Hour)), LastSeenAt: stamp(now), AuthenticatedAt: stamp(now), Amr: []string{"password"}})
	if err != nil {
		t.Fatal(err)
	}
	engine := &objectPermissionEngine{}
	coordinator := &queryengine.Coordinator{PromQL: engine, KQL: engine, Trace: engine}
	service := telemetry.Service{Store: store, Engine: permissionQueryBackend{coordinator}}
	handler := TelemetryHandler{Identity: EnterpriseIdentityHandler{Auth: SetupHandler{Identity: identity.Service{Store: store}}}, Service: service}
	request := httptest.NewRequest(http.MethodPost, "http://argus.test/api/v1/enterprise/logs/query", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName("enterprise"), Value: token})
	requestContext := WithRequestContext(ctx, httptest.NewRecorder(), request)
	registry := mcp.NewRegistry()
	if err = (telemetry.Tools{Service: service}).Register(registry); err != nil {
		t.Fatal(err)
	}
	gateway, err := toolgateway.New(registry, "object-access-test")
	if err != nil {
		t.Fatal(err)
	}
	tools, err := toolruntime.NewSet(toolruntime.Snapshot{}, gateway.CoreTools())
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		tool     string
		language queryengine.Language
		category string
	}{{"telemetry.promql.query", queryengine.LanguagePromQL, "metric"}, {"telemetry.kql.query", queryengine.LanguageKQL, "log"}, {"telemetry.skywalking.trace", queryengine.LanguageTrace, "trace"}} {
		result, err := handler.executeEngine(requestContext, scenario.tool, scenario.language, "*", "", "", nil, []uuid.UUID{host}, now.Add(-time.Minute), now, 0, nil, true)
		if err != nil {
			t.Fatalf("host.read should admit %s without signal permissions: %v", scenario.tool, err)
		}
		raw, _ := json.Marshal(result.Data)
		if strings.Contains(string(raw), "secret-value") {
			t.Fatal("credential policy varied by protocol")
		}
		last := engine.scopes[len(engine.scopes)-1]
		if last.SubjectID != user || last.SubjectType != "user" || len(last.ResourceIDs) != 1 {
			t.Fatalf("HTTP lost principal scope: %+v", last)
		}
		for _, subject := range []struct {
			kind string
			id   uuid.UUID
		}{{"user", user}, {"service_account", sa}} {
			call := mcp.Call{ToolID: scenario.tool, Subject: subject.id.String(), SubjectType: subject.kind, Enterprise: e.String(), Input: map[string]any{"resource_ids": []any{host.String()}, "from": now.Add(-time.Minute).Format(time.RFC3339), "to": now.Format(time.RFC3339), "query": "*", "document": "query { trace }"}}
			if _, err = registry.Call(ctx, call); err != nil {
				t.Fatalf("native %s denied object read: %v", subject.kind, err)
			}
			last = engine.scopes[len(engine.scopes)-1]
			if last.SubjectID != subject.id || last.SubjectType != subject.kind {
				t.Fatal("native query lost subject identity")
			}
			call.Input["resource_ids"] = []any{cluster.String()}
			if _, err = registry.Call(ctx, call); err == nil {
				t.Fatal("host capability authorized a cluster")
			}
		}
		_, err = tools.Invoke(ctx, "tool.describe", toolruntime.Invocation{Principal: toolruntime.Principal{EnterpriseID: e, UserID: user, Permissions: []string{"host.read"}}, Arguments: map[string]any{"category": scenario.category, "name": map[string]string{"metric": "query", "log": "query", "trace": "query"}[scenario.category]}})
		if err != nil {
			t.Fatal("native discovery hides a valid object reader", err)
		}
	}
	// Object capability is necessary in addition to a grant, and a mixed scope
	// must not silently return only the authorized half.
	for _, ids := range [][]uuid.UUID{{cluster}, {host, cluster}, {}, {uuid.New()}} {
		if _, err = handler.executeEngine(requestContext, "telemetry.kql.query", queryengine.LanguageKQL, "*", "", "", nil, ids, now.Add(-time.Minute), now, 0, nil, false); err == nil {
			t.Fatalf("invalid/unauthorized resource scope accepted: %v", ids)
		}
	}
	exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'kubernetes.read')", role)
	if _, err = handler.executeEngine(requestContext, "telemetry.kql.query", queryengine.LanguageKQL, "*", "", "", nil, []uuid.UUID{cluster}, now.Add(-time.Minute), now, 0, nil, false); err != nil {
		t.Fatal("cluster object read denied", err)
	}
	exec("UPDATE service_accounts SET allowed_tool_ids=ARRAY['telemetry.promql.query'] WHERE id=$1", sa)
	exec("UPDATE kubernetes_clusters SET status='disabled' WHERE id=$1", cluster)
	if _, err = handler.executeEngine(requestContext, "telemetry.kql.query", queryengine.LanguageKQL, "*", "", "", nil, []uuid.UUID{cluster}, now.Add(-time.Minute), now, 0, nil, false); err == nil {
		t.Fatal("disabled cluster remained queryable")
	}
	exec("UPDATE kubernetes_clusters SET status='active' WHERE id=$1", cluster)
	if _, err = registry.Call(ctx, mcp.Call{ToolID: "telemetry.kql.query", Subject: sa.String(), SubjectType: "service_account", Enterprise: e.String(), Input: map[string]any{}}); err == nil {
		t.Fatal("service account allowlist bypassed")
	}
	engine.during = func() {
		exec("UPDATE data_authorization_grants SET status='disabled' WHERE enterprise_id=$1 AND subject_id=$2 AND resource_id=$3", e, user, host)
	}
	if _, err = handler.executeEngine(requestContext, "telemetry.kql.query", queryengine.LanguageKQL, "*", "", "", nil, []uuid.UUID{host}, now.Add(-time.Minute), now, 0, nil, false); !errors.Is(err, telemetry.ErrDenied) {
		t.Fatalf("returned data after resource revocation: %v", err)
	}
	engine.during = nil
	exec("UPDATE enterprise_users SET authorization_version=2 WHERE id=$1", user)
	if _, err = handler.executeEngine(requestContext, "telemetry.kql.query", queryengine.LanguageKQL, "*", "", "", nil, []uuid.UUID{cluster}, now.Add(-time.Minute), now, 0, nil, false); err == nil || telemetryStatus(err) != http.StatusConflict {
		t.Fatal("stale session version was accepted", err)
	}
	var count int
	if err = store.Pool.QueryRow(ctx, "SELECT count(*) FROM permissions WHERE id IN ('telemetry.query.metrics','telemetry.query.logs','telemetry.query.traces','telemetry.sensitive_fields.read')").Scan(&count); err != nil || count != 0 {
		t.Fatal("retired privileges remain in fresh database")
	}
}
