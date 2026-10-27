package telemetry

import (
	"context"
	"fmt"
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

func TestClickHouseSelectedSeriesUsesBoundedScanAmidUnrelatedSamples(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	enterprise, target, source := uuid.New(), uuid.New(), uuid.New()
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
	now := time.Now().UTC().Truncate(time.Millisecond)
	point := &metricspb.NumberDataPoint{TimeUnixNano: uint64(now.Add(-time.Second).UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 7}}
	metric := &metricspb.Metric{Name: "selected_gauge", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{point}}}}
	attributes := []*commonpb.KeyValue{{Key: "argus.source.id", Value: stringValue(source.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "argus.source.type", Value: stringValue("mysql")}}
	resource := &metricspb.ResourceMetrics{Resource: &resourcepb.Resource{Attributes: attributes}, ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{metric}}}}
	payload := &collectmetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{resource}}
	writer := Writer{ClickHouse: conn, Router: router}
	if _, err := writer.writeMetrics(ctx, &kgo.Record{Topic: "owned-scan-regression", Partition: 0, Offset: 1}, TrustedIdentity{EnterpriseID: enterprise, ResourceID: target, CollectorID: uuid.New()}, payload, time.Hour); err != nil {
		t.Fatal(err)
	}
	tables, err := router.Tables(enterprise)
	if err != nil {
		t.Fatal(err)
	}
	// Controlled scalability fixture: real ClickHouse rows belonging to other
	// resource/source identities. No unrelated user table is touched.
	noise := fmt.Sprintf(`INSERT INTO %s (resource_id,series_id,metric_name,timestamp,value,float_value,sample_type,ingest_key,kafka_topic,kafka_partition,kafka_offset,record_sequence,expires_at,source_id,source_revision,source_type)
 SELECT toUUID(?),reinterpretAsUUID(MD5(toString(intDiv(number,64)))),'unrelated_gauge',fromUnixTimestamp64Milli(?),toFloat64(number),toFloat64(number),'gauge',toString(number),'owned-background',0,toInt64(number),0,now64(3)+INTERVAL 1 HOUR,toUUID(?),1,'hostmetrics' FROM numbers(400000)`, tables.MetricSamples)
	if err = conn.Exec(ctx, noise, uuid.New().String(), now.Add(-2*time.Second).UnixMilli(), uuid.New().String()); err != nil {
		t.Fatal(err)
	}
	engine := promqlengine.NewEngine(conn, router, nil)
	request := promqlengine.Request{Expression: "selected_gauge", Instant: true, Start: now.Add(-time.Minute), End: now, Scope: promqlengine.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{target}, SourceKeys: []string{source.String() + ":1"}}, MaxSeries: 10, MaxSamples: 100, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}
	baseline, err := engine.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	assertValue := func(value any) {
		t.Helper()
		v, ok := value.(promql.Vector)
		if !ok || len(v) != 1 || v[0].F != 7 || v[0].Metric.Get("argus_resource_id") != target.String() {
			t.Fatal("selected sample or resource isolation changed")
		}
	}
	assertValue(baseline.Value)
	t.Logf("same authorized result: baseline scanned rows=%d bytes=%d", baseline.ScannedRows, baseline.ScannedBytes)
	request.MaxScanBytes = 4 << 20
	bounded, err := engine.Execute(ctx, request)
	if err != nil {
		t.Fatalf("single selected series scanned unrelated resources beyond 4MiB: %v", err)
	}
	assertValue(bounded.Value)
	if bounded.ScannedBytes > request.MaxScanBytes || bounded.ScannedRows >= 50000 {
		t.Fatalf("selected-series pruning ineffective: rows=%d bytes=%d", bounded.ScannedRows, bounded.ScannedBytes)
	}
	t.Logf("bounded scan rows=%d bytes=%d; unchanged value and resource/source authorization", bounded.ScannedRows, bounded.ScannedBytes)
}

type sharedGranuleRouter struct {
	TenantTableRouter
	samples string
}

func (r sharedGranuleRouter) Table(name string, enterprise uuid.UUID) (string, error) {
	if name == "metric_samples" {
		return r.samples, nil
	}
	return r.TenantTableRouter.Table(name, enterprise)
}

