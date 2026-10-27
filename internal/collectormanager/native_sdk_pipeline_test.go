package collectormanager_test

import (
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"go.opentelemetry.io/otel/propagation"
	traceapi "go.opentelemetry.io/otel/trace"
)

// Real Go Trace + Log SDKs and actual HTTP propagation exercise native receiver
// authentication, real PG discovery, mTLS Ingest, SASL Kafka, Writer and Query.
// Enrollment/CA/control registration remain explicitly controlled fixtures.
func TestNativeSDKApplicationPipelineDiscoveryAPMLogsAndKeyRevocation(t *testing.T) {
	f := newNativeSDKPipeline(t)
	private := "sdk-private-canary-" + uuid.NewString()
	f.private = private
	payments := newActualSDKService(t, f, f.apps[0], "payments")
	inventory := newActualSDKService(t, f, f.apps[0], "inventory")
	partner := newActualSDKService(t, f, f.apps[1], "payments")
	propagator := propagation.TraceContext{}
	leaf := func(s *actualSDKService, operation, message string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := s.start(ctx, operation, traceapi.SpanKindServer, private)
			defer span.End()
			s.log(ctx, message, private)
			w.WriteHeader(http.StatusNoContent)
		}))
	}
	stock := leaf(inventory, "GET /inventory", "sdk-inventory-event")
	t.Cleanup(stock.Close)
	external := leaf(partner, "partner-private-handler", "sdk-partner-event")
	t.Cleanup(external.Close)
	traceIDs := make(chan string, 1)
	checkout := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, root := payments.start(r.Context(), "GET /checkout", traceapi.SpanKindServer, private)
		defer root.End()
		traceIDs <- root.SpanContext().TraceID().String()
		payments.log(ctx, "sdk-checkout-event", private)
		for _, request := range []struct{ address, operation string }{{stock.URL, "HTTP inventory"}, {external.URL, "HTTP partner"}} {
			callCtx, span := payments.start(ctx, request.operation, traceapi.SpanKindClient, private)
			req, err := http.NewRequestWithContext(callCtx, http.MethodGet, request.address, nil)
			if err != nil {
				span.End()
				http.Error(w, "request failed", 500)
				return
			}
			propagator.Inject(callCtx, propagation.HeaderCarrier(req.Header))
			response, err := http.DefaultClient.Do(req)
			if err == nil {
				_, _ = io.Copy(io.Discard, response.Body)
				response.Body.Close()
			}
			span.End()
			if err != nil || response.StatusCode != http.StatusNoContent {
				http.Error(w, "downstream failed", 500)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(checkout.Close)
	response, err := http.Get(checkout.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("actual SDK business request: %d", response.StatusCode)
	}
	traceID := <-traceIDs
	for _, service := range []*actualSDKService{payments, inventory, partner} {
		if service.traceExport.failure() != nil || service.logExport.failure() != nil {
			t.Fatal("real SDK did not export successfully")
		}
		if err := service.flushMetrics(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.waitRows(4, 2, 1, 1)
	verifySDKMetricSum(t, f, f.apps[0], 2)
	verifySDKMetricSum(t, f, f.apps[1], 1)
	services := map[string]uuid.UUID{}
	rows, err := f.store.Pool.Query(f.ctx, "SELECT service_namespace,id FROM monitoring_application_services WHERE enterprise_id=$1 AND application_id=$2", f.enterprise, f.apps[0].app)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		var id uuid.UUID
		if err = rows.Scan(&name, &id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		services[name] = id
	}
	rows.Close()
	if len(services) != 2 || services["payments"] == uuid.Nil || services["inventory"] == uuid.Nil || services["payments"] == services["inventory"] {
		t.Fatal("actual SDK service discovery merged same-name namespaces")
	}
	engine := skywalking.Engine{Conn: f.conn, Router: f.router}
	query := func(app sdkPipelineApplication, view string, variables map[string]any) map[string]any {
		t.Helper()
		result, err := engine.Execute(f.ctx, skywalking.Request{Document: skywalking.ApplicationAPMQueries()[view], OperationName: "ApplicationAPM", Variables: variables, Start: time.Now().Add(-time.Minute), End: time.Now().Add(time.Second), Scope: skywalking.Scope{EnterpriseID: f.enterprise, ResourceIDs: []uuid.UUID{app.app}, SourceKeys: []string{app.source + ":1"}}, Budget: skywalking.Budget{MaxRows: 1000, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}})
		if err != nil {
			t.Fatalf("actual SDK %s query: %v", view, err)
		}
		return result.Data
	}
	for _, test := range []struct{ view, root string }{{"services", "queryAPMServices"}, {"instances", "queryAPMInstances"}, {"endpoints", "queryAPMEndpoints"}, {"red", "queryAPMRED"}} {
		variables := map[string]any{"serviceIds": nil}
		if test.view == "red" {
			variables["bucketSeconds"] = 60
		}
		result := query(f.apps[0], test.view, variables)[test.root].(map[string]any)
		got := result["rows"].([]any)
		wantRows := 2
		if test.view == "endpoints" {
			wantRows = 4
		}
		if len(got) != wantRows {
			t.Fatalf("actual SDK %s rows=%d", test.view, len(got))
		}
		for _, value := range got {
			row := value.(map[string]any)
			if test.view == "endpoints" && strings.HasPrefix(row["operationName"].(string), "HTTP ") {
				if row["sampleCount"] != float64(0) || row["errorRate"] != nil || row["durationMeanMs"] != nil {
					t.Fatal("client-only operation fabricated request statistics")
				}
			} else if row["sampleCount"] != float64(1) {
				t.Fatalf("actual SDK %s counted client spans as requests", test.view)
			}
			if row["serviceId"] != services[row["serviceNamespace"].(string)].String() {
				t.Fatalf("actual SDK %s request counting or service identity wrong", test.view)
			}
		}
	}
	topology := query(f.apps[0], "topology", map[string]any{"serviceIds": nil})["queryAPMTopology"].(map[string]any)
	if len(topology["nodes"].([]any)) != 2 || len(topology["edges"].([]any)) != 1 {
		t.Fatalf("actual same-name service topology: nodes=%d edges=%d", len(topology["nodes"].([]any)), len(topology["edges"].([]any)))
	}
	traces := query(f.apps[0], "traces", map[string]any{"serviceIds": nil})["queryTraces"].(map[string]any)
	if traces["total"] != float64(1) {
		t.Fatal("actual business HTTP request did not produce one scoped trace")
	}
	detail := query(f.apps[0], "trace_detail", map[string]any{"traceId": traceID, "sourceId": f.apps[0].source, "resourceId": f.apps[0].app.String()})
	raw := mustSDKJSON(t, detail)
	if strings.Contains(string(raw), "partner-private-handler") || strings.Contains(string(raw), private) {
		t.Fatal("application Trace details exposed another app or private attribute")
	}
	trace := detail["queryTrace"].(map[string]any)
	if len(trace["spans"].([]any)) != 4 {
		t.Fatalf("authorized trace spans=%d", len(trace["spans"].([]any)))
	}
	other := query(f.apps[1], "trace_detail", map[string]any{"traceId": traceID, "sourceId": f.apps[1].source, "resourceId": f.apps[1].app.String()})["queryTrace"].(map[string]any)
	if len(other["spans"].([]any)) != 1 || other["rootPresent"] != false {
		t.Fatal("partial cross-application trace included unauthorized parents")
	}
	for namespace, serviceID := range services {
		logs, err := f.logs(f.apps[0].app, []string{f.apps[0].source + ":1"}, `resource_attributes.argus.service.id = "`+serviceID.String()+`" AND resource_attributes.deployment.environment.name = "production"`)
		if err != nil || len(logs) != 1 {
			t.Fatalf("SDK log filter %s rows=%d err=%v", namespace, len(logs), err)
		}
		if logs[0]["trace_id"] != traceID || strings.Contains(string(mustSDKJSON(t, logs)), private) {
			t.Fatal("SDK Log/Trace correlation or privacy failed")
		}
		if logs[0]["resource_id"] != f.apps[0].app {
			t.Fatal("SDK log did not inherit application environment and ownership")
		}
	}
	if hostLogs, e := f.logs(f.host, []string{f.apps[0].source + ":1"}, "*"); e != nil || len(hostLogs) != 0 {
		t.Fatal("SDK business logs leaked into execution-host scope")
	}
	verifyNativeSDKQueryEntries(t, f, traceID, services["inventory"])
	f.exec("UPDATE monitoring_ingest_keys SET revoked_at=now(),version=version+1 WHERE id=$1", f.apps[0].key)
	rejectedCtx, rejectedSpan := payments.start(f.ctx, "revoked-key-must-not-persist", traceapi.SpanKindServer, private)
	payments.log(rejectedCtx, "revoked-log-must-not-persist", private)
	rejectedSpan.End()
	if payments.traceExport.failure() == nil || payments.logExport.failure() == nil || payments.flushMetrics(f.ctx) == nil {
		t.Fatal("revoked key still accepted by actual SDK listener")
	}
	var rejected bool
	var rejectCode string
	if err = f.store.Pool.QueryRow(f.ctx, "SELECT last_rejected_at IS NOT NULL,last_error_code FROM monitoring_ingest_endpoints WHERE id=$1 AND enterprise_id=$2", f.apps[0].endpoint, f.enterprise).Scan(&rejected, &rejectCode); err != nil || !rejected || rejectCode != "APPLICATION_INGEST_KEY_REVOKED" {
		t.Fatal("actual rejected SDK did not update its authenticated endpoint diagnostic", err)
	}
	rotated := f.apps[0]
	rotated.key = uuid.New()
	rotated.token = sdkPipelineToken(t, rotated.key)
	digest := sha256.Sum256([]byte(rotated.token))
	f.exec("INSERT INTO monitoring_ingest_keys(id,enterprise_id,endpoint_id,application_id,key_prefix,key_digest,expires_at,created_by) SELECT $1,enterprise_id,endpoint_id,application_id,'owned-rotation',$2,now()+interval '1 hour',created_by FROM monitoring_ingest_keys WHERE id=$3", rotated.key, digest[:], f.apps[0].key)
	recovered := newActualSDKService(t, f, rotated, "payments")
	recoveryCtx, recoverySpan := recovered.start(f.ctx, "GET /rotated", traceapi.SpanKindServer, private)
	recovered.log(recoveryCtx, "sdk-rotated-event", private)
	recoverySpan.End()
	if recovered.traceExport.failure() != nil || recovered.logExport.failure() != nil || recovered.flushMetrics(f.ctx) != nil {
		t.Fatal("rotated application key did not recover actual SDK export")
	}
	f.waitRows(5, 3, 1, 1)
	logs, err := f.logs(f.apps[0].app, []string{f.apps[0].source + ":1"}, "*")
	if err != nil || strings.Contains(string(mustSDKJSON(t, logs)), "revoked-log-must-not-persist") {
		t.Fatal("revoked records entered persistent application logs")
	}
	var discovered, instances int
	if err = f.store.Pool.QueryRow(f.ctx, "SELECT count(*) FROM monitoring_application_services WHERE application_id=$1", f.apps[0].app).Scan(&discovered); err != nil {
		t.Fatal(err)
	}
	if err = f.store.Pool.QueryRow(f.ctx, "SELECT count(*) FROM monitoring_runtime_instances i JOIN monitoring_application_services s ON s.id=i.service_id WHERE s.application_id=$1", f.apps[0].app).Scan(&instances); err != nil {
		t.Fatal(err)
	}
	if discovered != 2 || instances != 2 {
		t.Fatal("rotation duplicated stable services or instances")
	}
	probeCtx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	seenTopics := map[string]bool{}
	for len(seenTopics) < 3 {
		fetches := f.probe.PollFetches(probeCtx)
		if fetches.Err() != nil {
			t.Fatal("no real Kafka records for SDK privacy check")
		}
		for _, record := range fetches.Records() {
			seenTopics[record.Topic] = true
			if strings.Contains(string(record.Value), private) || strings.Contains(string(record.Value), f.apps[0].token) {
				t.Fatal("SDK private input reached Kafka")
			}
		}
	}
	diagnostics, _ := f.d.command("logs", f.container)
	for _, secret := range []string{private, f.apps[0].token, f.apps[1].token, rotated.token} {
		if strings.Contains(diagnostics, secret) {
			t.Fatal("SDK native diagnostics exposed private input")
		}
	}
	t.Log("actual Go Trace/Log/Metric SDK HTTP calls -> native TLS listener -> real mTLS Key authority/discovery/Ingest -> SASL Kafka -> Writer/ClickHouse -> six APM/Trace views, correlated logs and PromQL; three-signal revocation denied and rotation preserved service identities; enrollment is a controlled fixture")
}

func (f *nativeSDKPipeline) logs(resource uuid.UUID, sources []string, expression string) ([]map[string]any, error) {
	result, err := kql.Execute(f.ctx, f.conn, f.router, kql.Request{Expression: expression, Pipeline: "limit 100", Start: time.Now().Add(-5 * time.Minute), End: time.Now().Add(time.Second), Scope: kql.Scope{EnterpriseID: f.enterprise, ResourceIDs: []uuid.UUID{resource}, SourceKeys: sources}, Budget: kql.Budget{MaxRows: 100, MaxScanBytes: 256 << 20, Timeout: 5 * time.Second}})
	return result.Data, err
}

func (f *nativeSDKPipeline) waitRows(app0Traces, app0Logs, app1Traces, app1Logs uint64) {
	f.t.Helper()
	tables, err := f.router.Tables(f.enterprise)
	if err != nil {
		f.t.Fatal(err)
	}
	deadline := time.Now().Add(35 * time.Second)
	for {
		var traces0, traces1, logs0, logs1 uint64
		e1 := f.conn.QueryRow(f.ctx, "SELECT countIf(resource_id=?),countIf(resource_id=?) FROM "+tables.Traces, f.apps[0].app, f.apps[1].app).Scan(&traces0, &traces1)
		e2 := f.conn.QueryRow(f.ctx, "SELECT countIf(resource_id=?),countIf(resource_id=?) FROM "+tables.Logs, f.apps[0].app, f.apps[1].app).Scan(&logs0, &logs1)
		if e1 != nil || e2 != nil {
			f.t.Fatalf("SDK persisted sample query: %v %v", e1, e2)
		}
		if traces0 == app0Traces && logs0 == app0Logs && traces1 == app1Traces && logs1 == app1Logs {
			return
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("SDK persisted coverage got %d/%d %d/%d want %d/%d %d/%d", traces0, logs0, traces1, logs1, app0Traces, app0Logs, app1Traces, app1Logs)
		}
		time.Sleep(300 * time.Millisecond)
	}
}
