package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/metricmeta"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/twmb/franz-go/pkg/kgo"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestClickHouseInputMetricMetadataUsesLoadedAuthorizedSeries(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := t.Context()
	tenant, target := uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), tenant) })
	writer := Writer{ClickHouse: conn, Router: router}
	identity := TrustedIdentity{EnterpriseID: tenant, ResourceID: target, CollectorID: uuid.New()}
	now := time.Now().UTC().Truncate(time.Second)
	sources := []SourceIdentity{{ID: uuid.New(), Revision: 2, Type: "hostmetrics"}, {ID: uuid.New(), Revision: 3, Type: "otlp"}}
	for index, source := range sources {
		points := []*metricspb.NumberDataPoint{{TimeUnixNano: uint64(now.Add(-time.Minute).UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 100}}, {TimeUnixNano: uint64(now.UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 160}}}
		metric := &metricspb.Metric{Name: "typed_source_value", Unit: "By"}
		if index == 0 {
			metric.Data = &metricspb.Metric_Sum{Sum: &metricspb.Sum{DataPoints: points, AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, IsMonotonic: true}}
		} else {
			metric.Unit = "1"
			metric.Data = &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: points}}
		}
		attrs := []*commonpb.KeyValue{{Key: "argus.source.id", Value: stringValue(source.ID.String())}, {Key: "argus.source.revision", Value: stringValue("2")}, {Key: "argus.source.type", Value: stringValue(source.Type)}}
		if index == 1 {
			attrs[1].Value = stringValue("3")
		}
		request := &collectmetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcepb.Resource{Attributes: attrs}, ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{metric}}}}}}
		if _, err := writer.writeMetrics(ctx, &kgo.Record{Topic: "otlp-metrics", Offset: int64(index + 1)}, identity, request, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	engine := promqlengine.NewEngine(conn, router, nil)
	query := func(expression string, keys []string) []metricmeta.Descriptor {
		t.Helper()
		result, err := engine.Execute(ctx, promqlengine.Request{Expression: expression, Start: now.Add(-2 * time.Minute), End: now.Add(time.Second), Step: time.Minute, Scope: promqlengine.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{target}, SourceKeys: keys}, MaxSeries: 50, MaxSamples: 100, MaxScanBytes: 32 << 20, Timeout: 20 * time.Second})
		if err != nil || !result.InputMetricsComplete {
			t.Fatal("query metadata failed/incomplete", err, result.InputMetrics)
		}
		return result.InputMetrics
	}
	counter := query("typed_source_value", []string{sources[0].Key()})
	if len(counter) != 1 || counter[0].Type != "counter" || counter[0].Unit != "By" || counter[0].SourceKey != sources[0].Key() || !counter[0].Monotonic {
		t.Fatal("actual cumulative source metadata changed", counter)
	}
	gauge := query("typed_source_value", []string{sources[1].Key()})
	if len(gauge) != 1 || gauge[0].Type != "gauge" || gauge[0].Unit != "1" || gauge[0].SourceKey != sources[1].Key() {
		t.Fatal("source selection leaked/merged metadata", gauge)
	}
	mixed := query("typed_source_value", []string{sources[0].Key(), sources[1].Key()})
	if len(mixed) != 2 {
		t.Fatal("same-name metadata conflict was silently merged", mixed)
	}
	derived := query("rate(typed_source_value[2m])", []string{sources[0].Key()})
	if len(derived) != 1 || derived[0].Type != "counter" || derived[0].Unit != "By" {
		t.Fatal("input descriptor was falsely rewritten as derived output unit/type", derived)
	}
}
