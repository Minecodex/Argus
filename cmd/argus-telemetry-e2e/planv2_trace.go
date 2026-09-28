//go:build m4e2e

package main

import (
	"context"
	"encoding/hex"
	"fmt"
	collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"time"
)

const linkedTraceID = "aabbccddeeff00112233445566778899"

func traceBytes(value string) []byte { decoded, _ := hex.DecodeString(value); return decoded }

func emitPlanV2Trace(ctx context.Context, endpoint string, transport credentials.TransportCredentials, role, generation string, at time.Time) error {
	if role != "frontend" && role != "backend" {
		return fmt.Errorf("invalid trace fixture role")
	}
	res := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "service.name", Value: stringValue("p2-" + role)}, {Key: "service.instance.id", Value: stringValue("p2-instance")}, {Key: "deployment.environment.name", Value: stringValue("p2-depth")}}}
	span := &tracepb.Span{TraceId: traceBytes(linkedTraceID), SpanId: traceBytes("1111111111111111"), Name: "GET /checkout", Kind: tracepb.Span_SPAN_KIND_SERVER, StartTimeUnixNano: uint64(at.UnixNano()), EndTimeUnixNano: uint64(at.Add(120 * time.Millisecond).UnixNano()), Status: &tracepb.Status{Code: tracepb.Status_STATUS_CODE_OK}}
	if role == "backend" {
		span.SpanId, span.ParentSpanId = traceBytes("2222222222222222"), traceBytes("1111111111111111")
		span.Name = "POST /orders"
		span.StartTimeUnixNano, span.EndTimeUnixNano = uint64(at.Add(10*time.Millisecond).UnixNano()), uint64(at.Add(70*time.Millisecond).UnixNano())
	}
	export := func(items ...*tracepb.Span) error {
		return exportWithRetry(ctx, endpoint, transport, 200*time.Millisecond, func(conn grpc.ClientConnInterface) error {
			_, err := collecttraces.NewTraceServiceClient(conn).Export(ctx, &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: res, ScopeSpans: []*tracepb.ScopeSpans{{Scope: &commonpb.InstrumentationScope{Name: "p2-depth"}, Spans: items}}}}})
			return err
		})
	}
	if err := export(span); err != nil {
		return err
	}
	// Duplicate identity in a separate OTLP batch updates facts without adding
	// another observed request. Event time is shared across late arrival order.
	span.Attributes = []*commonpb.KeyValue{{Key: "p2.phase", Value: stringValue("second-batch")}, {Key: "p2.generation", Value: stringValue(generation)}}
	span.Events = []*tracepb.Span_Event{{TimeUnixNano: span.StartTimeUnixNano, Name: "p2-checkpoint", Attributes: []*commonpb.KeyValue{{Key: "step", Value: stringValue("persist-order")}}}}
	span.Links = []*tracepb.Span_Link{{TraceId: traceBytes("ffeeddccbbaa99887766554433221100"), SpanId: traceBytes("4444444444444444"), Attributes: []*commonpb.KeyValue{{Key: "relation", Value: stringValue("async-followup")}}}}
	span.Status = &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR, Message: "controlled sample error"}
	if err := export(span); err != nil {
		return err
	}
	if role == "frontend" {
		orphan := &tracepb.Span{TraceId: traceBytes(linkedTraceID), SpanId: traceBytes("3333333333333333"), ParentSpanId: traceBytes("9999999999999999"), Name: "missing-parent", Kind: tracepb.Span_SPAN_KIND_INTERNAL, StartTimeUnixNano: uint64(at.Add(20 * time.Millisecond).UnixNano()), EndTimeUnixNano: uint64(at.Add(30 * time.Millisecond).UnixNano())}
		if err := export(orphan); err != nil {
			return err
		}
	}
	logs := []*logspb.LogRecord{}
	for i, name := range []string{"before", "anchor", "after"} {
		logs = append(logs, &logspb.LogRecord{TimeUnixNano: uint64(at.Add(time.Duration(10+i) * time.Millisecond).UnixNano()), SeverityText: "INFO", Body: stringValue(fmt.Sprintf("p2-depth %s %s %s", role, generation, name)), TraceId: span.TraceId, SpanId: span.SpanId})
	}
	return exportWithRetry(ctx, endpoint, transport, 200*time.Millisecond, func(conn grpc.ClientConnInterface) error {
		_, err := collectlogs.NewLogsServiceClient(conn).Export(ctx, &collectlogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: res, ScopeLogs: []*logspb.ScopeLogs{{Scope: &commonpb.InstrumentationScope{Name: "p2-depth"}, LogRecords: logs}}}}})
		return err
	})
}
