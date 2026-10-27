package telemetry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"github.com/prometheus/prometheus/promql"
	"github.com/twmb/franz-go/pkg/kgo"
	logsrpc "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsrpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracesrpc "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestClickHouseSignalWindowsKeepSubsecondBoundaries(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := t.Context()
	tenant, host, source := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.DropTenant(context.Background(), tenant); err != nil {
			t.Error(err)
		}
	})
	identity := TrustedIdentity{EnterpriseID: tenant, ResourceID: host, CollectorID: uuid.New()}
	writer := Writer{ClickHouse: conn, Router: router}
	at := time.Now().UTC().Truncate(time.Second).Add(123 * time.Millisecond)
	resource := func() *resourcepb.Resource {
		return &resourcepb.Resource{Attributes: []*common.KeyValue{{Key: "argus.source.id", Value: stringValue(source.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "argus.source.type", Value: stringValue("otlp")}, {Key: "service.name", Value: stringValue("precision")}}}
	}
	metric := &metricspb.Metric{Name: "precision_metric", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{}}}
	logs := &logsrpc.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: resource(), ScopeLogs: []*logspb.ScopeLogs{{}}}}}
	traces := &tracesrpc.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: resource(), ScopeSpans: []*tracepb.ScopeSpans{{}}}}}
	for i, offset := range []time.Duration{-1, 0, 1} {
		metric.GetGauge().DataPoints = append(metric.GetGauge().DataPoints, &metricspb.NumberDataPoint{TimeUnixNano: uint64(at.Add(offset * time.Millisecond).UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: float64(i + 1)}})
		logs.ResourceLogs[0].ScopeLogs[0].LogRecords = append(logs.ResourceLogs[0].ScopeLogs[0].LogRecords, &logspb.LogRecord{TimeUnixNano: uint64(at.Add(offset).UnixNano()), Body: stringValue("precision")})
		spanID := []byte("span0001")
		spanID[7] += byte(i)
		traces.ResourceSpans[0].ScopeSpans[0].Spans = append(traces.ResourceSpans[0].ScopeSpans[0].Spans, &tracepb.Span{TraceId: []byte("0123456789abcdef"), SpanId: spanID, Name: "precision", Kind: tracepb.Span_SPAN_KIND_SERVER, StartTimeUnixNano: uint64(at.Add(offset).UnixNano()), EndTimeUnixNano: uint64(at.Add(time.Millisecond).UnixNano())})
	}
	metrics := &metricsrpc.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: resource(), ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{metric}}}}}}
	if _, err := writer.writeMetrics(ctx, &kgo.Record{Topic: "otlp-metrics", Offset: 1}, identity, metrics, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.writeLogs(ctx, &kgo.Record{Topic: "otlp-logs", Offset: 1}, identity, logs, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.writeTraces(ctx, &kgo.Record{Topic: "otlp-traces", Offset: 1}, identity, traces, time.Hour); err != nil {
		t.Fatal(err)
	}
	keys := []string{source.String() + ":1"}
	logResult, err := kql.Execute(ctx, conn, router, kql.Request{Expression: "*", Pipeline: "stats count()", Start: at, End: at.Add(time.Nanosecond), Scope: kql.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{host}, SourceKeys: keys}, Budget: kql.Budget{MaxRows: 10, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	if len(logResult.Data) != 1 || len(logResult.Data[0]) != 1 {
		t.Fatalf("fractional log count shape: %+v", logResult.Data)
	}
	for _, value := range logResult.Data[0] {
		if fmt.Sprint(value) != "1" {
			t.Fatalf("log half-open nanosecond window: %+v", logResult.Data)
		}
	}
	traceResult, err := (skywalking.Engine{Conn: conn, Router: router}).Execute(ctx, skywalking.Request{Document: `query{queryAPMServices{coverage{observedSpanCount requestSampleCount}}}`, Start: at, End: at.Add(time.Nanosecond), Scope: skywalking.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{host}, SourceKeys: keys}, Budget: skywalking.Budget{MaxRows: 10, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	coverage := traceResult.Data["queryAPMServices"].(map[string]any)["coverage"].(map[string]any)
	if coverage["observedSpanCount"] != float64(1) {
		t.Fatalf("trace half-open nanosecond window: %+v", coverage)
	}
	metricResult, err := promqlengine.NewEngine(conn, router, nil).Execute(ctx, promqlengine.Request{Expression: `count_over_time(precision_metric[1ms])`, Instant: true, Start: at.Add(-time.Second), End: at, Scope: promqlengine.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{host}, SourceKeys: keys}, MaxSeries: 10, MaxSamples: 100, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	vector, ok := metricResult.Value.(promql.Vector)
	if !ok || len(vector) != 1 || vector[0].F != 1 {
		t.Fatalf("Prometheus millisecond window: %+v", metricResult.Value)
	}
	counts, err := (ClickHouseQuery{Conn: conn, Router: router}).Overview(ctx, OverviewRequest{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{host}, SourceKeys: keys, From: at, To: at.Add(time.Nanosecond), MaxScanBytes: 256 << 20, Timeout: 10 * time.Second})
	if err != nil || counts.MetricPoints != 1 || counts.LogRecords != 1 || counts.Spans != 1 {
		t.Fatalf("three-signal precise overview: %+v %v", counts, err)
	}
}
