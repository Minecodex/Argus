package telemetry

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"github.com/twmb/franz-go/pkg/kgo"
	collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClickHouseQueryProjectionKeepsTraceAndLogDetails(t *testing.T) {
	address := os.Getenv("ARGUS_CLICKHOUSE_TEST_ADDRESS")
	if address == "" {
		t.Skip("requires ClickHouse")
	}
	conn, err := OpenClickHouse(address, "argus_telemetry", "argus", os.Getenv("ARGUS_CLICKHOUSE_TEST_PASSWORD"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tenant, host, subject := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err = manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	defer manager.DropTenant(context.Background(), tenant)
	at := time.Now().UTC().Truncate(time.Millisecond)
	attr := func(key, value string) *commonpb.KeyValue {
		return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
	}
	resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{attr("service.name", "api"), attr("business", "kept"), attr("api_key", "resource-secret")}}
	writer := Writer{ClickHouse: conn, Router: router}
	trusted := TrustedIdentity{EnterpriseID: tenant, ResourceID: host, CollectorID: uuid.New()}
	logs := &collectlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: resource, ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{TimeUnixNano: uint64(at.UnixNano()), Body: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "completed password=body-secret duration=12"}}}}}}}}}
	if _, err = writer.writeLogs(ctx, &kgo.Record{Topic: "otlp-logs", Offset: 1}, trusted, logs, time.Hour); err != nil {
		t.Fatal(err)
	}
	traceID := []byte("0123456789abcdef")
	span := &tracepb.Span{TraceId: traceID, SpanId: []byte("12345678"), Name: "GET /work", StartTimeUnixNano: uint64(at.UnixNano()), EndTimeUnixNano: uint64(at.Add(time.Second).UnixNano()), Attributes: []*commonpb.KeyValue{attr("db.statement", "select ordinary"), attr("password", "span-secret")}}
	span.Events = []*tracepb.Span_Event{{Name: "finished", TimeUnixNano: uint64(at.UnixNano()), Attributes: []*commonpb.KeyValue{attr("event.note", "kept")}}}
	span.Links = []*tracepb.Span_Link{{TraceId: []byte("fedcba9876543210"), SpanId: []byte("87654321"), Attributes: []*commonpb.KeyValue{attr("linked.note", "kept")}}}
	traces := &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: resource, ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{span}}}}}}

	if _, err = writer.writeTraces(ctx, &kgo.Record{Topic: "otlp-traces", Offset: 1}, trusted, traces, time.Hour); err != nil {
		t.Fatal(err)
	}
	coordinator := &queryengine.Coordinator{KQL: queryengine.KQLEngine{Conn: conn, Router: router}, Trace: queryengine.TraceEngine{Engine: skywalking.Engine{Conn: conn, Router: router}}}
	request := queryengine.Request{Language: queryengine.LanguageKQL, Expression: "*", Start: at.Add(-time.Minute), End: at.Add(time.Minute), Scope: queryengine.Scope{EnterpriseID: tenant, SubjectID: subject, SubjectType: "user", ResourceIDs: []uuid.UUID{host}}, Budget: queryengine.Budget{MaxScanBytes: DefaultMaxScanBytes, MaxRows: 100, MaxResultBytes: 8 << 20, Timeout: time.Second * 10}}
	result, err := coordinator.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result.Data)
	if strings.Contains(string(raw), "body-secret") || !strings.Contains(string(raw), "completed") || !strings.Contains(string(raw), "duration=12") {
		t.Fatalf("log projection erased diagnostics or leaked credentials: %s", raw)
	}
	request.Language = queryengine.LanguageTrace
	request.Expression = `query {queryTrace(traceId:"` + hex.EncodeToString(traceID) + `"){traceId spans {spanId attributes resourceAttributes events links}}}`
	result, err = coordinator.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(result.Data)
	for _, secret := range []string{"resource-secret", "span-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("credential survived projection: %s", raw)
		}
	}
	for _, ordinary := range []string{"select ordinary", "finished", "linked.note", "kept"} {
		if !strings.Contains(string(raw), ordinary) {
			t.Fatalf("ordinary trace detail was hidden: %s", raw)
		}
	}
}
