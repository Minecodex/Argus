package telemetry

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	skywalking "github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
)

func TestSkyWalkingGraphQLClickHouseTraceQuery(t *testing.T) {
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
	enterpriseID, resourceID := uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, enterpriseID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), enterpriseID) })

	now := time.Now().UTC().Truncate(time.Millisecond)
	traceID := []byte("0123456789abcdef")
	rootID := []byte("rootspan")
	childID := []byte("childspn")
	request := &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "argus-m7-e2e"}}}}},
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{
			{TraceId: traceID, SpanId: rootID, Name: "argus-m7-e2e", StartTimeUnixNano: uint64(now.UnixNano()), EndTimeUnixNano: uint64(now.Add(time.Second).UnixNano())},
			{TraceId: traceID, SpanId: childID, ParentSpanId: rootID, Name: "argus-m7-e2e", StartTimeUnixNano: uint64(now.UnixNano()), EndTimeUnixNano: uint64(now.Add(500 * time.Millisecond).UnixNano())},
		}}},
	}}}
	writer := Writer{ClickHouse: conn, Router: router}
	identity := TrustedIdentity{EnterpriseID: enterpriseID, ResourceID: resourceID, CollectorID: uuid.New()}
	if _, err := writer.writeTraces(ctx, &kgo.Record{Topic: "otlp-traces", Partition: 0, Offset: 1}, identity, request, time.Hour); err != nil {
		t.Fatal(err)
	}

	result, err := (skywalking.Engine{Conn: conn, Router: router}).Execute(ctx, skywalking.Request{
		Document: `query { queryBasicTracesByName(serviceName: "argus-m7-e2e", operationName: "argus-m7-e2e", pageSize: 100) { total traces { traceId rootService rootOperation spans { spanId operationName } } } }`,
		Start:    now.Add(-time.Minute), End: now.Add(time.Minute), Scope: skywalking.Scope{EnterpriseID: enterpriseID, ResourceIDs: []uuid.UUID{resourceID}}, Budget: skywalking.Budget{MaxRows: 100, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second, MaxResultBytes: 8 << 20},
	})
	if err != nil {
		t.Fatalf("%v (graphql errors: %v)", err, result.Errors)
	}
	if result.LatestSampleAt == nil || !result.LatestSampleAt.Equal(now) {
		t.Fatalf("Span sample timestamp lost: %v", result.LatestSampleAt)
	}
	if result.Data == nil {
		t.Fatal("trace GraphQL result is empty")
	}

	// A hidden service participating in the same trace must not influence an
	// authorized resource's list through tag or instance membership queries.
	hiddenIdentity := identity
	hiddenIdentity.ResourceID = uuid.New()
	request.ResourceSpans[0].Resource.Attributes = append(request.ResourceSpans[0].Resource.Attributes,
		&commonpb.KeyValue{Key: "service.instance.id", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "hidden-instance"}}})
	request.ResourceSpans[0].ScopeSpans[0].Spans[0].Attributes = []*commonpb.KeyValue{
		{Key: "private.marker", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "hidden-value"}}},
	}
	if _, err := writer.writeTraces(ctx, &kgo.Record{Topic: "otlp-traces", Partition: 0, Offset: 2}, hiddenIdentity, request, time.Hour); err != nil {
		t.Fatal(err)
	}
	for _, filter := range []string{`serviceInstanceName: "hidden-instance"`, `tags: [{key: "private.marker", value: "hidden-value"}]`} {
		filtered, err := (skywalking.Engine{Conn: conn, Router: router}).Execute(ctx, skywalking.Request{
			Document: `query { queryBasicTraces(` + filter + `) { total traces { traceId } } }`,
			Start:    now.Add(-time.Minute), End: now.Add(time.Minute), Scope: skywalking.Scope{EnterpriseID: enterpriseID, ResourceIDs: []uuid.UUID{resourceID}},
			Budget: skywalking.Budget{MaxRows: 100, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second},
		})
		if err != nil {
			t.Fatalf("filter %s: %v (%v)", filter, err, filtered.Errors)
		}
		encoded, _ := json.Marshal(filtered.Data)
		var decoded struct {
			Page struct {
				Total int `json:"total"`
			} `json:"queryBasicTraces"`
		}
		if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.Page.Total != 0 {
			t.Fatalf("hidden resource influenced trace membership: %s (%v)", encoded, err)
		}
	}
}
