package telemetry

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	promengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/prometheus/prometheus/promql"
	"github.com/twmb/franz-go/pkg/kgo"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

func TestClickHouseObservedHistogramMethodsExecuteOnTheirActualRepresentation(t *testing.T) {
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
	at := time.Now().UTC().Truncate(time.Second)
	request := mixedMetricTypesRequest(uint64(at.UnixNano()))
	request.ResourceMetrics[0].Resource.Attributes = []*commonpb.KeyValue{
		{Key: "argus.source.id", Value: stringValue(source.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "argus.source.type", Value: stringValue("otlp")},
	}
	metrics := request.ResourceMetrics[0].ScopeMetrics[0]
	for _, metric := range metrics.Metrics {
		metric.Unit = "s"
	}
	sum := .6
	metrics.Metrics = append(metrics.Metrics, &metricspb.Metric{Name: "argus.test.classic.histogram", Unit: "s", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, DataPoints: []*metricspb.HistogramDataPoint{{TimeUnixNano: uint64(at.UnixNano()), Count: 3, Sum: &sum, BucketCounts: []uint64{1, 2, 0, 0}, ExplicitBounds: []float64{.1, .5, 1}}}}}})
	if _, err := (&Writer{ClickHouse: conn, Router: router}).writeMetrics(ctx, &kgo.Record{Topic: "otlp-metrics"}, TrustedIdentity{EnterpriseID: enterprise, ResourceID: resource, CollectorID: uuid.New()}, request, time.Hour); err != nil {
		t.Fatal(err)
	}
	catalog, err := (ClickHouseQuery{Conn: conn, Router: router}).DiscoverData(ctx, DataCatalogRequest{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{resource}, SourceKeys: []string{source.String() + ":1"}, Signal: "metrics", Kind: "metric_variants", From: at.Add(-time.Minute), To: at.Add(time.Second), Limit: 20, Budget: queryengine.Budget{MaxScanBytes: 32 << 20, Timeout: 10 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	engine := promengine.NewEngine(conn, router, nil)
	seen := map[string]bool{}
	for _, metric := range catalog.Metrics {
		method := enrichObservedMetric(metric, nil, nil).QueryMethods
		if method == nil {
			continue
		}
		seen[method.Representation] = true
		wantCount, wantMean := 3.0, 7.0/3
		if method.Representation == "classic_histogram" {
			wantMean = .2
		}
		if method.Representation == "summary" {
			wantCount, wantMean = 2, 3.5
		}
		for expression, expected := range map[string]float64{method.CountExpression: wantCount, method.MeanExpression: wantMean} {
			result, err := engine.Execute(ctx, promengine.Request{Expression: expression, Instant: true, Start: at.Add(-time.Minute), End: at.Add(time.Second), Scope: promengine.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{resource}, SourceKeys: []string{source.String() + ":1"}}, MaxScanBytes: 32 << 20, Timeout: 10 * time.Second})
			if err != nil {
				t.Fatal("catalog method failed in actual query engine", method.Representation, expression, err)
			}
			values, ok := result.Value.(promql.Vector)
			if !ok || len(values) != 1 || math.Abs(values[0].F-expected) > 1e-9 {
				t.Fatal("catalog method changed actual representation/value", method.Representation, expression, result.Value, expected)
			}
		}
	}
	if len(seen) != 3 || !seen["classic_histogram"] || !seen["native_histogram"] || !seen["summary"] {
		t.Fatal("actual catalog lost representation-specific methods", seen)
	}
	t.Log("Actual Writer/ClickHouse metadata -> shared methods -> PromQL classic/native/summary count and mean; controlled source admission, not Collector or model")
}
