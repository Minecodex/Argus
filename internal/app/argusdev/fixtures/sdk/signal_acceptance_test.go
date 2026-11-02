package main

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestSignalAcceptanceUsesActualNativeHistogramAndLiteralLabel(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), signalMetricView())
	defer func() { _ = provider.Shutdown(context.Background()) }()
	if err := emitSignalMetrics(t.Context(), provider); err != nil {
		t.Fatal(err)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	native, label := false, false
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			switch metric.Name {
			case "argus_plan11_sdk_native_latency":
				histogram, ok := metric.Data.(metricdata.ExponentialHistogram[float64])
				if !ok || len(histogram.DataPoints) != 1 || histogram.DataPoints[0].Count != 3 {
					t.Fatal("native instrument did not use SDK exponential histogram")
				}
				native = true
			case "argus_plan11_sdk_untrusted_note":
				gauge, ok := metric.Data.(metricdata.Gauge[float64])
				if !ok || len(gauge.DataPoints) != 1 {
					t.Fatal("untrusted note gauge missing")
				}
				value, ok := gauge.DataPoints[0].Attributes.Value(attribute.Key("plan11_note"))
				if !ok || value.AsString() != signalLabelMarker {
					t.Fatal("untrusted label was changed")
				}
				label = true
			}
		}
	}
	if !native || !label {
		t.Fatal("declared actual SDK observations missing")
	}
}

func TestSignalLogMarkerCannotCollideWithOrdinarySDKLookup(t *testing.T) {
	initial := "plan8-sdk-otlp_http-initial"
	rotated := "plan8-sdk-otlp_http-rotated"
	for _, ordinary := range []string{initial, rotated} {
		extra := signalLogMarker(ordinary)
		if !strings.HasPrefix(extra, "plan11-untrusted-log:") || strings.Contains(extra, ordinary) {
			t.Fatal("injectedfixture contaminated ordinarylog query")
		}
	}
	if signalLogMarker(initial) == signalLogMarker(rotated) {
		t.Fatal("distinct real SDK sends lost their own marker identity")
	}
}
