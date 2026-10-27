package telemetry

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/prometheus/prometheus/promql"
	"github.com/twmb/franz-go/pkg/kgo"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestClickHouseMetricResourceAndRevisionIdentity(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	enterprise, target, collector, collection := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, enterprise); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), enterprise) })
	writer := Writer{ClickHouse: conn, Router: router}
	identity := TrustedIdentity{EnterpriseID: enterprise, ResourceID: target, CollectorID: collector}
	now := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	first, second := uuid.New(), uuid.New()
	write := func(metric, pod string, source uuid.UUID, revision, epoch int64, at time.Time, value float64, offset int64) {
		t.Helper()
		attrs := []*commonpb.KeyValue{
			{Key: "argus.source.id", Value: stringValue(source.String())}, {Key: "argus.source.revision", Value: stringValue(strconv.FormatInt(revision, 10))},
			{Key: "argus.source.type", Value: stringValue("mysql")}, {Key: "argus.collection.id", Value: stringValue(collection.String())},
			{Key: "argus.assignment.epoch", Value: stringValue(strconv.FormatInt(epoch, 10))},
		}
		if pod != "" {
			attrs = append(attrs, &commonpb.KeyValue{Key: "k8s.pod.uid", Value: stringValue(pod)})
		}
		point := &metricspb.NumberDataPoint{TimeUnixNano: uint64(at.UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: value}}
		metricValue := &metricspb.Metric{Name: metric, Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{point}}}}
		request := &collectmetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: &resourcepb.Resource{Attributes: attrs}, ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{metricValue}}}}}}
		if _, err := writer.writeMetrics(ctx, &kgo.Record{Topic: "otlp-metrics", Partition: 0, Offset: offset}, identity, request, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	write("pod_value", "pod-a", first, 1, 1, now, 3, 1)
	write("pod_value", "pod-b", first, 1, 1, now, 7, 2)
	write("pod_value", "pod-b", first, 1, 1, now, 7, 2) // Kafka replay.
	write("revision_value", "", first, 1, 1, now, 10, 3)
	write("revision_value", "", first, 2, 1, now.Add(10*time.Second), 20, 4)
	// A moved collection may have a lower Collector revision. Assignment epoch
	// determines the winner for a simultaneous sample, not source UUID ordering.
	write("revision_value", "", second, 1, 2, now.Add(10*time.Second), 30, 5)
	query := func(expression string, keys []string) promql.Vector {
		t.Helper()
		result, err := promqlengine.NewEngine(conn, router, nil).Execute(ctx, promqlengine.Request{Expression: expression, Instant: true, Start: now.Add(-time.Minute), End: now.Add(11 * time.Second), Scope: promqlengine.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{target}, SourceKeys: keys}, MaxSeries: 100, MaxSamples: 1000})
		if err != nil {
			t.Fatal(err)
		}
		v, ok := result.Value.(promql.Vector)
		if !ok {
			t.Fatalf("unexpected %T", result.Value)
		}
		return v
	}
	if values := query("pod_value", nil); len(values) != 2 {
		t.Fatalf("Pods collapsed: %v", values)
	}
	for _, tc := range []struct {
		expression string
		keys       []string
		want       float64
	}{
		{"sum(pod_value)", nil, 10}, {"sum(revision_value)", nil, 30},
		{"sum(revision_value)", []string{first.String() + ":1"}, 10},
		{"sum(revision_value)", []string{first.String() + ":2"}, 20},
		{"sum(revision_value)", []string{second.String() + ":1"}, 30},
	} {
		values := query(tc.expression, tc.keys)
		if len(values) != 1 || values[0].F != tc.want {
			t.Fatalf("%s %v: got %v want %g", tc.expression, tc.keys, values, tc.want)
		}
	}
	tables, _ := router.Tables(enterprise)
	overview, err := (ClickHouseQuery{Conn: conn, Router: router}).Overview(ctx, OverviewRequest{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{target}, SourceKeys: []string{first.String() + ":1"}, From: now.Add(-time.Minute), To: now.Add(time.Minute), MaxScanBytes: DefaultMaxScanBytes, Timeout: DefaultTimeout})
	if err != nil || overview.MetricPoints != 3 {
		t.Fatalf("overview source isolation: %+v %v", overview, err)
	}
	blank, err := (ClickHouseQuery{Conn: conn, Router: router}).Overview(ctx, OverviewRequest{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{target}, From: now.Add(-time.Minute), To: now.Add(time.Minute)})
	if err != nil || blank.MetricPoints != 0 || blank.LogRecords != 0 || blank.Spans != 0 {
		t.Fatalf("empty overview sources expanded: %+v %v", blank, err)
	}
	empty, err := promqlengine.NewEngine(conn, router, nil).Execute(ctx, promqlengine.Request{Expression: "pod_value", Instant: true, Start: now.Add(-time.Minute), End: now.Add(11 * time.Second), Scope: promqlengine.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{}}})
	if err != nil {
		t.Fatal(err)
	}
	if values, ok := empty.Value.(promql.Vector); !ok || len(values) != 0 {
		t.Fatalf("empty scope expanded: %v", empty.Value)
	}
	var sources uint64
	if err := conn.QueryRow(ctx, "SELECT uniqExact(source_key) FROM `"+tables.MetricSamples+"` WHERE metric_name='revision_value'").Scan(&sources); err != nil || sources != 3 {
		t.Fatalf("physical provenance lost: %d %v", sources, err)
	}
}
