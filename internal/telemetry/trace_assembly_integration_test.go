package telemetry

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"github.com/twmb/franz-go/pkg/kgo"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestTraceClickHouseCrossBatchAssembly(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := context.Background()
	tenant, host, source := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(ctx, tenant) })
	writer := Writer{ClickHouse: conn, Router: router}
	identity := TrustedIdentity{EnterpriseID: tenant, ResourceID: host, CollectorID: uuid.New()}
	traceID := []byte("0123456789abcdef")
	rootID, childID := []byte("rootspan"), []byte("childspn")
	at := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	span := func(id, parent []byte, name string, offset time.Duration) *tracepb.Span {
		return &tracepb.Span{TraceId: traceID, SpanId: id, ParentSpanId: parent, Name: name, StartTimeUnixNano: uint64(at.Add(offset).UnixNano()), EndTimeUnixNano: uint64(at.Add(offset + time.Second).UnixNano()), Status: &tracepb.Status{Code: tracepb.Status_STATUS_CODE_OK}}
	}
	send := func(origin uuid.UUID, revision, offset int64, spans ...*tracepb.Span) {
		t.Helper()
		request := &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "service.name", Value: stringValue("api")}, {Key: "argus.source.id", Value: stringValue(origin.String())}, {Key: "argus.source.revision", Value: stringValue(strconv.FormatInt(revision, 10))}, {Key: "argus.source.type", Value: stringValue("otlp")}}}, ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}}}}}
		if _, err := writer.writeTraces(ctx, &kgo.Record{Topic: "otlp-traces", Offset: offset}, identity, request, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	child := span(childID, rootID, "child", 100*time.Millisecond)
	child.Events = []*tracepb.Span_Event{{Name: "checkpoint", TimeUnixNano: uint64(at.UnixNano())}}
	child.Links = []*tracepb.Span_Link{{TraceId: []byte("fedcba9876543210"), SpanId: []byte("linkedsp")}}
	request := skywalking.Request{Document: `query {queryTrace(traceId:"` + hex.EncodeToString(traceID) + `"){traceId sourceId resourceId rootOperation rootPresent rootCount spanCount errorCount missingParentCount completeness spans{spanId operationName events links resourceAttributes} edges{parentSpanId childSpanId parentService depth missingParent cycle}}}`, Start: at.Add(-time.Second), End: at.Add(time.Minute), Scope: skywalking.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{host}, SourceKeys: []string{source.String() + ":1", source.String() + ":2"}}, Budget: skywalking.Budget{MaxRows: 100, MaxRelationExpansions: 3, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}}
	engine := skywalking.Engine{Conn: conn, Router: router}
	query := func() map[string]any {
		t.Helper()
		result, err := engine.Execute(ctx, request)
		if err != nil {
			t.Fatalf("%v: %v", err, result.Errors)
		}
		return result.Data["queryTrace"].(map[string]any)
	}
	send(source, 1, 1, child)
	first := query()
	if first["spanCount"] != float64(1) || first["rootPresent"] != false || first["missingParentCount"] != float64(1) || first["completeness"] != "missing_spans" {
		t.Fatalf("orphan concealed: %+v", first)
	}
	send(source, 1, 2, span(rootID, nil, "actual-root", 0))
	send(source, 1, 3, child)
	assembled := query()
	if assembled["spanCount"] != float64(2) || assembled["rootOperation"] != "actual-root" || assembled["missingParentCount"] != float64(0) || assembled["completeness"] != "observed_consistent" {
		t.Fatalf("cross-batch assembly or dedup failed: %+v", assembled)
	}
	edge := assembled["edges"].([]any)[0].(map[string]any)
	if edge["depth"] != float64(1) || edge["parentService"] != "api" || edge["missingParent"] != false {
		t.Fatalf("late parent not connected: %+v", edge)
	}
	late := span([]byte("latespan"), childID, "late", 200*time.Millisecond)
	late.Status.Code = tracepb.Status_STATUS_CODE_ERROR
	send(source, 1, 4, late)
	child.Name = "updated-child"
	send(source, 2, 5, child)
	final := query()
	if final["spanCount"] != float64(3) || final["errorCount"] != float64(1) || final["sourceId"] != source.String() {
		t.Fatalf("late span/revision duplicated or lost: %+v", final)
	}
	encoded, _ := json.Marshal(final)
	if !strings.Contains(string(encoded), "checkpoint") || !strings.Contains(string(encoded), "linkedsp") && !strings.Contains(string(encoded), "bGlua2Vkc3A=") || !strings.Contains(string(encoded), "updated-child") {
		t.Fatalf("span details lost: %s", encoded)
	}
	limited := request
	limited.Budget.MaxRelationExpansions = 2
	if _, err := engine.Execute(ctx, limited); !errors.Is(err, skywalking.ErrBudget) {
		t.Fatalf("relation truncation was silent: %v", err)
	}
	other := uuid.New()
	send(other, 1, 6, span(rootID, nil, "separate-install", 0))
	request.Scope.SourceKeys = append(request.Scope.SourceKeys, other.String()+":1")
	if _, err := engine.Execute(ctx, request); err == nil {
		t.Fatal("ambiguous trace identities silently merged")
	}
	request.Document = `query {queryTrace(traceId:"` + hex.EncodeToString(traceID) + `",sourceId:"` + source.String() + `",resourceId:"` + host.String() + `"){spanCount}}`
	if selected := query(); selected["spanCount"] != float64(3) {
		t.Fatalf("explicit origin selection failed: %+v", selected)
	}
	request.Document = `query {queryTraces {total traces {traceId sourceId resourceId spanCount}}}`
	page, err := engine.Execute(ctx, request)
	if err != nil {
		t.Fatalf("%v %v", err, page.Errors)
	}
	if page.Data["queryTraces"].(map[string]any)["total"] != float64(2) {
		t.Fatal("same-ID source histories merged in list")
	}
	request.Document = `query {queryTraces(status:"ERROR"){total}}`
	page, err = engine.Execute(ctx, request)
	if err != nil || page.Data["queryTraces"].(map[string]any)["total"] != float64(1) {
		t.Fatalf("status filter did not match normalized spans: %+v %v", page, err)
	}
	request.Budget.MaxRelationExpansions = 10
	request.Document = `query {queryTraceGraph(traceId:"` + hex.EncodeToString(traceID) + `",sourceId:"` + source.String() + `",resourceId:"` + host.String() + `"){completeness excludedSpanCount spans{spanId sourceId} edges{parentSpanId childSpanId}}}`
	graph, err := engine.Execute(ctx, request)
	if err != nil {
		t.Fatalf("%v %v", err, graph.Errors)
	}
	fragment := graph.Data["queryTraceGraph"].(map[string]any)
	if len(fragment["spans"].([]any)) != 3 || fragment["excludedSpanCount"] != float64(1) || fragment["completeness"] != "ambiguous_fragments" {
		t.Fatalf("unconnected installation was silently joined: %+v", fragment)
	}
}
