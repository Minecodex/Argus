package telemetry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	promengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/prometheus/prometheus/promql"
)

// Same-source, same-hour noise is deliberately inside the old source/time
// projection. A single metric must not read every other metric's hour of data.
func TestClickHouseMetricWindowPrunesOtherMetricsInSameSource(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	tenant, target, source := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), tenant) })
	tables, _ := router.Tables(tenant)
	at := time.Now().UTC().Truncate(time.Minute)
	metadata := fmt.Sprintf(`INSERT INTO %s (resource_id,series_id,metric_name,labels,labels_hash,metric_type,expires_at,source_id,source_revision,source_type)
SELECT toUUID(?),reinterpretAsUUID(MD5(concat('wanted-',toString(number)))),'window_metric',map('device',toString(number)),cityHash64(number),'gauge',now64(3)+INTERVAL 1 HOUR,toUUID(?),1,'kubeletstats' FROM numbers(32)`, tables.MetricSeries)
	if err := conn.Exec(ctx, metadata, target.String(), source.String()); err != nil {
		t.Fatal(err)
	}
	wanted := fmt.Sprintf(`INSERT INTO %s (resource_id,series_id,metric_name,timestamp,float_value,sample_type,ingest_key,expires_at,source_id,source_revision,source_type)
SELECT resource_id,series_id,metric_name,fromUnixTimestamp64Milli(?)+toIntervalSecond(number*10),7,'gauge',concat(toString(series_id),'-',toString(number)),expires_at,source_id,source_revision,source_type FROM %s AS metadata CROSS JOIN numbers(360) AS ticks`, tables.MetricSamples, tables.MetricSeries)
	if err := conn.Exec(ctx, wanted, at.Add(-time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	noise := fmt.Sprintf(`INSERT INTO %s (resource_id,series_id,metric_name,timestamp,float_value,sample_type,ingest_key,expires_at,source_id,source_revision,source_type)
SELECT toUUID(?),reinterpretAsUUID(MD5(concat('noise-',toString(number%%10000)))),concat('noise_',toString(number%%1000)),fromUnixTimestamp64Milli(?)+toIntervalSecond(number%%3600),1,'gauge',toString(number),now64(3)+INTERVAL 1 HOUR,toUUID(?),1,'kubeletstats' FROM numbers(800000)`, tables.MetricSamples)
	if err := conn.Exec(ctx, noise, target.String(), at.Add(-time.Hour).UnixMilli(), source.String()); err != nil {
		t.Fatal(err)
	}
	if err := conn.Exec(ctx, "OPTIMIZE TABLE "+tables.MetricSamples+" FINAL"); err != nil {
		t.Fatal(err)
	}
	request := promengine.Request{Expression: "sum(window_metric)", Start: at.Add(-time.Hour), End: at.Add(-10 * time.Second), Step: time.Minute,
		Scope:     promengine.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{target}, SourceKeys: []string{source.String() + ":1"}},
		MaxSeries: 1000, MaxSamples: 50000, MaxScanBytes: 4 << 20, Timeout: 20 * time.Second}
	result, err := promengine.NewEngine(conn, router, nil).Execute(ctx, request)
	if err != nil {
		t.Fatalf("same-source unrelated metrics exhausted one-hour query: %v", err)
	}
	matrix, ok := result.Value.(promql.Matrix)
	if !ok || len(matrix) != 1 || len(matrix[0].Floats) < 59 {
		t.Fatal("one-hour result missing", result.Value)
	}
	for _, point := range matrix[0].Floats {
		if point.F != 224 {
			t.Fatal("metric selection changed", point)
		}
	}
	t.Logf("32 series, one hour, unrelated metrics excluded: bytes=%d rows=%d", result.ScannedBytes, result.ScannedRows)
}
