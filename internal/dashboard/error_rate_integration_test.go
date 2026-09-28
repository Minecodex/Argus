package dashboard

import (
	"context"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
	"github.com/prometheus/prometheus/promql"
)

func seedErrorCounters(t *testing.T, ctx context.Context, conn driver.Conn, tables telemetry.TenantTables, host, source uuid.UUID, at time.Time) {
	t.Helper()
	for _, item := range []struct {
		service, status string
		increment       float64
	}{
		{"orders", "200", 8}, {"orders", "500", 2}, {"healthy", "200", 10}, {"idle", "200", 0}, {"incomplete", "200", 10}, {"incomplete", "500", 1},
	} {
		series := uuid.New()
		if err := conn.Exec(ctx, "INSERT INTO `"+tables.MetricSeries+"` (resource_id,series_id,metric_name,metric_type,temporality,is_monotonic,labels,source_id,source_revision,source_type,expires_at) VALUES (?,?,'planv2_requests_total','sum','cumulative',true,?,?,1,'otlp',now64(3)+INTERVAL 1 HOUR)", host, series, map[string]string{"service": item.service, "status": item.status, "env": "prod"}, source); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 6; i++ {
			if item.service == "incomplete" && item.status == "500" && i != 5 {
				continue
			}
			value := float64(i) * item.increment
			if err := conn.Exec(ctx, "INSERT INTO `"+tables.MetricSamples+"` (resource_id,series_id,metric_name,timestamp,value,float_value,sample_type,ingest_key,source_id,source_revision,source_type,expires_at) VALUES (?,?,'planv2_requests_total',?,?,?,'float',?,?,1,'otlp',now64(3)+INTERVAL 1 HOUR)", host, series, at.Add(time.Duration(i-5)*30*time.Second), value, value, fmt.Sprintf("%s-%d", series, i), source); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestErrorRateAgainstReceivedCounterSamples(t *testing.T) {
	address := os.Getenv("ARGUS_CLICKHOUSE_TEST_ADDRESS")
	if address == "" {
		t.Skip("ClickHouse endpoint required")
	}
	conn, err := telemetry.OpenClickHouse(address, "argus_telemetry", "argus", os.Getenv("ARGUS_CLICKHOUSE_TEST_PASSWORD"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx := context.Background()
	tenant, host, source := uuid.New(), uuid.New(), uuid.New()
	router := telemetry.TenantTableRouter{}
	manager := telemetry.ClickHouseTenantSchemaManager{Conn: conn, Router: router}
	if err := manager.EnsureTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.DropTenant(ctx, tenant) })
	tables, _ := router.Tables(tenant)
	at := time.Now().UTC().Truncate(time.Second)
	seedErrorCounters(t, ctx, conn, tables, host, source, at)
	engine := promqlengine.NewEngine(conn, router, nil)
	b := Builder{Operation: "error_rate", Metric: "planv2_requests_total", MetricType: "counter", WindowSeconds: 300, GroupBy: []string{"service"}, ErrorFilters: []Filter{{Field: "status", Operator: "=~", Value: "5.."}}}
	query, err := compileMetricBuilder(b)
	if err != nil {
		t.Fatal(err)
	}
	request := promqlengine.Request{Expression: query.Expression, Instant: true, Start: at.Add(-5 * time.Minute), End: at, Scope: promqlengine.Scope{EnterpriseID: tenant, ResourceIDs: []uuid.UUID{host}, SourceKeys: []string{source.String() + ":1"}}, MaxScanBytes: telemetry.DefaultMaxScanBytes}
	result, err := engine.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	vector, ok := result.Value.(promql.Vector)
	if !ok || len(vector) != 2 {
		t.Fatalf("zero traffic should not be reported healthy: %+v", result.Value)
	}
	got := map[string]float64{}
	for _, sample := range vector {
		got[sample.Metric.Get("service")] = sample.F
	}
	if math.Abs(got["orders"]-.2) > 1e-10 || got["healthy"] != 0 {
		t.Fatalf("incorrect request ratio: %+v", got)
	}
	request.Scope.SourceKeys = []string{uuid.NewString() + ":1"}
	result, err = engine.Execute(ctx, request)
	if err != nil || len(result.Value.(promql.Vector)) != 0 {
		t.Fatalf("foreign source leaked: %+v %v", result.Value, err)
	}
	request.Scope.SourceKeys = []string{source.String() + ":1"}
	request.Scope.ResourceIDs = []uuid.UUID{uuid.New()}
	result, err = engine.Execute(ctx, request)
	if err != nil || len(result.Value.(promql.Vector)) != 0 {
		t.Fatalf("foreign resource leaked: %+v %v", result.Value, err)
	}
	request.Scope.ResourceIDs = []uuid.UUID{host}
	request.Start = at.Add(time.Hour)
	request.End = at.Add(2 * time.Hour)
	result, err = engine.Execute(ctx, request)
	if err != nil || len(result.Value.(promql.Vector)) != 0 {
		t.Fatalf("absent samples fabricated zero: %+v %v", result.Value, err)
	}
}