func TestClickHouseSelectedSeriesDoesNotReadHistogramPayloadColumns(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	enterprise, target, source := uuid.New(), uuid.New(), uuid.New()
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
	now := time.Now().UTC().Truncate(time.Millisecond)
	point := &metricspb.NumberDataPoint{TimeUnixNano: uint64(now.Add(-time.Second).UnixNano()), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 7}}
	payload := &collectmetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			{Key: "argus.source.id", Value: stringValue(source.String())},
			{Key: "argus.source.revision", Value: stringValue("1")},
			{Key: "argus.source.type", Value: stringValue("hostmetrics")},
		}},
		ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{{Name: "shared_granule_gauge", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{point}}}}}}},
	}}}
	writer := Writer{ClickHouse: conn, Router: router}
	if _, err := writer.writeMetrics(ctx, &kgo.Record{Topic: "owned-shared-granule", Offset: 1}, TrustedIdentity{EnterpriseID: enterprise, ResourceID: target, CollectorID: uuid.New()}, payload, time.Hour); err != nil {
		t.Fatal(err)
	}
	tables, err := router.Tables(enterprise)
	if err != nil {
		t.Fatal(err)
	}
	// Same authorized source, same time window and one shared granule: primary
	// index pruning cannot avoid reading its unrelated wide histogram payloads.
	samples := tables.MetricSamples + "_wide_regression"
	if err = conn.Exec(ctx, "CREATE TABLE "+samples+" AS "+tables.MetricSamples+" ENGINE=ReplacingMergeTree ORDER BY (series_id,timestamp,ingest_key) SETTINGS index_granularity_bytes=0, deduplicate_merge_projection_mode='rebuild'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Exec(context.Background(), "DROP TABLE IF EXISTS "+samples+" SYNC") })
	columns := "resource_id,series_id,metric_name,timestamp,value,float_value,sample_type,ingest_key,kafka_topic,kafka_partition,kafka_offset,record_sequence,expires_at,source_id,source_revision,source_type"
	if err = conn.Exec(ctx, "INSERT INTO "+samples+" ("+columns+") SELECT "+columns+" FROM "+tables.MetricSamples); err != nil {
		t.Fatal(err)
	}
	noise := fmt.Sprintf(`INSERT INTO %s (resource_id,series_id,metric_name,timestamp,value,sample_type,count,sum,bucket_counts,explicit_bounds,ingest_key,kafka_topic,kafka_offset,expires_at,source_id,source_revision,source_type)
 SELECT toUUID(?),reinterpretAsUUID(MD5(toString(number))),'unselected_histogram',fromUnixTimestamp64Milli(?),0,'histogram',2048,2048,
 arrayMap(x -> toUInt64(1),range(2048)),arrayMap(x -> toFloat64(x),range(2047)),toString(number),'owned-shared-granule',toInt64(number),now64(3)+INTERVAL 1 HOUR,toUUID(?),1,'hostmetrics' FROM numbers(2048)`, samples)
	if err = conn.Exec(ctx, noise, target.String(), now.Add(-time.Second).UnixMilli(), source.String()); err != nil {
		t.Fatal(err)
	}
	if err = conn.Exec(ctx, "OPTIMIZE TABLE "+samples+" FINAL"); err != nil {
		t.Fatal(err)
	}
	engine := promqlengine.NewEngine(conn, sharedGranuleRouter{TenantTableRouter: router, samples: samples}, nil)
	result, err := engine.Execute(ctx, promqlengine.Request{Expression: "shared_granule_gauge", Instant: true,
		Start: now.Add(-time.Minute), End: now,
		Scope:     promqlengine.Scope{EnterpriseID: enterprise, ResourceIDs: []uuid.UUID{target}, SourceKeys: []string{source.String() + ":1"}},
		MaxSeries: 10, MaxSamples: 100, MaxScanBytes: 4 << 20, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("selected gauge read unrelated histogram payloads beyond 4MiB: %v", err)
	}
	values, ok := result.Value.(promql.Vector)
	if !ok || len(values) != 1 || values[0].F != 7 || values[0].Metric.Get("argus_resource_id") != target.String() {
		t.Fatal("early filtering changed selected data or authorization")
	}
	if result.ScannedBytes > 4<<20 {
		t.Fatalf("wide payloads were read before filtering: %d", result.ScannedBytes)
	}
	t.Logf("shared granule: scanned rows=%d bytes=%d; selected value=7", result.ScannedRows, result.ScannedBytes)
}
