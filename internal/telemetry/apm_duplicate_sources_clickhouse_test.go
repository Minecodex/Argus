package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"github.com/twmb/franz-go/pkg/kgo"
	rpc "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestAPMApplicationAggregatesSourcesWithoutCountingDuplicateSpans(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := t.Context()
	tenant, app, service, a, b := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), tenant) })
	at := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	identity := TrustedIdentity{EnterpriseID: tenant, ResourceID: app, ResourceType: "application", CollectorID: uuid.New()}
	writer := Writer{ClickHouse: conn, Router: router}
	send := func(origin uuid.UUID, protocol string, offset int64, ids ...string) {
		t.Helper()
		attrs := append(trustedAttributes(identity), []*commonpb.KeyValue{
			{Key: "argus.source.id", Value: stringValue(origin.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "argus.source.type", Value: stringValue(protocol)},
			{Key: "argus.application.id", Value: stringValue(app.String())}, {Key: "argus.service.id", Value: stringValue(service.String())},
			{Key: "service.name", Value: stringValue("orders")}, {Key: "service.namespace", Value: stringValue("commerce")}, {Key: "service.instance.id", Value: stringValue("replica-1")},
		}...)
		spans := []*tracepb.Span{}
		for _, id := range ids {
			code := tracepb.Status_STATUS_CODE_OK
			if id == "span0002" {
				code = tracepb.Status_STATUS_CODE_ERROR
			}
			spans = append(spans, &tracepb.Span{TraceId: []byte("0123456789abcdef"), SpanId: []byte(id), Name: "GET /orders", Kind: tracepb.Span_SPAN_KIND_SERVER, Status: &tracepb.Status{Code: code}, StartTimeUnixNano: uint64(at.UnixNano()), EndTimeUnixNano: uint64(at.Add(10 * time.Millisecond).UnixNano())})
		}
		request := &rpc.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: &resourcepb.Resource{Attributes: attrs}, ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}}}}}
		if _, err := writer.writeTraces(ctx, &kgo.Record{Topic: "otlp-traces", Offset: offset, Headers: []kgo.RecordHeader{{Key: "argus-normalization-version", Value: []byte("v3")}}}, identity, request, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	send(a, "otlp", 1, "span0001")
	send(b, "jaeger", 2, "span0001", "span0002")
	engine := skywalking.Engine{Conn: conn, Router: router}
	for _, all := range []bool{false, true} {
		keys := []string{a.String() + ":1"}
		wantSamples, wantSources, wantDuplicates := float64(1), 1, float64(0)
		if all {
			keys = append(keys, b.String()+":1")
			wantSamples, wantSources, wantDuplicates = 2, 2, 1
		}
		query := skywalking.Request{Document: `query{queryAPMServices{coverage{requestSampleCount duplicateObservationCount} rows{sourceId sourceIds serviceId sampleCount errorCount}}}`, Start: at.Add(-time.Second), End: at.Add(time.Second), Scope: skywalking.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{app}, SourceKeys: keys}, Budget: skywalking.Budget{MaxRows: 100, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}}
		result, err := engine.Execute(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		root := result.Data["queryAPMServices"].(map[string]any)
		rows := root["rows"].([]any)
		if len(rows) != 1 {
			t.Fatal("one stable application service split by collector", rows)
		}
		row := rows[0].(map[string]any)
		coverage := root["coverage"].(map[string]any)
		wantErrors := float64(0)
		if all {
			wantErrors = 1
		}
		if row["sampleCount"] != wantSamples || row["errorCount"] != wantErrors || len(row["sourceIds"].([]any)) != wantSources || coverage["duplicateObservationCount"] != wantDuplicates {
			t.Fatal("duplicate or unauthorized source affected APM statistics", root)
		}
	}
}
