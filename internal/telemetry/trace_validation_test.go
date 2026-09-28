package telemetry

import (
	"testing"

	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestTraceIngestRejectsInvalidAPMSampleIdentityAndTime(t *testing.T) {
	valid := func() *tracepb.Span {
		return &tracepb.Span{TraceId: []byte("0123456789abcdef"), SpanId: []byte("rootspan"), StartTimeUnixNano: 1, EndTimeUnixNano: 2, Kind: tracepb.Span_SPAN_KIND_SERVER}
	}
	request := func(span *tracepb.Span) *collecttraces.ExportTraceServiceRequest {
		return &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{span}}}}}}
	}
	if err := validateTraces(request(valid())); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*tracepb.Span){
		func(s *tracepb.Span) { s.TraceId = make([]byte, 16) }, func(s *tracepb.Span) { s.SpanId = []byte("short") },
		func(s *tracepb.Span) { s.StartTimeUnixNano = 0 }, func(s *tracepb.Span) { s.EndTimeUnixNano = 0 }, func(s *tracepb.Span) { s.EndTimeUnixNano = ^uint64(0) },
		func(s *tracepb.Span) { s.ParentSpanId = make([]byte, 8) }, func(s *tracepb.Span) { s.Kind = 99 }, func(s *tracepb.Span) { s.Links = []*tracepb.Span_Link{{}} },
	} {
		s := valid()
		change(s)
		if err := validateTraces(request(s)); err == nil {
			t.Fatal("invalid span accepted into sample statistics")
		}
	}
	if err := validateTraces(request(nil)); err == nil {
		t.Fatal("nil span accepted")
	}
}
