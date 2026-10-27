package telemetry

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/chscope"
	promqlengine "github.com/kakj-go/Argus/internal/telemetry/queryengine/promql"
)

func OpenClickHouse(address, database, username, password string) (driver.Conn, error) {
	if address == "" || username == "" || password == "" {
		return nil, ErrUnavailable
	}
	return clickhouse.Open(&clickhouse.Options{Addr: []string{address}, Auth: clickhouse.Auth{Database: database, Username: username, Password: password}, DialTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, MaxOpenConns: 20, MaxIdleConns: 5})
}

type ClickHouseQuery struct {
	Conn   driver.Conn
	Router TenantTableRouter
	PromQL *promqlengine.Engine
}

func boundedOverviewContext(ctx context.Context, request OverviewRequest) context.Context {
	return clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"max_bytes_to_read": request.MaxScanBytes, "max_execution_time": request.Timeout.Seconds(), "max_result_rows": 1, "result_overflow_mode": "throw"}))
}

func (query ClickHouseQuery) Overview(ctx context.Context, request OverviewRequest) (Overview, error) {
	result := Overview{ResourceCount: len(request.ResourceIDs), WindowSeconds: int(request.To.Sub(request.From).Seconds())}
	if len(request.ResourceIDs) == 0 || len(request.SourceKeys) == 0 {
		return result, nil
	}
	if query.Conn == nil {
		return Overview{}, ErrQueryBackend
	}
	tables, err := query.Router.Tables(request.EnterpriseID)
	if err != nil {
		return Overview{}, err
	}
	ctx, err = chscope.WithSources(ctx, request.SourceKeys)
	if err != nil {
		return result, err
	}
	var metricPoints, logRecords, spans uint64
	metricWhere := "resource_id IN (?) AND " + chscope.BasePredicate("source_key") + " AND timestamp >= fromUnixTimestamp64Nano(?, 'UTC') AND timestamp < fromUnixTimestamp64Nano(?, 'UTC')"
	metricArgs := []any{request.ResourceIDs, request.From.UnixNano(), request.To.UnixNano()}
	if strings.Contains(strings.Join(request.SourceKeys, ","), "@") {
		predicate := chscope.FacetedPredicate("source_key", "resource_attributes['argus.namespace.uid']")
		metricWhere += " AND series_id IN (SELECT series_id FROM " + quoteIdentifier(tables.MetricSeries) + " FINAL WHERE resource_id IN (?) AND " + predicate + ")"
		metricArgs = append(metricArgs, request.ResourceIDs)
	}
	sql := fmt.Sprintf(`SELECT
(SELECT count() FROM %s FINAL WHERE %s),
(SELECT count() FROM %s FINAL WHERE resource_id IN (?) AND %s AND timestamp >= fromUnixTimestamp64Nano(?, 'UTC') AND timestamp < fromUnixTimestamp64Nano(?, 'UTC')),
(SELECT count() FROM %s FINAL WHERE resource_id IN (?) AND %s AND start_time >= fromUnixTimestamp64Nano(?, 'UTC') AND start_time < fromUnixTimestamp64Nano(?, 'UTC'))`, quoteIdentifier(tables.MetricSamples), metricWhere, quoteIdentifier(tables.Logs), chscope.ExactPredicate("source_key"), quoteIdentifier(tables.Traces), chscope.ExactPredicate("source_key"))
	args := append(metricArgs, request.ResourceIDs, request.From.UnixNano(), request.To.UnixNano(), request.ResourceIDs, request.From.UnixNano(), request.To.UnixNano())
	if err := query.Conn.QueryRow(boundedOverviewContext(ctx, request), sql, args...).Scan(&metricPoints, &logRecords, &spans); err != nil {
		return Overview{}, err
	}
	if result.MetricPoints, err = checkedInt64FromUInt64(metricPoints); err != nil {
		return Overview{}, err
	}
	if result.LogRecords, err = checkedInt64FromUInt64(logRecords); err != nil {
		return Overview{}, err
	}
	if result.Spans, err = checkedInt64FromUInt64(spans); err != nil {
		return Overview{}, err
	}
	return result, nil
}

func checkedInt64FromUInt64(value uint64) (int64, error) {
	if value > uint64(math.MaxInt64) {
		return 0, fmt.Errorf("telemetry count %d exceeds int64 range", value)
	}
	return int64(value), nil
}

func quoteIdentifier(value string) string { return "`" + strings.ReplaceAll(value, "`", "") + "`" }

var _ OverviewBackend = ClickHouseQuery{}
