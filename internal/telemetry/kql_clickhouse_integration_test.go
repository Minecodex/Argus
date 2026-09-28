package telemetry

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"github.com/twmb/franz-go/pkg/kgo"
	collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestKQLClickHousePipelineSemantics(t *testing.T) {
	address := os.Getenv("ARGUS_CLICKHOUSE_TEST_ADDRESS")
	if address == "" {
		t.Skip("ARGUS_CLICKHOUSE_TEST_ADDRESS is not set")
	}
	conn, err := OpenClickHouse(address, envOrDefault("ARGUS_CLICKHOUSE_TEST_DATABASE", "argus_telemetry"), envOrDefault("ARGUS_CLICKHOUSE_TEST_USERNAME", "argus"), envOrDefault("ARGUS_CLICKHOUSE_TEST_PASSWORD", "argus"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	enterprise, resource := uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, enterprise); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), enterprise) })
	now := time.Now().UTC().Truncate(time.Minute)
	var records []*logspb.LogRecord
	for i, body := range []string{`{"duration":10}`, `{"duration":20}`, `{"duration":30}`} {
		records = append(records, &logspb.LogRecord{TimeUnixNano: uint64(now.Add(time.Duration(i) * time.Second).UnixNano()), Body: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: body}}})
	}
	payload := &collectlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "api"}}}}}, ScopeLogs: []*logspb.ScopeLogs{{LogRecords: records}}}}}
	writer := Writer{ClickHouse: conn, Router: router}
	identity := TrustedIdentity{EnterpriseID: enterprise, ResourceID: resource, CollectorID: uuid.New()}
	if _, err := writer.writeLogs(ctx, &kgo.Record{Topic: "otlp-logs", Offset: 1}, identity, payload, time.Hour); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		query, kind string
		rows        int
		partial     bool
		key         string
		value       any
	}{
		{`* | stats count() by bin(timestamp, 1m), service_name`, "timeseries", 1, false, "count", uint64(3)},
		{`* | parse json | stats avg(json.duration) by service_name`, "table", 1, false, "value", float64(20)},
		{`* | parse json | where json.duration >= 20 | stats count()`, "table", 1, false, "count", uint64(2)},
		{`* | parse json | where json.duration = 20 | stats count()`, "table", 1, false, "count", uint64(1)},
		{`* | parse json | where json.duration = "20" | stats count()`, "table", 1, false, "count", uint64(0)},
		{`*`, "log_entries", 2, true, "service_name", "api"},
		{`* | limit 2`, "log_entries", 2, false, "service_name", "api"},
		{`body : "*duration*" | stats count()`, "table", 1, false, "count", uint64(3)},
		{`service_name : "a?i" | stats count()`, "table", 1, false, "count", uint64(3)},
	} {
		result, err := kql.Execute(ctx, conn, router, kql.Request{Expression: test.query, Start: now.Add(-time.Second), End: now.Add(time.Minute), Scope: kql.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{resource}}, Budget: kql.Budget{MaxRows: 2, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}})
		if err != nil {
			t.Fatalf("%s: %v", test.query, err)
		}
		if test.kind != "log_entries" && result.LatestSampleAt != nil {
			t.Fatal("bucket/aggregate timestamp was reported as an original event")
		}
		if test.kind == "log_entries" && (result.LatestSampleAt == nil || !result.LatestSampleAt.Equal(now.Add(2*time.Second))) {
			t.Fatalf("log event time lost: %v", result.LatestSampleAt)
		}
		if result.ResultType != test.kind || len(result.Data) != test.rows || result.Partial != test.partial || result.Data[0][test.key] != test.value {
			t.Fatalf("%s: %+v", test.query, result)
		}
	}
	identity.ResourceID = uuid.New()
	payload.ResourceLogs[0].ScopeLogs[0].LogRecords = []*logspb.LogRecord{{TimeUnixNano: uint64(now.UnixNano()), Body: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: `message="connection refused" duration=15`}}}}
	if _, err := writer.writeLogs(ctx, &kgo.Record{Topic: "otlp-logs", Offset: 2}, identity, payload, time.Hour); err != nil {
		t.Fatal(err)
	}
	result, err := kql.Execute(ctx, conn, router, kql.Request{Expression: `* | parse logfmt | where logfmt.duration >= 10 | unwrap logfmt.message`, Start: now.Add(-time.Second), End: now.Add(time.Minute), Scope: kql.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{identity.ResourceID}}, Budget: kql.Budget{MaxRows: 2, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}})
	if err != nil || len(result.Data) != 1 || result.Data[0]["unwrap"] != "connection refused" {
		t.Fatalf("logfmt predicate and projection diverged: %+v %v", result, err)
	}
}
