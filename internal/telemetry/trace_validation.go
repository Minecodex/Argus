package telemetry

import (
	"math"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// OTLP requires nonzero IDs and meaningful start/end times. Invalid spans
// cannot participate in deduplication or become fabricated APM durations.
func validTraceSpan(span *tracepb.Span) bool {
	if span == nil || !validTraceID(span.TraceId, 16) || !validTraceID(span.SpanId, 8) {
		return false
	}
	if len(span.ParentSpanId) > 0 && !validTraceID(span.ParentSpanId, 8) {
		return false
	}
	if span.StartTimeUnixNano == 0 || span.EndTimeUnixNano < span.StartTimeUnixNano || span.EndTimeUnixNano > math.MaxInt64 {
		return false
	}
	if span.Kind < tracepb.Span_SPAN_KIND_UNSPECIFIED || span.Kind > tracepb.Span_SPAN_KIND_CONSUMER {
		return false
	}
	if span.Status != nil && (span.Status.Code < tracepb.Status_STATUS_CODE_UNSET || span.Status.Code > tracepb.Status_STATUS_CODE_ERROR) {
		return false
	}
	for _, link := range span.Links {
		if link == nil || !validTraceID(link.TraceId, 16) || !validTraceID(link.SpanId, 8) {
			return false
		}
	}
	return true
}

func validTraceID(value []byte, size int) bool {
	if len(value) != size {
		return false
	}
	for _, b := range value {
		if b != 0 {
			return true
		}
	}
	return false
}
