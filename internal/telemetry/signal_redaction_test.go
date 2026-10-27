package telemetry

import (
	"bytes"
	"testing"

	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func privateAttribute() []*commonpb.KeyValue {
	return []*commonpb.KeyValue{{Key: "password", Value: stringValue("signal-private-canary")}, {Key: "argus.application.key.id", Value: stringValue("nonsecret-key-reference")}}
}

func TestTraceCredentialsAreRemovedBeforeQueuedProtobuf(t *testing.T) {
	traceID, spanID := []byte("0123456789abcdef"), []byte("span0001")
	span := &tracepb.Span{TraceId: traceID, SpanId: spanID, Name: "GET /?token=operation-canary", TraceState: "vendor=value,token=state-canary", Attributes: privateAttribute(), Status: &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR, Message: "authorization: Bearer status-canary"}, Events: []*tracepb.Span_Event{{Name: "exception password=event-name-canary", Attributes: privateAttribute()}}, Links: []*tracepb.Span_Link{{TraceId: traceID, SpanId: spanID, TraceState: "token=link-state-canary", Attributes: privateAttribute()}}}
	request := &collecttraces.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: &resourcepb.Resource{Attributes: privateAttribute()}, ScopeSpans: []*tracepb.ScopeSpans{{Scope: &commonpb.InstrumentationScope{Name: "real-sdk", Attributes: privateAttribute()}, Spans: []*tracepb.Span{span}}}}}}
	if err := redactTracePayload(request); err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"signal-private-canary", "operation-canary", "state-canary", "status-canary", "event-name-canary", "link-state-canary"} {
		if bytes.Contains(raw, []byte(value)) {
			t.Fatalf("credential remained in trace %s", value)
		}
	}
	if !bytes.Equal(span.TraceId, traceID) || !bytes.Equal(span.SpanId, spanID) || !bytes.Contains(raw, []byte("nonsecret-key-reference")) || span.Status.Code != tracepb.Status_STATUS_CODE_ERROR {
		t.Fatal("trace identity, status or public provenance changed")
	}
}

func TestMetricCredentialsAreRemovedWithoutChangingMeasurements(t *testing.T) {
	exemplars := func() []*metricspb.Exemplar {
		return []*metricspb.Exemplar{{FilteredAttributes: privateAttribute(), TraceId: []byte("0123456789abcdef"), SpanId: []byte("span0001"), Value: &metricspb.Exemplar_AsInt{AsInt: 9223372036854775806}}}
	}
	number := func() *metricspb.NumberDataPoint {
		return &metricspb.NumberDataPoint{Attributes: privateAttribute(), Exemplars: exemplars(), Value: &metricspb.NumberDataPoint_AsInt{AsInt: 9223372036854775806}}
	}
	value := 2.5
	metrics := []*metricspb.Metric{
		{Name: "gauge", Metadata: privateAttribute(), Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{number()}}}},
		{Name: "sum", Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{IsMonotonic: true, AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, DataPoints: []*metricspb.NumberDataPoint{number()}}}},
		{Name: "histogram", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{DataPoints: []*metricspb.HistogramDataPoint{{Attributes: privateAttribute(), Exemplars: exemplars(), Count: 2, Sum: &value, BucketCounts: []uint64{1, 1}, ExplicitBounds: []float64{1.25}}}}}},
		{Name: "exponential", Data: &metricspb.Metric_ExponentialHistogram{ExponentialHistogram: &metricspb.ExponentialHistogram{DataPoints: []*metricspb.ExponentialHistogramDataPoint{{Attributes: privateAttribute(), Exemplars: exemplars(), Count: 2, Sum: &value, Scale: 2, Positive: &metricspb.ExponentialHistogramDataPoint_Buckets{Offset: 2, BucketCounts: []uint64{1, 1}}}}}}},
		{Name: "summary", Data: &metricspb.Metric_Summary{Summary: &metricspb.Summary{DataPoints: []*metricspb.SummaryDataPoint{{Attributes: privateAttribute(), Count: 2, Sum: value, QuantileValues: []*metricspb.SummaryDataPoint_ValueAtQuantile{{Quantile: 0.5, Value: 1.25}}}}}}},
	}
	request := &collectmetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcepb.Resource{Attributes: privateAttribute()}, ScopeMetrics: []*metricspb.ScopeMetrics{{Scope: &commonpb.InstrumentationScope{Attributes: privateAttribute()}, Metrics: metrics}}}}}
	if err := redactMetricPayload(request); err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("signal-private-canary")) || !bytes.Contains(raw, []byte("nonsecret-key-reference")) {
		t.Fatal("metric credentials or public provenance were not projected correctly")
	}
	if metrics[0].GetGauge().DataPoints[0].GetAsInt() != 9223372036854775806 || metrics[1].GetSum().DataPoints[0].Exemplars[0].GetAsInt() != 9223372036854775806 {
		t.Fatal("integer measurement precision changed")
	}
	// Forwarding/retry may project a payload again. That must not change already
	// masked attributes, public identifiers, or the measurements.
	once := proto.Clone(request)
	if err := redactMetricPayload(request); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(request, once) {
		t.Fatal("repeated projection changed the payload")
	}
	if got := metrics[2].GetHistogram().DataPoints[0]; got.Count != 2 || got.GetSum() != value || len(got.BucketCounts) != 2 || got.BucketCounts[1] != 1 || got.ExplicitBounds[0] != 1.25 {
		t.Fatal("histogram changed")
	}
	if metrics[3].GetExponentialHistogram().DataPoints[0].Positive.Offset != 2 || metrics[4].GetSummary().DataPoints[0].QuantileValues[0].Value != 1.25 {
		t.Fatal("exponential histogram or summary changed")
	}
}
