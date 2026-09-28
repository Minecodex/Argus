package telemetry

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"github.com/prometheus/prometheus/promql"
	"github.com/twmb/franz-go/pkg/kgo"
	collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func sourceTestClickHouse(t *testing.T) driver.Conn {
	t.Helper()
	address := os.Getenv("ARGUS_CLICKHOUSE_TEST_ADDRESS")
	if address == "" {
		t.Skip("ARGUS_CLICKHOUSE_TEST_ADDRESS is not configured")
	}
	conn, err := OpenClickHouse(address, envOrDefault("ARGUS_CLICKHOUSE_TEST_DATABASE", "argus_telemetry"), envOrDefault("ARGUS_CLICKHOUSE_TEST_USERNAME", "argus"), envOrDefault("ARGUS_CLICKHOUSE_TEST_PASSWORD", "argus"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestClickHouseSourceIsolationAcrossSignals(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := context.Background()
	enterprise, resource := uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, enterprise); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), enterprise) })
	now := time.Now().UTC().Truncate(time.Second)
	writer := Writer{ClickHouse: conn, Router: router}
	identity := TrustedIdentity{EnterpriseID: enterprise, ResourceID: resource, CollectorID: uuid.New()}
	sources := []SourceIdentity{{ID: uuid.New(), Revision: 1, Type: "otlp"}, {ID: uuid.New(), Revision: 1, Type: "otlp"}}
	traceID, spanID := []byte("0123456789abcdef"), []byte("rootspan")
	for index, source := range sources {
		attrs := []*commonpb.KeyValue{{Key: "argus.source.id", Value: stringValue(source.ID.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "argus.source.type", Value: stringValue(source.Type)}, {Key: "service.name", Value: stringValue("same-name")}}
		metadata := &resourcepb.Resource{Attributes: attrs}
		record := &kgo.Record{Topic: "otlp-logs", Offset: int64(index + 1)}
		logs := &collectlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: metadata, ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{TimeUnixNano: uint64(now.UnixNano()), Body: stringValue(source.ID.String())}}}}}}}
		if _, err := writer.writeLogs(ctx, record, identity, logs, time.Hour); err != nil {
			t.Fatal(err)
		}
		record.Topic = "otlp-metrics"

		point := &metricspb.NumberDataPoint{TimeUnixNano: uint64(now.UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: float64(index + 1)}}
		metric := &metricspb.Metric{Name: "source_value", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{point}}}}
		metrics := &collectmetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: metadata, ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{metric}}}}}}

		if _, err := writer.writeMetrics(ctx, record, identity, metrics, time.Hour); err != nil {
			t.Fatal(err)
		}
		record.Topic = "otlp-traces"

		span := &tracepb.Span{TraceId: traceID, SpanId: spanID, Name: source.ID.String(), StartTimeUnixNano: uint64(now.UnixNano()), EndTimeUnixNano: uint64(now.Add(time.Second).UnixNano()), Attributes: []*commonpb.KeyValue{{Key: "source.marker", Value: stringValue(source.ID.String())}}}
		traces := &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: metadata, ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{span}}}}}}

		if _, err := writer.writeTraces(ctx, record, identity, traces, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	for index, source := range sources {
		logs, err := kql.Execute(ctx, conn, router, kql.Request{Expression: "*", Start: now.Add(-time.Minute), End: now.Add(time.Minute), Scope: kql.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{resource}, SourceKeys: []string{source.Key()}}, Budget: kql.Budget{MaxRows: 100, Timeout: time.Second * 10}})
		if err != nil || len(logs.Data) != 1 || logs.Data[0]["body"] != source.ID.String() {
			t.Fatalf("log origin escaped: %+v %v", logs, err)
		}
		metrics, err := promqlengine.NewEngine(conn, router, nil).Execute(ctx, promqlengine.Request{Expression: "sum(source_value)", Instant: true, Start: now.Add(-time.Minute), End: now.Add(time.Second), Scope: promqlengine.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{resource}, SourceKeys: []string{source.Key()}}})
		if err != nil {
			t.Fatal(err)
		}
		vector, ok := metrics.Value.(promql.Vector)
		if !ok || len(vector) != 1 || vector[0].F != float64(index+1) {
			t.Fatalf("metric sources merged: %v", metrics.Value)
		}
		catalog, e := (ClickHouseQuery{Conn: conn, Router: router}).DiscoverData(ctx, DataCatalogRequest{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{resource}, SourceKeys: []string{source.Key()}, Signal: "metrics", Kind: "metrics", From: now.Add(-time.Minute), To: now.Add(time.Minute), Limit: 10})
		if e != nil || len(catalog.Metrics) != 1 || catalog.Metrics[0].Name != "source_value" || catalog.Metrics[0].Type != "gauge" {
			t.Fatalf("observed metric discovery failed: %+v %v", catalog, e)
		}
		query := skywalking.Request{Document: `query { queryTrace(traceId: "` + hex.EncodeToString(traceID) + `") { spans { operationName } } }`, Start: now.Add(-time.Minute), End: now.Add(time.Minute), Scope: skywalking.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{resource}, SourceKeys: []string{source.Key()}}, Budget: skywalking.Budget{MaxRows: 100, Timeout: time.Second * 10}}
		traces, err := (skywalking.Engine{Conn: conn, Router: router}).Execute(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(traces.Data)
		if !strings.Contains(string(encoded), source.ID.String()) || strings.Contains(string(encoded), sources[1-index].ID.String()) {
			t.Fatalf("trace source collision: %s", encoded)
		}
		query.Document = `query { queryBasicTraces(tags: [{key: "source.marker",value:"` + sources[1-index].ID.String() + `"}]) { total } }`
		traces, err = (skywalking.Engine{Conn: conn, Router: router}).Execute(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		page := traces.Data["queryBasicTraces"].(map[string]any)
		if page["total"].(float64) != 0 {
			t.Fatal("foreign source influenced membership")
		}
	}
}

