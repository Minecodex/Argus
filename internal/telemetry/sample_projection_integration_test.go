package telemetry

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/twmb/franz-go/pkg/kgo"
	collectmetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

func TestClickHouseNumericProjectionMaterializesExistingSamples(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	tenant, host, source := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.DropTenant(context.Background(), tenant); err != nil {
			t.Error(err)
		}
	})
	tables, _ := router.Tables(tenant)
	if err := conn.Exec(ctx, "ALTER TABLE "+tables.MetricSamples+" DROP PROJECTION "+numericSampleProjection); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	points := []*metricspb.NumberDataPoint{}
	for i := range 128 {
		points = append(points, &metricspb.NumberDataPoint{TimeUnixNano: uint64(now.Add(-time.Second).UnixNano()),
			Attributes: []*commonpb.KeyValue{{Key: "device", Value: stringValue(fmt.Sprint(i))}}, Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: float64(i)}})
	}
	payload := &collectmetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			{Key: "argus.source.id", Value: stringValue(source.String())}, {Key: "argus.source.revision", Value: stringValue("1")}, {Key: "argus.source.type", Value: stringValue("hostmetrics")},
		}}, ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{{Name: "short_lived_series", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: points}}}}}},
	}}}
	writer := Writer{ClickHouse: conn, Router: router}
	if _, err := writer.writeMetrics(ctx, &kgo.Record{Topic: "owned-projection-regression", Offset: 1}, TrustedIdentity{EnterpriseID: tenant, ResourceID: host, CollectorID: uuid.New()}, payload, 4*time.Hour); err != nil {
		t.Fatal(err)
	}
	noise := fmt.Sprintf(`INSERT INTO %s (resource_id,series_id,metric_name,timestamp,float_value,sample_type,ingest_key,kafka_offset,expires_at,source_id,source_revision,source_type)
SELECT toUUID(?),reinterpretAsUUID(MD5(toString(intDiv(number,64)))),'older_unrelated_series',fromUnixTimestamp64Milli(?),toFloat64(number),'gauge',hex(MD5(toString(number))),toInt64(number),now64(3)+INTERVAL 4 HOUR,toUUID(?),1,'hostmetrics' FROM numbers(500000)`, tables.MetricSamples)
	if err := conn.Exec(ctx, noise, host.String(), now.Add(-2*time.Hour).UnixMilli(), source.String()); err != nil {
		t.Fatal(err)
	}
	if err := conn.Exec(ctx, "OPTIMIZE TABLE "+tables.MetricSamples+" FINAL"); err != nil {
		t.Fatal(err)
	}
	engine := promqlengine.NewEngine(conn, router, nil)
	req := promqlengine.Request{Expression: "short_lived_series", Instant: true, Start: now.Add(-time.Minute), End: now, Scope: promqlengine.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{host}, SourceKeys: []string{source.String() + ":1"}}, MaxSeries: 1000, MaxSamples: 1000, MaxScanBytes: 256 << 20, Timeout: 10 * time.Second}
	before, err := engine.Execute(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.upgradeSampleProjection(ctx, tables); err != nil {
		t.Fatal(err)
	}
	if err = manager.VerifyTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	req.MaxScanBytes = 8 << 20
	after, err := engine.Execute(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Value, after.Value) {
		t.Fatal("projection changed observations or series identities")
	}
	if after.ScannedBytes >= before.ScannedBytes/2 {
		t.Fatalf("projection did not avoid old UUID granules: before=%d after=%d", before.ScannedBytes, after.ScannedBytes)
	}
	t.Logf("unchanged 128 series; scan before=%d after=%d", before.ScannedBytes, after.ScannedBytes)
}
