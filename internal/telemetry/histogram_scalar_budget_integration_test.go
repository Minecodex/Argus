package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	promengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/prometheus/prometheus/promql"
	"github.com/twmb/franz-go/pkg/kgo"
	rpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestClickHouseHistogramScalarsDoNotReadBucketPayloads(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := t.Context()
	tenant, target, source := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), tenant) })
	at := time.Now().UTC().Truncate(time.Second)
	bounds, buckets := make([]float64, 16384), make([]uint64, 16385)
	for i := range bounds {
		bounds[i] = float64(i + 1)
	}
	buckets[0], buckets[1] = 1, 1
	sum := 2.0
	points := make([]*metricspb.HistogramDataPoint, 64)
	for i := range points {
		points[i] = &metricspb.HistogramDataPoint{TimeUnixNano: uint64(at.Add(time.Duration(i-63) * 10 * time.Second).UnixNano()),
			Count: 2, Sum: &sum, BucketCounts: buckets, ExplicitBounds: bounds}
	}
	payload := &rpc.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			{Key: "argus.source.id", Value: stringValue(source.String())},
			{Key: "argus.source.revision", Value: stringValue("1")},
			{Key: "argus.source.type", Value: stringValue("kubernetes_apiserver")},
		}}, ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{{Name: "wide_duration_seconds",
			Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{
				AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, DataPoints: points}}}}}},
	}}}
	if _, err := (&Writer{ClickHouse: conn, Router: router}).writeMetrics(ctx, &kgo.Record{Topic: "owned-scalar-budget"},
		TrustedIdentity{EnterpriseID: tenant, ResourceID: target, CollectorID: uuid.New()}, payload, time.Hour); err != nil {
		t.Fatal(err)
	}
	engine := promengine.NewEngine(conn, router, nil)
	for _, name := range []string{"wide_duration_seconds_count", "wide_duration_seconds_sum"} {
		result, err := engine.Execute(ctx, promengine.Request{Expression: name, Instant: true, Start: at.Add(-time.Hour), End: at,
			Scope:     promengine.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{target}, SourceKeys: []string{source.String() + ":1"}},
			MaxSeries: 100, MaxSamples: 1000, MaxScanBytes: 4 << 20, Timeout: 20 * time.Second})
		if err != nil {
			t.Fatalf("%s read irrelevant buckets: %v", name, err)
		}
		vector, ok := result.Value.(promql.Vector)
		if !ok || len(vector) != 1 || vector[0].F != 2 {
			t.Fatal("histogram scalar changed", result.Value)
		}
		t.Logf("%s: bytes=%d", name, result.ScannedBytes)
	}
}