func TestMissingLogTimestampRetryIsIdempotent(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := context.Background()
	enterprise, resource := uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, enterprise); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), enterprise) })
	writer := Writer{ClickHouse: conn, Router: router}
	record := &kgo.Record{Topic: "otlp-logs", Offset: 1, Timestamp: time.Now().UTC().Add(-time.Minute)}
	request := &collectlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{Body: stringValue("timestamp unavailable")}}}}}}}
	identity := TrustedIdentity{EnterpriseID: enterprise, ResourceID: resource, CollectorID: uuid.New()}
	for i := 0; i < 2; i++ {
		if _, err := writer.writeLogs(ctx, record, identity, request, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	tables, _ := router.Tables(enterprise)
	var count uint64
	if err := conn.QueryRow(ctx, "SELECT count() FROM `"+tables.Logs+"` FINAL").Scan(&count); err != nil || count != 1 {
		t.Fatalf("retry duplicated a record with missing timestamp: %d %v", count, err)
	}
}

func TestClickHouseSourceUpgradePreservesExistingRows(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := context.Background()
	enterprise := uuid.New()
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: TenantTableRouter{}}
	tables, _ := manager.Router.Tables(enterprise)
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), enterprise) })
	for _, ddl := range tenantTableDDL(tables) {
		// Reconstruct v3 physical tables to exercise the actual ALTER migration.
		ddl = strings.ReplaceAll(ddl, ",\nsource_id UUID, source_revision Int64, source_type LowCardinality(String), source_key String MATERIALIZED concat(toString(source_id), ':', toString(source_revision))", "")
		ddl = strings.ReplaceAll(ddl, ", source_id, source_revision) TTL", ") TTL")
		ddl = strings.ReplaceAll(ddl, "resource_attributes Map(String,String), body String", "body String")
		if err := conn.Exec(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	if err := conn.Exec(ctx, "INSERT INTO `"+tables.Logs+"` (resource_id,collector_id,timestamp,body,event_id,expires_at) VALUES (?,?,now64(9),'kept','old-event',now64(3)+INTERVAL 1 HOUR)", uuid.New(), uuid.New()); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnsureTenant(ctx, enterprise); err != nil {
		t.Fatal(err)
	}
	if err := manager.VerifyTenant(ctx, enterprise); err != nil {
		t.Fatal(err)
	}
	var count uint64
	if err := conn.QueryRow(ctx, "SELECT count() FROM `"+tables.Logs+"` WHERE body='kept'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("upgrade lost data: count=%d err=%v", count, err)
	}
}
