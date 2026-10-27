package collectormanager_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	promengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/transport/httpapi"
)

type sdkQueryBackend struct{ coordinator *queryengine.Coordinator }

func (b sdkQueryBackend) ExecuteEngineQuery(ctx context.Context, request queryengine.Request) (queryengine.Result, error) {
	return b.coordinator.Execute(ctx, request)
}

// The API uses actual session/role/grant checks and the actual generated HTTP
// adapter. The trusted session is fixture-created; this is not a login, deployed
// Portal, Query gRPC transport, or real-model acceptance claim.
func verifyNativeSDKQueryEntries(t *testing.T, f *nativeSDKPipeline, traceID string, inventoryService uuid.UUID) {
	t.Helper()
	department, userID, role, grant := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f.exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'SDK readers')", department, f.enterprise)
	f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'SDK application reader')", userID, f.enterprise, department, userID.String())
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'SDK application read only')", role, f.enterprise)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'application.read')", role)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), f.enterprise, userID, role)
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'application',$4)", grant, f.enterprise, userID, f.apps[0].app)
	user, err := f.store.Queries.GetEnterpriseUser(f.ctx, db.GetEnterpriseUserParams{EnterpriseID: f.enterprise, ID: userID})
	if err != nil {
		t.Fatal(err)
	}
	token := uuid.NewString()
	csrf := uuid.NewString()
	now := time.Now().UTC()
	stamp := func(value time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: value, Valid: true} }
	_, err = f.store.Queries.CreateSession(f.ctx, db.CreateSessionParams{ID: uuid.New(), TokenHash: identity.TokenHash(token), CsrfHash: identity.TokenHash(csrf), Audience: "enterprise", UserID: userID, EnterpriseID: uuid.NullUUID{UUID: f.enterprise, Valid: true}, DepartmentID: uuid.NullUUID{UUID: department, Valid: true}, AuthorizationVersion: pgtype.Int8{Int64: user.AuthorizationVersion, Valid: true}, Locale: "zh-CN", IdleExpiresAt: stamp(now.Add(time.Hour)), AbsoluteExpiresAt: stamp(now.Add(time.Hour)), LastSeenAt: stamp(now), AuthenticatedAt: stamp(now), Amr: []string{"password"}})
	if err != nil {
		t.Fatal(err)
	}
	coordinator := &queryengine.Coordinator{Trace: queryengine.TraceEngine{Engine: skywalking.Engine{Conn: f.conn, Router: f.router}}, KQL: queryengine.KQLEngine{Conn: f.conn, Router: f.router}, PromQL: queryengine.PromQLEngine{Engine: promengine.NewEngine(f.conn, f.router, nil)}}
	service := telemetry.Service{Store: f.store, Engine: sdkQueryBackend{coordinator}}
	handler := httpapi.TelemetryHandler{Identity: httpapi.EnterpriseIdentityHandler{Auth: httpapi.SetupHandler{Config: config.Server{AllowedOrigins: []string{"http://127.0.0.1:4173"}}, Identity: identity.Service{Store: f.store}}}, Service: service}
	monitor := httpapi.MonitoringHandler{Identity: handler.Identity, Service: monitoring.Service{Store: f.store, CursorKey: bytes.Repeat([]byte{7}, 32)}}
	server := httptest.NewServer(httpapi.NewRouterWithOptions(httpapi.RouterOptions{Setup: &handler.Identity.Auth, Telemetry: &handler, Monitoring: &monitor, EnterpriseIdentity: &handler.Identity, AllowedOrigins: []string{"http://127.0.0.1:4173"}}))
	t.Cleanup(server.Close)
	post := func(path string, body any) (int, map[string]any) {
		t.Helper()
		request, err := http.NewRequestWithContext(f.ctx, http.MethodPost, server.URL+"/api/v1/enterprise/"+path, bytes.NewReader(mustSDKJSON(t, body)))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(&http.Cookie{Name: "argus_enterprise_session", Value: token})
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result map[string]any
		if err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, result
	}
	variables := map[string]any{"serviceIds": []any{inventoryService.String()}}
	body := map[string]any{"resource_ids": []string{f.apps[0].app.String()}, "query": skywalking.ApplicationAPMQueries()["services"], "operation_name": "ApplicationAPM", "variables": variables, "time_range": map[string]any{"from": now.Add(-time.Minute), "to": now.Add(time.Second)}}
	status, result := post("traces/graphql", body)
	if status != http.StatusOK {
		t.Fatalf("actual SDK HTTP APM denied: status=%d code=%v", status, result["code"])
	}
	rows := result["data"].(map[string]any)["queryAPMServices"].(map[string]any)["rows"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["serviceId"] != inventoryService.String() {
		t.Fatal("HTTP query did not preserve stable service variables")
	}
	registry := mcp.NewRegistry()
	if err = (telemetry.Tools{Service: service}).Register(registry); err != nil {
		t.Fatal(err)
	}
	if err = (toolgateway.MonitoringTools{Base: toolgateway.ResourceTools{Store: f.store}, Monitoring: monitor.Service, Kubernetes: kubernetesworkbench.Service{Store: f.store}}).Register(registry); err != nil {
		t.Fatal(err)
	}
	verifyApplicationToolRevocation := verifyNativeSDKApplicationDirectoryTools(t, f, registry, userID)
	call := mcp.Call{ToolID: "telemetry.skywalking.trace", Subject: userID.String(), SubjectType: "user", Enterprise: f.enterprise.String(), Input: map[string]any{"document": skywalking.ApplicationAPMQueries()["services"], "operation_name": "ApplicationAPM", "variables": variables, "resource_ids": []any{f.apps[0].app.String()}, "from": now.Add(-time.Minute).Format(time.RFC3339Nano), "to": now.Add(time.Second).Format(time.RFC3339Nano)}}
	toolResult, err := registry.Call(f.ctx, call)
	if err != nil {
		t.Fatalf("actual SDK native APM query: %v", err)
	}
	toolRows := toolResult.Structured["data"].(map[string]any)["queryAPMServices"].(map[string]any)["rows"].([]any)
	if len(toolRows) != 1 || toolRows[0].(map[string]any)["serviceId"] != inventoryService.String() {
		t.Fatal("native tool did not match HTTP service filter")
	}
	typedCall := call
	typedCall.Input = map[string]any{"application_view": "services", "service_ids": []any{inventoryService.String()}, "resource_ids": []any{f.apps[0].app.String()}, "from": call.Input["from"], "to": call.Input["to"]}
	typedResult, err := registry.Call(f.ctx, typedCall)
	if err != nil {
		t.Fatalf("actual SDK typed APM query: %v", err)
	}
	typedRows := typedResult.Structured["data"].(map[string]any)["queryAPMServices"].(map[string]any)["rows"].([]any)
	if len(typedRows) != 1 {
		t.Fatal("typed APM lost its actual service scope")
	}
	for _, field := range []string{"serviceId", "sampleCount", "errorCount"} {
		if typedRows[0].(map[string]any)[field] != toolRows[0].(map[string]any)[field] {
			t.Fatal("typed APM diverged from the same HTTP/document query", field)
		}
	}
	for _, view := range []string{"services", "instances", "endpoints", "red", "topology", "traces"} {
		for _, filtered := range []bool{false, true} {
			viewCall := typedCall
			viewCall.Input = map[string]any{"application_view": view, "resource_ids": []any{f.apps[0].app.String()}, "from": call.Input["from"], "to": call.Input["to"]}
			viewCall.Input["limit"] = float64(100)
			if filtered {
				viewCall.Input["service_ids"] = []any{inventoryService.String()}
			}
			result, e := registry.Call(f.ctx, viewCall)
			if e != nil || len(result.Structured["data"].(map[string]any)) == 0 {
				t.Fatalf("actual SDK typed %s view (service filter=%t) failed: %v", view, filtered, e)
			}
		}
	}
	for _, resource := range []uuid.UUID{f.host, f.apps[1].app} {
		body["resource_ids"] = []string{resource.String()}
		if status, _ = post("traces/graphql", body); status != http.StatusForbidden {
			t.Fatalf("application reader gained another root: %d", status)
		}
		call.Input["resource_ids"] = []any{resource.String()}
		if _, err = registry.Call(f.ctx, call); err == nil {
			t.Fatal("native application reader gained another root")
		}
		typedCall.Input["resource_ids"] = []any{resource.String()}
		if _, err = registry.Call(f.ctx, typedCall); err == nil {
			t.Fatal("typed APM expanded into an unauthorized root")
		}
	}
	typedCall.Input["resource_ids"] = []any{f.apps[0].app.String()}
	body["resource_ids"] = []string{f.apps[0].app.String()}
	body["query"] = skywalking.ApplicationAPMQueries()["trace_detail"]
	detailVars := map[string]any{"traceId": traceID, "sourceId": f.apps[0].source, "resourceId": f.apps[0].app.String()}
	body["variables"] = detailVars
	if status, result = post("traces/graphql", body); status != http.StatusOK {
		t.Fatalf("actual SDK HTTP detail failed: %d", status)
	}
	if spans := result["data"].(map[string]any)["queryTrace"].(map[string]any)["spans"].([]any); len(spans) != 4 {
		t.Fatal("HTTP application detail leaked or lost spans")
	}
	call.Input["document"], call.Input["resource_ids"], call.Input["variables"] = body["query"], []any{f.apps[0].app.String()}, detailVars
	if toolResult, err = registry.Call(f.ctx, call); err != nil {
		t.Fatalf("parameterized native Trace detail failed: %v", err)
	}
	if spans := toolResult.Structured["data"].(map[string]any)["queryTrace"].(map[string]any)["spans"].([]any); len(spans) != 4 {
		t.Fatal("native Trace detail diverged from HTTP")
	}
	detailVars["resourceId"], detailVars["sourceId"] = f.apps[1].app.String(), f.apps[1].source
	if status, result = post("traces/graphql", body); status != http.StatusOK || result["data"].(map[string]any)["queryTrace"] != nil {
		t.Fatal("GraphQL variables expanded the HTTP authorization scope")
	}
	if toolResult, err = registry.Call(f.ctx, call); err != nil || toolResult.Structured["data"].(map[string]any)["queryTrace"] != nil {
		t.Fatal("GraphQL variables expanded the native authorization scope")
	}
	logBody := map[string]any{"query": `resource_attributes.argus.service.id = "` + inventoryService.String() + `"`, "pipeline": "limit 100", "resource_ids": []string{f.apps[0].app.String()}, "time_range": body["time_range"]}
	if status, result = post("logs/query", logBody); status != http.StatusOK || len(result["data"].([]any)) != 1 {
		t.Fatalf("actual SDK HTTP correlated log status=%d", status)
	}
	if result["data"].([]any)[0].(map[string]any)["trace_id"] != traceID {
		t.Fatal("HTTP log lost actual SDK Trace correlation")
	}
	metricBody := map[string]any{"query": "sum(plan8_sdk_request_count)", "resource_ids": []string{f.apps[0].app.String()}, "time_range": body["time_range"]}
	if status, result = post("metrics/query", metricBody); status != http.StatusOK {
		t.Fatalf("actual SDK HTTP PromQL failed: %d", status)
	}
	metricRows := result["data"].(map[string]any)["result"].([]any)
	if len(metricRows) != 1 || metricRows[0].(map[string]any)["value"].([]any)[1] != "2" {
		t.Fatal("HTTP application metrics mixed service dimensions or application scope")
	}
	metricCall := mcp.Call{ToolID: "telemetry.promql.query", Subject: userID.String(), SubjectType: "user", Enterprise: f.enterprise.String(), Input: map[string]any{"query": "sum(plan8_sdk_request_count)", "resource_ids": []any{f.apps[0].app.String()}, "from": now.Add(-time.Minute).Format(time.RFC3339Nano), "to": now.Add(time.Second).Format(time.RFC3339Nano)}}
	metricResult, err := registry.Call(f.ctx, metricCall)
	if err != nil {
		t.Fatalf("actual SDK native PromQL failed: %v", err)
	}
	encodedMetric, _ := json.Marshal(metricResult.Structured)
	var normalizedMetric map[string]any
	_ = json.Unmarshal(encodedMetric, &normalizedMetric)
	metricRows = normalizedMetric["data"].(map[string]any)["result"].([]any)
	if len(metricRows) != 1 || metricRows[0].(map[string]any)["value"].([]any)[1] != "2" {
		t.Fatal("native application metric query diverged from HTTP")
	}
	runNativeSDKPortal(t, f, server.URL, token, csrf, traceID)
	f.exec("DELETE FROM data_authorization_grants WHERE id=$1", grant)
	verifyApplicationToolRevocation()
	if status, _ = post("traces/graphql", body); status != http.StatusForbidden {
		t.Fatal("HTTP retained access after application grant revocation")
	}
	if _, err = registry.Call(f.ctx, call); err == nil {
		t.Fatal("native query retained access after application grant revocation")
	}
	if _, err = registry.Call(f.ctx, typedCall); err == nil {
		t.Fatal("typed APM retained revoked application access")
	}
	t.Log("actual three-signal SDK data queried through generated HTTP/session authorization and native Tool; app-only grant, stable service variables, Trace details, correlation, PromQL, cross-root denial, variable forgery and revocation passed")
}
