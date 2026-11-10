package telemetry

import (
	"context"
	"encoding/json"
	"math"
	"os"
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

// A controlled GET/WATCH histogram verifies the actual generated dashboard
// expressions and millisecond units. Live API timings are verified separately.
func TestAPIServerTemplateExcludesLongConnectionsFromLatency(t *testing.T) {
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
	points := []*metricspb.HistogramDataPoint{}
	for _, verb := range []string{"GET", "WATCH", "WATCHLIST", "CONNECT", "PROXY"} {
		for index := range 2 {
			factor := uint64(index + 1)
			count, sum, buckets := 10*factor, .1*float64(factor), []uint64{8 * factor, 2 * factor, 0}
			if verb != "GET" {
				count, sum, buckets = 5*factor, 2500*float64(factor), []uint64{0, 0, 5 * factor}
			}
			points = append(points, &metricspb.HistogramDataPoint{
				TimeUnixNano: uint64(at.Add(time.Duration(index-2) * time.Minute).UnixNano()),
				Count:        count, Sum: &sum, BucketCounts: buckets, ExplicitBounds: []float64{.01, .02},
				Attributes: []*commonpb.KeyValue{{Key: "verb", Value: stringValue(verb)}},
			})
		}
	}
	payload := &rpc.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			{Key: "argus.source.id", Value: stringValue(source.String())},
			{Key: "argus.source.revision", Value: stringValue("1")},
			{Key: "argus.source.type", Value: stringValue("kubernetes_apiserver")},
		}}, ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{{Name: "apiserver_request_duration_seconds", Unit: "s",
			Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{
				AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, DataPoints: points}}}}}},
	}}}
	if _, err := (&Writer{ClickHouse: conn, Router: router}).writeMetrics(ctx, &kgo.Record{Topic: "owned-api-latency"},
		TrustedIdentity{EnterpriseID: tenant, ResourceID: target, CollectorID: uuid.New()}, payload, time.Hour); err != nil {
		t.Fatal(err)
	}
	var templates []struct {
		Key     string `json:"key"`
		Version int    `json:"version"`
		Spec    struct {
			Panels []struct {
				ID      string `json:"id"`
				Unit    string `json:"unit"`
				Targets []struct {
					Definition struct {
						DSL struct {
							Expression string `json:"expression"`
						} `json:"dsl"`
					} `json:"source_definition"`
				} `json:"targets"`
			} `json:"panels"`
		} `json:"spec"`
	}
	raw, err := os.ReadFile("../../api/contracts/dashboard-templates.json")
	if err != nil || json.Unmarshal(raw, &templates) != nil {
		t.Fatal("generated template contract unreadable", err)
	}
	engine := promengine.NewEngine(conn, router, nil)
	checked := 0
	for _, template := range templates {
		if template.Key != "kubernetes.apiserver" {
			continue
		}
		if template.Version < 2 {
			t.Fatal("changed latency semantics reused an immutable template version")
		}
		for _, panel := range template.Spec.Panels {
			want, ok := map[string]float64{"api_latency": 10, "api_p95": 17.5}[panel.ID]
			if !ok {
				continue
			}
			if panel.Unit != "ms" {
				t.Fatal("ordinary API durations must display milliseconds")
			}
			result, err := engine.Execute(ctx, promengine.Request{Expression: panel.Targets[0].Definition.DSL.Expression,
				Instant: true, Start: at.Add(-5 * time.Minute), End: at,
				Scope:     promengine.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{target}, SourceKeys: []string{source.String() + ":1"}},
				MaxSeries: 1000, MaxSamples: 10000, MaxScanBytes: 32 << 20, Timeout: 10 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			vector, valid := result.Value.(promql.Vector)
			if !valid || len(vector) != 1 || math.Abs(vector[0].F-want) > .000001 {
				t.Fatalf("long connections distorted %s: got=%v want=%v ms", panel.ID, result.Value, want)
			}
			checked++
		}
	}
	if checked != 2 {
		t.Fatal("both API latency panels must be verified")
	}
}
