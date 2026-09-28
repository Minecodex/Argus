package kql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/chstats"
)

func Execute(ctx context.Context, conn driver.Conn, router TableRouter, request Request) (Result, error) {
	if conn == nil || router == nil {
		return Result{}, fmt.Errorf("KQL storage unavailable")
	}
	if request.Scope.EnterpriseID == uuid.Nil {
		return Result{}, fmt.Errorf("enterprise id required")
	}
	if request.Budget.MaxRows <= 0 {
		request.Budget.MaxRows = 50000
	}
	table, err := router.Table("logs", request.Scope.EnterpriseID)
	if err != nil {
		return Result{}, err
	}
	plan, err := CompileQuery(table, request)
	if err != nil {
		return Result{}, err
	}
	started := time.Now()
	progress := &chstats.Tracker{}
	settings := clickhouse.Settings{"max_result_rows": plan.Pipeline.Limit + 1, "max_execution_time": max(1, int(request.Budget.Timeout.Seconds()))}
	if request.Budget.MaxScanBytes > 0 {
		settings["max_bytes_to_read"] = request.Budget.MaxScanBytes
	}
	rows, err := conn.Query(progress.Context(ctx, settings), plan.SQL, plan.Args...)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	result := Result{Data: []map[string]any{}, Warnings: []string{}, ResultType: plan.ResultType, Columns: plan.Columns}
	for rows.Next() {
		if len(result.Data) >= plan.Pipeline.Limit {
			result.Partial = true
			result.Warnings = append(result.Warnings, "system row budget truncated the query result")
			break
		}
		values := make([]any, len(plan.Columns))
		for i, column := range plan.Columns {
			switch column.Type {
			case "timestamp":
				values[i] = new(time.Time)
			case "uuid":
				values[i] = new(uuid.UUID)
			case "uint8":
				values[i] = new(uint8)
			case "uint64":
				values[i] = new(uint64)
			case "int64":
				values[i] = new(int64)
			case "nullable_float64":
				values[i] = new(*float64)
			default:
				values[i] = new(string)
			}
		}
		if err := rows.Scan(values...); err != nil {
			return Result{}, err
		}
		item := make(map[string]any, len(values))
		for i, column := range plan.Columns {
			switch value := values[i].(type) {
			case *time.Time:
				item[column.Name] = *value
				if plan.Pipeline.Aggregate == nil && column.Name == "timestamp" {
					progress.ObserveEvent(*value)
				}
			case *uuid.UUID:
				item[column.Name] = *value
			case *uint8:
				item[column.Name] = *value
			case *uint64:
				item[column.Name] = *value
			case *int64:
				item[column.Name] = *value
			case **float64:
				if *value == nil {
					item[column.Name] = nil
				} else {
					item[column.Name] = **value
				}
			case *string:
				item[column.Name] = *value
			}
		}
		if plan.Pipeline.Parser != "" && plan.Pipeline.Aggregate == nil {
			body, _ := item["body"].(string)
			parsed := parsePipelineBody(body, plan.Pipeline.Parser)
			item["parsed_fields"] = parsed
			if field := plan.Pipeline.Unwrap; field != "" {
				_, key, found := strings.Cut(field, ".")
				if !found {
					key = field
				}
				if value, ok := parsed[key]; ok {
					item["unwrap"] = typedValue(value)
				} else {
					item["unwrap"] = nil
				}
			}
		}
		result.Data = append(result.Data, item)
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	result.Elapsed = time.Since(started)
	result.ScannedBytes = progress.Bytes()
	result.ScannedRows = progress.Rows()
	result.LatestSampleAt = progress.LatestEvent()
	return result, nil
}
