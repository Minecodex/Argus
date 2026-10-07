package telemetry

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"github.com/twmb/franz-go/pkg/kgo"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

const apmTestFields = `basis status limited percentileMethod coverage{observedSpanCount requestSampleCount missingServiceCount missingInstanceCount missingOperationCount} rows{sourceId resourceId serviceName instanceId instanceName operationName timestamp intervalSeconds observedSpanCount sampleCount errorCount errorRate samplesPerSecond durationMeanMs durationP50Ms durationP95Ms durationP99Ms}`

type apmScanCounter struct {
	driver.Conn
	statements int
}

func (c *apmScanCounter) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	c.statements++
	return c.Conn.Query(ctx, query, args...)
}
func (c *apmScanCounter) QueryRow(ctx context.Context, query string, args ...any) driver.Row {
	c.statements++
	return c.Conn.QueryRow(ctx, query, args...)
}

func TestAPMClickHouseReceivedSamples(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := context.Background()
	tenant, host, source, otherSource, hiddenHost := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(ctx, tenant) })
	writer := Writer{ClickHouse: conn, Router: router}
	at := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Second)
	makeSpan := func(id string, seconds int, ms int, kind tracepb.Span_SpanKind, status tracepb.Status_StatusCode) *tracepb.Span {
		return &tracepb.Span{TraceId: []byte("0123456789abcdef"), SpanId: []byte(id), Name: "GET /orders", Kind: kind, StartTimeUnixNano: uint64(at.Add(time.Duration(seconds) * time.Second).UnixNano()), EndTimeUnixNano: uint64(at.Add(time.Duration(seconds)*time.Second + time.Duration(ms)*time.Millisecond).UnixNano()), Status: &tracepb.Status{Code: status}}
	}
	send := func(resource, origin uuid.UUID, service, instance string, offset int64, spans ...*tracepb.Span) {
		t.Helper()
		attrs := []*commonpb.KeyValue{{Key: "service.name", Value: stringValue(service)}, {Key: "service.instance.id", Value: stringValue(instance)}, {Key: "service.instance.name", Value: stringValue("display-name")}, {Key: "argus.source.id", Value: stringValue(origin.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "argus.source.type", Value: stringValue("otlp")}}
		environment := "prod"
		if origin == otherSource {
			environment = "stage"
		}
		attrs = append(attrs, &commonpb.KeyValue{Key: "deployment.environment.name", Value: stringValue(environment)})
		request := &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: &resourcepb.Resource{Attributes: attrs}, ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}}}}}
		if _, err := writer.writeTraces(ctx, &kgo.Record{Topic: "otlp-traces", Offset: offset}, TrustedIdentity{EnterpriseID: tenant, ResourceID: resource, CollectorID: uuid.New()}, request, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	first := makeSpan("server01", 1, 100, tracepb.Span_SPAN_KIND_SERVER, tracepb.Status_STATUS_CODE_OK)
	second := makeSpan("server02", 11, 300, tracepb.Span_SPAN_KIND_SERVER, tracepb.Status_STATUS_CODE_ERROR)
	internal := makeSpan("internal", 5, 9000, tracepb.Span_SPAN_KIND_INTERNAL, tracepb.Status_STATUS_CODE_ERROR)
	send(host, source, "api", "instance-1", 1, first, second, internal)
	send(host, source, "api", "instance-1", 2, first) // replay in a different Kafka record
	send(host, otherSource, "api", "instance-1", 3, makeSpan("server01", 1, 5000, tracepb.Span_SPAN_KIND_SERVER, tracepb.Status_STATUS_CODE_ERROR))
	send(hiddenHost, source, "api", "instance-1", 4, makeSpan("hidden01", 1, 5000, tracepb.Span_SPAN_KIND_SERVER, tracepb.Status_STATUS_CODE_ERROR))
	scans := &apmScanCounter{Conn: conn}
	engine := skywalking.Engine{Conn: scans, Router: router}
	request := skywalking.Request{Start: at, End: at.Add(15 * time.Second), Scope: skywalking.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{host}, SourceKeys: []string{source.String() + ":1"}}, Budget: skywalking.Budget{MaxRows: 100, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}}
	query := func(root string) map[string]any {
		t.Helper()
		request.Document = "query {result:" + root + " {" + apmTestFields + "}}"
		result, err := engine.Execute(ctx, request)
		if err != nil {
			t.Fatalf("%s: %v %v", root, err, result.Errors)
		}
		return result.Data["result"].(map[string]any)
	}
	services := query("queryAPMServices")
	if scans.statements != 1 {
		t.Fatalf("coverage and rows performed %d scans, expected one", scans.statements)
	}
	if empty := query(`queryAPMServices(serviceName:"")`); len(empty["rows"].([]any)) != 0 || empty["status"] != "no_data" {
		t.Fatal("literal empty service filter broadened to All")
	}
	rows := services["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("sources/resources merged: %+v", services)
	}
	r := rows[0].(map[string]any)
	if services["basis"] != "received_entry_spans" || r["sampleCount"] != float64(2) || r["observedSpanCount"] != float64(3) || r["errorCount"] != float64(1) || r["errorRate"] != 0.5 || r["durationMeanMs"] != float64(200) || r["durationP95Ms"] != float64(300) || math.Abs(r["samplesPerSecond"].(float64)-2.0/15) > 1e-10 {
		t.Fatalf("sample semantics incorrect: %+v", services)
	}
	if filtered := query(`queryAPMServices(filters:[{resource:true,key:"deployment.environment.name",values:["stage"]}])`); len(filtered["rows"].([]any)) != 0 {
		t.Fatal("environment filter ignored")
	}
	if all := query(`queryAPMServices(filters:[{resource:true,key:"deployment.environment.name"}])`); len(all["rows"].([]any)) != 1 {
		t.Fatal("typed All attribute selection rejected")
	}
	parameterized := request
	parameterized.Document = `query($env:[String!]){result:queryAPMServices(filters:[{resource:true,key:"deployment.environment.name",values:$env}]){rows{sampleCount}}}`
	parameterized.Variables = map[string]any{"env": nil}
	all, err := engine.Execute(ctx, parameterized)
	if err != nil || len(all.Data["result"].(map[string]any)["rows"].([]any)) != 1 {
		t.Fatalf("native All variable failed: %+v %v", all, err)
	}
	instances := query("queryAPMInstances(serviceName:\"api\")")
	if instances["rows"].([]any)[0].(map[string]any)["instanceId"] != "instance-1" {
		t.Fatal("instance identity lost")
	}
	endpoints := query("queryAPMEndpoints(serviceInstanceName:\"instance-1\")")
	if endpoints["rows"].([]any)[0].(map[string]any)["operationName"] != "GET /orders" {
		t.Fatal("endpoint identity lost")
	}
	red := query("queryAPMRED(bucketSeconds:10)")
	points := red["rows"].([]any)
	if len(points) != 2 || points[0].(map[string]any)["samplesPerSecond"] != 0.1 || points[1].(map[string]any)["samplesPerSecond"] != 0.2 || points[1].(map[string]any)["intervalSeconds"] != float64(5) {
		t.Fatalf("partial final bucket denominator wrong: %+v", red)
	}
	request.Scope.SourceKeys = append(request.Scope.SourceKeys, otherSource.String()+":1")
	if len(query("queryAPMServices")["rows"].([]any)) != 2 {
		t.Fatal("same-named services from different sources merged")
	}
	limited := query("queryAPMServices(limit:1)")
	if limited["limited"] != true || len(limited["rows"].([]any)) != 1 {
		t.Fatal("explicit limit lost")
	}
	request.Budget.MaxRows = 2
	request.Document = "query {queryAPMServices {rows{serviceName}}}"
	if _, err := engine.Execute(ctx, request); !errors.Is(err, skywalking.ErrBudget) {
		t.Fatalf("system budget silently truncated APM: %v", err)
	}
	request.Budget.MaxRows = 100
	request.Scope.SourceKeys = []string{source.String() + ":1"}
	send(host, source, "internal-only", "", 5, makeSpan("unknown1", 5, 1000, tracepb.Span_SPAN_KIND_INTERNAL, tracepb.Status_STATUS_CODE_OK))
	missing := query("queryAPMServices(serviceName:\"internal-only\")")
	if missing["status"] != "insufficient_request_samples" || missing["rows"].([]any)[0].(map[string]any)["errorRate"] != nil || missing["rows"].([]any)[0].(map[string]any)["durationP95Ms"] != nil {
		t.Fatalf("non-request spans fabricated RED: %+v", missing)
	}
	missing = query("queryAPMInstances(serviceName:\"internal-only\")")
	if missing["status"] != "incomplete_dimensions" || len(missing["rows"].([]any)) != 0 {
		t.Fatalf("missing instance ID fabricated: %+v", missing)
	}
	// Topology joins only received parent IDs inside the supplied authorization
	// scope, including a different resource/collector of the same receiver type.
	parent := makeSpan("client01", 1, 400, tracepb.Span_SPAN_KIND_CLIENT, tracepb.Status_STATUS_CODE_OK)
	child := makeSpan("remote01", 2, 300, tracepb.Span_SPAN_KIND_SERVER, tracepb.Status_STATUS_CODE_ERROR)
	child.ParentSpanId = parent.SpanId
	send(host, source, "frontend", "front-1", 6, parent)
	send(hiddenHost, otherSource, "backend", "back-1", 7, child)
	request.Scope.ResourceIDs = []uuid.UUID{host, hiddenHost}
	request.Scope.SourceKeys = []string{source.String() + ":1", otherSource.String() + ":1"}
	request.Document = `query{queryAPMTopology(serviceName:"backend"){basis status coverage{missingParentCount ambiguousParentCount} nodes{id sourceId resourceId serviceName} edges{sourceNodeId targetNodeId sampleCount errorCount durationP95Ms}}}`
	graph, err := engine.Execute(ctx, request)
	if err != nil {
		t.Fatalf("topology: %v %v", err, graph.Errors)
	}
	view := graph.Data["queryAPMTopology"].(map[string]any)
	if len(view["nodes"].([]any)) != 2 || len(view["edges"].([]any)) != 1 || view["edges"].([]any)[0].(map[string]any)["durationP95Ms"] != float64(300) {
		t.Fatalf("authorized cross-resource parent not connected: %+v", view)
	}
	request.Scope.ResourceIDs = []uuid.UUID{hiddenHost}
	graph, err = engine.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	view = graph.Data["queryAPMTopology"].(map[string]any)
	if len(view["edges"].([]any)) != 0 || len(view["nodes"].([]any)) != 1 || view["coverage"].(map[string]any)["missingParentCount"] != float64(1) {
		t.Fatalf("topology escaped authorized resources: %+v", view)
	}
	request.Scope.ResourceIDs = []uuid.UUID{host, hiddenHost}
	request.Document = `query{queryAPMTopology(serviceName:"backend",filters:[{resource:true,key:"deployment.environment.name",values:["stage"]}]){nodes{serviceName} edges{sampleCount} coverage{missingParentCount}}}`
	graph, err = engine.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	view = graph.Data["queryAPMTopology"].(map[string]any)
	if len(view["edges"].([]any)) != 0 || len(view["nodes"].([]any)) != 1 || view["coverage"].(map[string]any)["missingParentCount"] != float64(1) {
		t.Fatal("topology attribute filter imported a parent from another environment")
	}
}
