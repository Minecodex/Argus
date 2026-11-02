package telemetry

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/prometheus/prometheus/promql"
	"testing"
	"time"
)

func TestClickHouseMetricAssignmentUpgradePreservesExistingRows(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := t.Context()
	tenant := uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), tenant) })
	tables, _ := router.Tables(tenant)
	if err := conn.Exec(ctx, "ALTER TABLE "+tables.MetricSeries+" DROP COLUMN assignment_epoch"); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	if err := conn.Exec(ctx, "INSERT INTO "+tables.MetricSeries+" (resource_id,series_id,metric_name,resource_attributes,expires_at) VALUES (?,?,'upgrade_value',map('argus.assignment.epoch','17','keep','original'),now64(3)+INTERVAL 1 HOUR)", id, id); err != nil {
		t.Fatal(err)
	}
	if manager.VerifyTenant(ctx, tenant) == nil {
		t.Fatal("old schema incorrectly accepted")
	}
	for range 2 {
		if err := manager.EnsureTenant(ctx, tenant); err != nil {
			t.Fatal(err)
		}
		if err := manager.VerifyTenant(ctx, tenant); err != nil {
			t.Fatal(err)
		}
		var count uint64
		var epoch int64
		var original string
		if err := conn.QueryRow(ctx, "SELECT count(),any(assignment_epoch),any(resource_attributes['keep']) FROM "+tables.MetricSeries).Scan(&count, &epoch, &original); err != nil {
			t.Fatal(err)
		}
		if count != 1 || epoch != 17 || original != "original" {
			t.Fatal("upgrade changed authoritative data", count, epoch, original)
		}
	}
}

// Controlled ClickHouse rows exercise the maximum-size SQL transport path;
// this is a query regression, not evidence of real receiver collection.
func TestClickHouseLargeSelectedSeriesUsesBoundedSQL(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := t.Context()
	tenant, target, source := uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), tenant) })
	tables, _ := router.Tables(tenant)
	now := time.Now().UTC().Truncate(time.Second)
	const count = 10000
	metadata := fmt.Sprintf("INSERT INTO %s (resource_id,series_id,metric_name,labels,labels_hash,metric_type,expires_at,source_id,source_revision,source_type) SELECT toUUID(?),reinterpretAsUUID(MD5(toString(number))),'many_selected',map('item',toString(number),'argus_resource_id',?),cityHash64(number),'gauge',now64(3)+INTERVAL 1 HOUR,toUUID(?),1,'hostmetrics' FROM numbers(%d)", tables.MetricSeries, count)
	if err := conn.Exec(ctx, metadata, target.String(), target.String(), source.String()); err != nil {
		t.Fatal(err)
	}
	samples := fmt.Sprintf("INSERT INTO %s (resource_id,series_id,metric_name,timestamp,float_value,sample_type,ingest_key,expires_at,source_id,source_revision,source_type) SELECT resource_id,series_id,metric_name,fromUnixTimestamp64Milli(?),1,'gauge',toString(series_id),expires_at,source_id,source_revision,source_type FROM %s", tables.MetricSamples, tables.MetricSeries)
	if err := conn.Exec(ctx, samples, now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	result, err := promqlengine.NewEngine(conn, router, nil).Execute(ctx, promqlengine.Request{Expression: "sum(many_selected)", Instant: true, Start: now.Add(-time.Minute), End: now.Add(time.Second), Scope: promqlengine.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{target}, SourceKeys: []string{source.String() + ":1"}}, MaxSeries: count + 1, MaxSamples: count + 1, MaxScanBytes: 32 << 20, Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	values, ok := result.Value.(promql.Vector)
	if !ok || len(values) != 1 || values[0].F != count {
		t.Fatal("large selected set changed", result.Value)
	}
}
