package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	promengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/prometheus/prometheus/promql"
	"github.com/twmb/franz-go/pkg/kgo"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestPromQLSuffixNamesKeepExactMetricsAndTypedFamilyProjections(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := t.Context()
	enterprise, resource, source := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, enterprise); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.DropTenant(context.Background(), enterprise); err != nil {
			t.Error(err)
		}
	})
	at := time.Now().UTC().Add(-time.Second)
	attrs := func(kind string) []*commonpb.KeyValue {
		return []*commonpb.KeyValue{{Key: "kind", Value: stringValue(kind)}}
	}
	metrics := []*metricspb.Metric{}
	for name, value := range map[string]int64{"plain": 99, "plain_count": 7, "plain_sum": 11, "plain_bucket": 13, "gauge": 99, "latency_count": 5, "latency_sum": 6, "latency_bucket": 7, "duration_count": 5, "duration_sum": 7, "duration_bucket": 17} {
		metrics = append(metrics, &metricspb.Metric{Name: name, Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{{TimeUnixNano: uint64(at.UnixNano()), Value: &metricspb.NumberDataPoint_AsInt{AsInt: value}, Attributes: attrs("native")}}}}})
	}
	histogramSum := 1.5
	metrics = append(metrics,
		&metricspb.Metric{Name: "latency", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{DataPoints: []*metricspb.HistogramDataPoint{{TimeUnixNano: uint64(at.UnixNano()), Count: 2, Sum: &histogramSum, BucketCounts: []uint64{1, 1}, ExplicitBounds: []float64{1}, Attributes: attrs("histogram")}}}}},
		&metricspb.Metric{Name: "duration", Data: &metricspb.Metric_Summary{Summary: &metricspb.Summary{DataPoints: []*metricspb.SummaryDataPoint{{TimeUnixNano: uint64(at.UnixNano()), Count: 2, Sum: 3, Attributes: attrs("summary")}}}}},
	)
	request := &collectmetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{Key: "argus.source.id", Value: stringValue(source.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "argus.source.type", Value: stringValue("otlp")}}}, ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: metrics}}}}}
	if _, err := (&Writer{ClickHouse: conn, Router: router}).writeMetrics(ctx, &kgo.Record{Topic: "otlp-metrics"}, TrustedIdentity{EnterpriseID: enterprise, ResourceID: resource, CollectorID: uuid.New()}, request, time.Hour); err != nil {
		t.Fatal(err)
	}
	engine := promengine.NewEngine(conn, router, nil)
	for expression, want := range map[string]float64{
		"sum(plain_count)": 7, "sum(plain_sum)": 11, "sum(plain_bucket)": 13,
		"sum(latency_count)": 7, "sum(latency_sum)": 7.5, "sum(latency_bucket)": 10,
		`latency_bucket{kind="native"}`: 7, `latency_count{kind="histogram"}`: 2,
		"sum(duration_count)": 7, "sum(duration_sum)": 10, "sum(duration_bucket)": 17,
	} {
		t.Run(expression, func(t *testing.T) {
			result, err := engine.Execute(ctx, promengine.Request{Expression: expression, Instant: true, Start: at.Add(-time.Minute), End: at.Add(time.Second), Scope: promengine.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{resource}, SourceKeys: []string{source.String() + ":1"}}})
			if err != nil {
				t.Fatal(err)
			}
			vector, ok := result.Value.(promql.Vector)
			if !ok || len(vector) != 1 || vector[0].F != want {
				t.Fatalf("suffix metric query got=%v want=%v", result.Value, want)
			}
		})
	}
	for _, name := range []string{"gauge_count", "gauge_sum", "gauge_bucket"} {
		result, err := engine.Execute(ctx, promengine.Request{Expression: name, Instant: true, Start: at.Add(-time.Minute), End: at.Add(time.Second), Scope: promengine.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{resource}}})
		if err != nil {
			t.Fatal(err)
		}
		if vector, ok := result.Value.(promql.Vector); !ok || len(vector) != 0 {
			t.Fatal("scalar base leaked into virtual histogram family")
		}
	}
}
