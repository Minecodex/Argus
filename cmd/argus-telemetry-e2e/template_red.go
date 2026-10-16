//go:build m4e2e

package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"time"
)

func emitTemplateRED(ctx context.Context, endpoint string, transport credentials.TransportCredentials, resourceID string) error {
	start := time.Now().UTC().Truncate(time.Second).Add(-15 * time.Minute)
	request := templateREDRequest(start, resourceID)
	if err := exportWithRetry(ctx, endpoint, transport, 500*time.Millisecond, func(connection grpc.ClientConnInterface) error {
		_, err := collecttraces.NewTraceServiceClient(connection).Export(ctx, request)
		return err
	}); err != nil {
		return err
	}
	raw, _ := json.Marshal(map[string]any{"kind": "controlled-template-red-otlp", "series": 2, "observations": 1200, "first_sample_at": start.Format(time.RFC3339Nano), "last_sample_at": start.Add(599 * time.Second).Format(time.RFC3339Nano)})
	fmt.Println(string(raw))
	return nil
}

func templateREDRequest(start time.Time, resourceID string) *collecttraces.ExportTraceServiceRequest {
	request := &collecttraces.ExportTraceServiceRequest{}
	for group := 0; group < 2; group++ {
		service := fmt.Sprintf("argus-template-red-%c", 'a'+group)
		resource := &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "service.name", Value: stringValue(service)}, {Key: "service.instance.id", Value: stringValue(service + "-instance")}, {Key: "argus.resource.id", Value: stringValue(resourceID)}}}
		spans := []*tracepb.Span{}
		for i := 0; i < 600; i++ {
			traceID, spanID := make([]byte, 16), make([]byte, 8)
			binary.BigEndian.PutUint64(traceID[:8], uint64(start.UnixNano()))
			binary.BigEndian.PutUint64(traceID[8:], uint64(i+1))
			binary.BigEndian.PutUint64(spanID, uint64(group*600+i+1))
			var parent []byte
			// Alternate the entry service in separate traces. Each trace remains
			// a two-span tree; the observed service topology has two real edges.
			if group != i%2 {
				parent = make([]byte, 8)
				binary.BigEndian.PutUint64(parent, uint64((1-group)*600+i+1))
			}
			at := start.Add(time.Duration(i) * time.Second)
			duration := time.Duration(i%101+1) * time.Millisecond
			if i == 511 {
				duration = 900 * time.Millisecond
			}
			status := tracepb.Status_STATUS_CODE_OK
			if i%17 == 0 {
				status = tracepb.Status_STATUS_CODE_ERROR
			}
			kind := tracepb.Span_SPAN_KIND_SERVER
			if i == 200 {
				kind = tracepb.Span_SPAN_KIND_INTERNAL
			}
			spans = append(spans, &tracepb.Span{TraceId: traceID, SpanId: spanID, ParentSpanId: parent, Name: "GET /template/acceptance", Kind: kind, StartTimeUnixNano: uint64(at.UnixNano()), EndTimeUnixNano: uint64(at.Add(duration).UnixNano()), Status: &tracepb.Status{Code: status}})
		}
		request.ResourceSpans = append(request.ResourceSpans, &tracepb.ResourceSpans{Resource: resource, ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}}})
	}
	return request
}
