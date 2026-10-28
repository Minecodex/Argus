package telemetry

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func TestCatalogPrunesUnrelatedSeriesBeforeReadingMetadata(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
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
	// Put wide, unrelated metadata in the same index granules as the selected
	// resource. A sparse primary-key lookup alone must not read those maps.
	wide := tables.MetricSeries + "_wide"
	if err := conn.Exec(ctx, "CREATE TABLE "+wide+" AS "+tables.MetricSeries+" ENGINE=ReplacingMergeTree ORDER BY (metric_name,labels_hash,resource_id,series_id) SETTINGS index_granularity_bytes=0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Exec(context.Background(), "DROP TABLE IF EXISTS "+wide) })
	if err := conn.Exec(ctx, "EXCHANGE TABLES "+tables.MetricSeries+" AND "+wide); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(-time.Minute)
	for _, name := range []string{"catalog_cpu", "catalog_memory"} {
		series := uuid.New()
		if err := conn.Exec(ctx, "INSERT INTO "+tables.MetricSeries+" (resource_id,series_id,metric_name,labels,metric_type,unit,source_id,source_revision,expires_at) VALUES (?,?,?,?, 'gauge','1',?,1,now64(3)+INTERVAL 1 HOUR)", host, series, name, map[string]string{"state": "used"}, source); err != nil {
			t.Fatal(err)
		}
		if err := conn.Exec(ctx, "INSERT INTO "+tables.MetricSamples+" (resource_id,series_id,metric_name,timestamp,value,source_id,source_revision,expires_at) VALUES (?,?,?,?,1,?,1,now64(3)+INTERVAL 1 HOUR)", host, series, name, at, source); err != nil {
			t.Fatal(err)
		}
	}
	// The old source has a sample, but the latest metadata belongs to another
	// source. Filtering mutable source identity before FINAL would resurrect it.
	replaced := uuid.New()
	for _, owner := range []uuid.UUID{source, uuid.New()} {
		if err := conn.Exec(ctx, "INSERT INTO "+tables.MetricSeries+" (resource_id,series_id,metric_name,labels,metric_type,source_id,source_revision,expires_at) VALUES (?,?,'catalog_replaced',map('state','old'),'gauge',?,1,now64(3)+INTERVAL 1 HOUR)", host, replaced, owner); err != nil {
			t.Fatal(err)
		}
	}
	if err := conn.Exec(ctx, "INSERT INTO "+tables.MetricSamples+" (resource_id,series_id,metric_name,timestamp,value,source_id,source_revision,expires_at) VALUES (?,?,'catalog_replaced',?,1,?,1,now64(3)+INTERVAL 1 HOUR)", host, replaced, at, source); err != nil {
		t.Fatal(err)
	}
	noise := fmt.Sprintf(`INSERT INTO %s (resource_id,series_id,metric_name,labels,labels_hash,resource_attributes,metric_type,expires_at,source_id,source_revision) SELECT toUUID(?),reinterpretAsUUID(MD5(toString(number))),'catalog_cpu',map('noise','true'),cityHash64(number),map('wide',repeat('x',32768)),'gauge',now64(3)+INTERVAL 1 HOUR,toUUID(?),1 FROM numbers(2048)`, tables.MetricSeries)
	if err := conn.Exec(ctx, noise, uuid.New().String(), uuid.New().String()); err != nil {
		t.Fatal(err)
	}
	if err := conn.Exec(ctx, "OPTIMIZE TABLE "+tables.MetricSeries+" FINAL"); err != nil {
		t.Fatal(err)
	}
	backend := ClickHouseQuery{Conn: conn, Router: router}
	request := DataCatalogRequest{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{host}, SourceKeys: []string{source.String() + ":1"}, Signal: "metrics", From: at.Add(-time.Second), To: at.Add(time.Second), Limit: 1, Budget: queryengine.Budget{MaxScanBytes: 16 << 20, MaxRows: 10, MaxResultBytes: 1 << 20}}
	for _, kind := range []string{"metrics", "fields", "values"} {
		t.Run(kind, func(t *testing.T) {
			req := request
			req.Kind = kind
			if kind != "metrics" {
				req.Metric = "catalog_cpu"
				req.Limit = 10
			}
			if kind == "values" {
				req.Field = "state"
				req.SelectedValues = []string{"used", "missing"}
			}
			data, err := backend.DiscoverData(ctx, req)
			if err != nil {
				t.Fatalf("unrelated metadata exhausted catalog budget: %v", err)
			}
			switch kind {
			case "metrics":
				if len(data.Metrics) != 1 || data.Metrics[0].Name != "catalog_cpu" || data.Metrics[0].Type != "gauge" || !data.HasMore || data.Complete {
					t.Fatalf("wrong metric page: %+v", data)
				}
				req.Cursor = data.NextCursor
				data, err = backend.DiscoverData(ctx, req)
				if err != nil || len(data.Metrics) != 1 || data.Metrics[0].Name != "catalog_memory" || data.HasMore {
					t.Fatalf("wrong continuation: %+v, %v", data, err)
				}
			case "fields":
				if !slices.Equal(data.Fields, []CatalogField{{Name: "__name__", Type: "string"}, {Name: "state", Type: "string"}}) {
					t.Fatalf("wrong scoped fields: %+v", data)
				}
			case "values":
				if !slices.Equal(data.Values, []string{"used"}) || !data.Membership["used"] || data.Membership["missing"] {
					t.Fatalf("wrong scoped values: %+v", data)
				}
			}
			t.Logf("catalog %s scanned bytes=%d", kind, data.Meta.ScannedBytes)
		})
	}
}

func TestCatalogPrunesHistoricalSamplesUsingSourceTimeProjection(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	tenant, host, source, series := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	router := TenantTableRouter{}
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(context.Background(), tenant) })
	tables, _ := router.Tables(tenant)
	at := time.Now().UTC().Add(-time.Minute)
	if err := conn.Exec(ctx, "INSERT INTO "+tables.MetricSeries+" (resource_id,series_id,metric_name,metric_type,source_id,source_revision,expires_at) VALUES (?,?,'catalog_recent','gauge',?,1,now64(3)+INTERVAL 1 HOUR)", host, series, source); err != nil {
		t.Fatal(err)
	}
	if err := conn.Exec(ctx, "INSERT INTO "+tables.MetricSamples+" (resource_id,series_id,metric_name,timestamp,value,source_id,source_revision,expires_at) VALUES (?,?,'catalog_recent',?,1,?,1,now64(3)+INTERVAL 1 HOUR)", host, series, at, source); err != nil {
		t.Fatal(err)
	}
	// The same source has many historical short series. Resource/source filters
	// alone cannot prune the raw series-first index; the existing projection can.
	noise := fmt.Sprintf(`INSERT INTO %s (resource_id,series_id,metric_name,timestamp,value,source_id,source_revision,expires_at) SELECT toUUID(?),reinterpretAsUUID(MD5(toString(number))),'catalog_old',now64(9)-INTERVAL 2 DAY,1,toUUID(?),1,now64(3)+INTERVAL 1 HOUR FROM numbers(500000)`, tables.MetricSamples)
	if err := conn.Exec(ctx, noise, host.String(), source.String()); err != nil {
		t.Fatal(err)
	}
	if err := conn.Exec(ctx, "OPTIMIZE TABLE "+tables.MetricSamples+" FINAL"); err != nil {
		t.Fatal(err)
	}
	backend := ClickHouseQuery{Conn: conn, Router: router}
	request := DataCatalogRequest{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{host}, SourceKeys: []string{source.String() + ":1"}, Signal: "metrics", Kind: "metrics", From: at.Add(-time.Second), To: at.Add(time.Second), Limit: 10, Budget: queryengine.Budget{MaxScanBytes: 2 << 20, MaxRows: 10, MaxResultBytes: 1 << 20}}
	result, err := backend.DiscoverData(ctx, request)
	if err != nil {
		t.Fatalf("historical samples exhausted catalog budget: %v", err)
	}
	if len(result.Metrics) != 1 || result.Metrics[0].Name != "catalog_recent" {
		t.Fatalf("catalog lost recent sample: %+v", result)
	}
	t.Logf("historical sample pruning scanned bytes=%d", result.Meta.ScannedBytes)
	for len(request.SourceKeys) < 10000 {
		request.SourceKeys = append(request.SourceKeys, uuid.NewString()+":1")
	}
	result, err = backend.DiscoverData(ctx, request)
	if err != nil || len(result.Metrics) != 1 || result.Metrics[0].Name != "catalog_recent" {
		t.Fatalf("large source scope changed catalog or expanded SQL: %+v, %v", result, err)
	}
}
