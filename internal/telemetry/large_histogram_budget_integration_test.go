package telemetry

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/prometheus/prometheus/promql"
)

// Controlled rows reproduce a large component selection sharing storage with
// old, unrelated samples. They are not evidence of live component collection.
func TestClickHouseLargeHistogramSelectionPrunesUnrelatedSamples(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	tenant, target, source := uuid.New(), uuid.New(), uuid.New()
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
	if err := conn.Exec(ctx, "ALTER TABLE "+tables.MetricSamples+" DROP PROJECTION "+histogramSampleProjection); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Second)
	const count = 256
	metadata := fmt.Sprintf(`INSERT INTO %s (resource_id,series_id,metric_name,labels,labels_hash,metric_type,expires_at,source_id,source_revision,source_type)
SELECT toUUID(?),reinterpretAsUUID(MD5(concat('selected-',toString(number)))),'component_duration_seconds',map('item',toString(number),'argus_resource_id',?),cityHash64(number),'histogram',now64(3)+INTERVAL 1 HOUR,toUUID(?),1,'kubernetes_apiserver' FROM numbers(%d)`, tables.MetricSeries, count)
	if err := conn.Exec(ctx, metadata, target.String(), target.String(), source.String()); err != nil {
		t.Fatal(err)
	}
	samples := fmt.Sprintf(`INSERT INTO %s (resource_id,series_id,metric_name,timestamp,count,sum,bucket_counts,explicit_bounds,sample_type,ingest_key,expires_at,source_id,source_revision,source_type)
SELECT resource_id,series_id,metric_name,fromUnixTimestamp64Milli(?),3,1.5,[1,2],[1.0],'histogram',toString(series_id),expires_at,source_id,source_revision,source_type FROM %s`, tables.MetricSamples, tables.MetricSeries)
	if err := conn.Exec(ctx, samples, at.Add(-time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	noise := fmt.Sprintf(`INSERT INTO %s (resource_id,series_id,metric_name,timestamp,float_value,sample_type,ingest_key,expires_at,source_id,source_revision,source_type)
SELECT toUUID(?),reinterpretAsUUID(MD5(concat('unrelated-',toString(number)))),'unrelated',fromUnixTimestamp64Milli(?),1,'gauge',toString(number),now64(3)+INTERVAL 1 HOUR,toUUID(?),1,'kubernetes_apiserver' FROM numbers(800000)`, tables.MetricSamples)
	if err := conn.Exec(ctx, noise, target.String(), at.Add(-2*time.Hour).UnixMilli(), source.String()); err != nil {
		t.Fatal(err)
	}
	if err := conn.Exec(ctx, "OPTIMIZE TABLE "+tables.MetricSamples+" FINAL"); err != nil {
		t.Fatal(err)
	}
	request := promqlengine.Request{Expression: "sum(component_duration_seconds_count)", Instant: true,
		Start: at.Add(-time.Minute), End: at, MaxSeries: count * 3, MaxSamples: count * 3,
		MaxScanBytes: 256 << 20, Timeout: 20 * time.Second,
		Scope: promqlengine.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{target}, SourceKeys: []string{source.String() + ":1"}}}
	engine := promqlengine.NewEngine(conn, router, nil)
	before, err := engine.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if manager.VerifyTenant(ctx, tenant) == nil {
		t.Fatal("tenant without the histogram projection was marked ready")
	}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	if err := manager.VerifyTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	var mutationCount, repeatedCount uint64
	mutationQuery := "SELECT count() FROM system.mutations WHERE database=currentDatabase() AND table=?"
	if err := conn.QueryRow(ctx, mutationQuery, tables.MetricSamples).Scan(&mutationCount); err != nil {
		t.Fatal(err)
	}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, mutationQuery, tables.MetricSamples).Scan(&repeatedCount); err != nil || repeatedCount != mutationCount {
		t.Fatalf("complete projections were unnecessarily rebuilt: before=%d after=%d err=%v", mutationCount, repeatedCount, err)
	}
	request.MaxScanBytes = 8 << 20
	result, err := engine.Execute(ctx, request)
	if err != nil {
		t.Fatalf("large selected histogram scanned unrelated sample history: %v", err)
	}
	values, ok := result.Value.(promql.Vector)
	if !ok || len(values) != 1 || values[0].F != count*3 {
		t.Fatal("histogram selection changed", result.Value)
	}
	if !reflect.DeepEqual(before.Value, result.Value) || result.ScannedBytes >= before.ScannedBytes/2 {
		t.Fatalf("projection changed observations or failed to prune history: before=%d after=%d", before.ScannedBytes, result.ScannedBytes)
	}
	t.Logf("256 randomly distributed histograms retained; existing rows backfilled, scanned bytes before=%d after=%d", before.ScannedBytes, result.ScannedBytes)
}
