package main

import (
	"context"
	"time"

	metricapi "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// Only the explicit semantics acceptance phase enables these instruments.
// Both cumulative observations use the real clock and authenticated exporter.
func emitMetricSemantics(ctx context.Context, provider *sdkmetric.MeterProvider, reader *sdkmetric.ManualReader, exporter sdkmetric.Exporter, signals bool) error {
	meter := provider.Meter("argus.plan11.actual-sdk")
	counter, err := meter.Int64Counter("argus_plan11_sdk_completed", metricapi.WithUnit("{request}"))
	if err != nil {
		return err
	}
	gauge, err := meter.Float64Gauge("argus_plan11_sdk_queue", metricapi.WithUnit("{request}"))
	if err != nil {
		return err
	}
	histogram, err := meter.Float64Histogram("argus_plan11_sdk_latency", metricapi.WithUnit("s"), metricapi.WithExplicitBucketBoundaries(.1, .5, 1))
	if err != nil {
		return err
	}
	if err := emitMetricConflicts(ctx, provider); err != nil {
		return err
	}
	if signals {
		if err := emitSignalMetrics(ctx, provider); err != nil {
			return err
		}
	}
	export := func() error {
		var data metricdata.ResourceMetrics
		if err := reader.Collect(ctx, &data); err != nil {
			return err
		}
		return exporter.Export(ctx, &data)
	}
	counter.Add(ctx, 2)
	gauge.Record(ctx, 9)
	histogram.Record(ctx, .1)
	histogram.Record(ctx, .3)
	if err := export(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1100 * time.Millisecond):
	}
	counter.Add(ctx, 3)
	gauge.Record(ctx, 7)
	histogram.Record(ctx, .2)
	return export()
}

func emitMetricConflicts(ctx context.Context, provider *sdkmetric.MeterProvider) error {
	bytesGauge, err := provider.Meter("argus.plan11.variant-a").Float64Gauge("argus_plan11_sdk_variant", metricapi.WithUnit("By"))
	if err != nil {
		return err
	}
	secondsGauge, err := provider.Meter("argus.plan11.variant-b").Float64Gauge("argus_plan11_sdk_variant", metricapi.WithUnit("s"))
	if err != nil {
		return err
	}
	bytesCounter, err := provider.Meter("argus.plan11.variant-c").Int64Counter("argus_plan11_sdk_variant", metricapi.WithUnit("By"))
	if err != nil {
		return err
	}
	bytesGauge.Record(ctx, 1024)
	secondsGauge.Record(ctx, .4)
	bytesCounter.Add(ctx, 64)
	return nil
}
