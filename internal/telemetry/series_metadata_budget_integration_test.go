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
	rpc "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	metrics "go.opentelemetry.io/proto/otlp/metrics/v1"
	resource "go.opentelemetry.io/proto/otlp/resource/v1"
)

type metadataGranuleRouter struct {
	TenantTableRouter
	series string
}

func (r metadataGranuleRouter) Table(name string, enterprise uuid.UUID) (string, error) {
	if name == "metric_series" {
		return r.series, nil
	}
	return r.TenantTableRouter.Table(name, enterprise)
}

func TestClickHouseSeriesMetadataFiltersPrimaryKeysBeforeWideColumns(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	tenant, target, source := uuid.New(), uuid.New(), uuid.New()
	base := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: base}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.DropTenant(context.Background(), tenant); err != nil {
			t.Error(err)
		}
	})
	now := time.Now().UTC().Truncate(time.Millisecond)
	payload := &rpc.ExportMetricsServiceRequest{ResourceMetrics: []*metrics.ResourceMetrics{{
		Resource: &resource.Resource{Attributes: []*common.KeyValue{
			{Key: "argus.source.id", Value: stringValue(source.String())},
			{Key: "argus.source.revision", Value: stringValue("1")},
			{Key: "argus.source.type", Value: stringValue("redis")},
		}},
		ScopeMetrics: []*metrics.ScopeMetrics{{Metrics: []*metrics.Metric{{Name: "metadata_scan_value", Data: &metrics.Metric_Gauge{Gauge: &metrics.Gauge{DataPoints: []*metrics.NumberDataPoint{{TimeUnixNano: uint64(now.Add(-time.Second).UnixNano()), Value: &metrics.NumberDataPoint_AsDouble{AsDouble: 7}}}}}}}}},
	}}}
	writer := Writer{ClickHouse: conn, Router: base}
	if _, err := writer.writeMetrics(ctx, &kgo.Record{Topic: "owned-metadata-budget", Offset: 1}, TrustedIdentity{EnterpriseID: tenant, ResourceID: target, CollectorID: uuid.New()}, payload, time.Hour); err != nil {
		t.Fatal(err)
	}
	tables, _ := base.Tables(tenant)
	wide := tables.MetricSeries + "_wide_metadata"
	if err := conn.Exec(ctx, "CREATE TABLE "+wide+" AS "+tables.MetricSeries+" ENGINE=ReplacingMergeTree ORDER BY (metric_name,labels_hash,resource_id,series_id) SETTINGS index_granularity_bytes=0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Exec(context.Background(), "DROP TABLE "+wide); err != nil {
			t.Error(err)
		}
	})
	columns := "resource_id,series_id,metric_name,labels,labels_hash,resource_attributes,metric_type,expires_at,source_id,source_revision,source_type"
	if err := conn.Exec(ctx, "INSERT INTO "+wide+" ("+columns+") SELECT "+columns+" FROM "+tables.MetricSeries); err != nil {
		t.Fatal(err)
	}
	noise := fmt.Sprintf(`INSERT INTO %s (%s) SELECT toUUID(?),reinterpretAsUUID(MD5(toString(number))),'metadata_scan_value',map('noise','true'),cityHash64(number),map('wide',repeat('x',32768),'argus.assignment.epoch','1'),'gauge',now64(3)+INTERVAL 1 HOUR,toUUID(?),1,'redis' FROM numbers(2048)`, wide, columns)
	if err := conn.Exec(ctx, noise, uuid.New().String(), uuid.New().String()); err != nil {
		t.Fatal(err)
	}
	if err := conn.Exec(ctx, "OPTIMIZE TABLE "+wide+" FINAL"); err != nil {
		t.Fatal(err)
	}
	engine := promqlengine.NewEngine(conn, metadataGranuleRouter{base, wide}, nil)
	// ClickHouse may read a compressed block larger than the selected map.
	// 16 MiB bounds that block overhead while rejecting the old 64 MiB scan.
	req := promqlengine.Request{Expression: "metadata_scan_value", Instant: true, Start: now.Add(-time.Minute), End: now, Scope: promqlengine.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{target}, SourceKeys: []string{source.String() + ":1"}}, MaxSeries: 10, MaxSamples: 100, MaxScanBytes: 16 << 20, Timeout: 10 * time.Second}
	result, err := engine.Execute(ctx, req)
	if err != nil {
		t.Fatalf("unrelated wide metadata exhausted selected-resource budget: %v", err)
	}
	values, ok := result.Value.(promql.Vector)
	if !ok || len(values) != 1 || values[0].F != 7 || values[0].Metric.Get("argus_resource_id") != target.String() {
		t.Fatal("metadata pruning changed the selected value or resource identity")
	}
	t.Logf("selected value 7, original source scope retained, scanned bytes=%d", result.ScannedBytes)
}
