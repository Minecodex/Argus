package telemetry

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"testing"
	"time"
)

func TestObservationMetricCatalogPreservesSourceAndUnitVariants(t *testing.T) {
	conn := sourceTestClickHouse(t)
	ctx := t.Context()
	tenant, host, a, b := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	manager := ClickHouseTenantSchemaManager{Conn: conn, Router: TenantTableRouter{}}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.DropTenant(context.Background(), tenant); err != nil {
			t.Error(err)
		}
	})
	tables, _ := manager.Router.Tables(tenant)
	at := time.Now().UTC().Add(-time.Minute)
	for i, item := range []struct {
		source uuid.UUID
		unit   string
	}{{a, "1"}, {b, "1"}, {a, "bytes"}} {
		series := uuid.New()
		if err := conn.Exec(ctx, "INSERT INTO "+tables.MetricSeries+" (resource_id,series_id,metric_name,labels,metric_type,unit,source_id,source_revision,source_type,expires_at) VALUES (?,?,'same_name',map('state','used'),'gauge',?, ?,1,'hostmetrics',now64(3)+INTERVAL 1 HOUR)", host, series, item.unit, item.source); err != nil {
			t.Fatal(err)
		}
		if err := conn.Exec(ctx, "INSERT INTO "+tables.MetricSamples+" (resource_id,series_id,metric_name,timestamp,value,source_id,source_revision,source_type,expires_at) VALUES (?,?,'same_name',?,?,?,1,'hostmetrics',now64(3)+INTERVAL 1 HOUR)", host, series, at, float64(i+1), item.source); err != nil {
			t.Fatal(err)
		}
	}
	query := ClickHouseQuery{Conn: conn, Router: manager.Router}
	input := DataCatalogRequest{EnterpriseID: tenant, SubjectID: uuid.New(), SubjectType: "user", AuthorizationVersion: 1, ResourceIDs: []uuid.UUID{host}, SourceKeys: []string{a.String() + ":1", b.String() + ":1"}, Signal: "metrics", Kind: "metric_variants", From: at.Add(-time.Second), To: at.Add(time.Second), Limit: 1, Budget: queryengine.Budget{MaxScanBytes: DefaultMaxScanBytes, Timeout: DefaultTimeout}}
	seen := map[string]bool{}
	for page := 0; page < 4; page++ {
		result, err := query.DiscoverData(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Metrics) != 1 {
			t.Fatalf("page %d: %+v", page, result)
		}
		m := result.Metrics[0]
		if m.SourceType != "hostmetrics" || len(m.SourceKeys) != 1 || m.Type != "gauge" {
			t.Fatalf("bad metadata: %+v", m)
		}
		key := m.SourceKeys[0] + "/" + m.Unit
		if seen[key] {
			t.Fatal("cursor repeated variant")
		}
		seen[key] = true
		if !result.HasMore {
			break
		}
		input.Cursor = result.NextCursor
	}
	if len(seen) != 3 {
		t.Fatalf("merged incompatible variants: %v", seen)
	}
	input.SubjectID = uuid.New()
	if _, err := query.DiscoverData(ctx, input); err == nil {
		t.Fatal("cursor reused by another subject")
	}
}
