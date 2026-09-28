package skywalking

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const entrySpan = "span_kind IN (2,5)"

func (state *executionState) apmSamples(args apmArgs) (string, []any) {
	where, values := state.spanFilterScope()
	// Source/resource arguments may narrow the already-authorized scope only.
	if value := stringValue(args.SourceID); value != "" {
		where += " AND source_id=?"
		values = append(values, value)
	}
	if value := stringValue(args.ResourceID); value != "" {
		where += " AND resource_id=?"
		values = append(values, value)
	}
	query := "WITH facts AS (" + state.spanFacts(where) + "), samples AS (SELECT * FROM facts WHERE 1"
	for _, f := range []struct {
		column string
		value  *string
	}{{"service_name", args.ServiceName}, {"operation", args.OperationName}} {
		if f.value != nil {
			query += " AND " + f.column + "=?"
			values = append(values, *f.value)
		}
	}
	if args.ServiceInstanceName != nil {
		value := *args.ServiceInstanceName
		query += " AND (resource_attributes['service.instance.id']=? OR resource_attributes['service.instance.name']=?)"
		values = append(values, value, value)
	}
	attributes, filterValues := apmAttributeSQL(args.Filters)
	return query + attributes + ") ", append(values, filterValues...)
}

func (state *executionState) queryAPM(ctx context.Context, group string, args apmArgs, bucket int32) (*apmResultResolver, error) {
	result := &apmResultResolver{group: group, start: state.request.Start, end: state.request.End, rows: []*apmRowResolver{}, status: "available"}
	base, values := state.apmSamples(args)
	coverageSQL := base + "SELECT count(),countIf(" + entrySpan + "),countIf(service_name=''),countIf(resource_attributes['service.instance.id']=''),countIf(operation=''),countIf(source_id=toUUID('00000000-0000-0000-0000-000000000000') OR source_type IN ('','unknown')),maxOrNull(start_time) FROM samples"
	c := &result.coverage
	var latestSample *time.Time
	if err := state.engine.Conn.QueryRow(queryContext(ctx, state.request.Budget), coverageSQL, values...).Scan(&c.observed, &c.requests, &c.missingService, &c.missingInstance, &c.missingOperation, &c.unknownSource, &latestSample); err != nil {
		return nil, err
	}
	if latestSample != nil {
		state.request.Budget.progress.ObserveEvent(*latestSample)
	}
	if err := state.recordRows(1); err != nil {
		return nil, err
	}
	if c.observed == 0 {
		result.status = "no_data"
		return result, nil
	}
	if c.requests == 0 {
		result.status = "insufficient_request_samples"
	}
	if c.missingService > 0 || group == "instances" && c.missingInstance > 0 || group == "endpoints" && c.missingOperation > 0 {
		result.status = "incomplete_dimensions"
	}
	if c.unknownSource > 0 {
		result.status = "unknown_source"
	}
	state.mu.Lock()
	remaining := state.request.Budget.MaxRows - state.rows - state.relations
	state.mu.Unlock()
	limit := remaining
	if args.Limit != nil {
		limit = int(*args.Limit)
	}
	if limit < 1 || limit > remaining {
		return nil, ErrBudget
	}
	instance, instanceName, operation := "''", "''", "''"
	groups := "resource_id,source_id,service_name"
	conditions := "service_name!=''"
	if group == "instances" {
		instance = "resource_attributes['service.instance.id']"
		instanceName = "argMax(resource_attributes['service.instance.name'],tuple(start_time,span_id))"
		groups += ",resource_attributes['service.instance.id']"
		conditions += " AND resource_attributes['service.instance.id']!=''"
	}
	if group == "endpoints" {
		operation = "operation"
		groups += ",operation"
		conditions += " AND operation!=''"
	}
	bucketSQL := "toDateTime64(?,9,'UTC')"
	rowArgs := append(append([]any{}, values...), state.request.Start)
	if group == "red" {
		bucketSQL = "fromUnixTimestamp64Nano(toUnixTimestamp64Nano(toDateTime64(?,9,'UTC'))+intDiv(toUnixTimestamp64Nano(start_time)-toUnixTimestamp64Nano(toDateTime64(?,9,'UTC')),?)*?, 'UTC')"
		bucketNanos := int64(bucket) * int64(time.Second)
		rowArgs = append(rowArgs, state.request.Start, bucketNanos, bucketNanos)
		groups += ",bucket_start"
	}
	query := base + `SELECT resource_id,source_id,service_name,` + instance + `,` + instanceName + `,` + operation + `,` + bucketSQL + ` AS bucket_start,
 count(),countIf(` + entrySpan + `),countIf((` + entrySpan + `) AND status='error'),
 avgIf(toFloat64(duration_ns)/1e6,` + entrySpan + `),quantilesTDigestIf(0.5,0.95,0.99)(toFloat64(duration_ns)/1e6,` + entrySpan + `)
 FROM samples WHERE ` + conditions + ` GROUP BY ` + groups + ` ORDER BY bucket_start,source_id,resource_id,service_name,` + instance + `,` + operation + ` LIMIT ?`
	rows, err := state.engine.Conn.Query(queryContext(ctx, state.request.Budget), query, append(rowArgs, limit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		if len(result.rows) == limit {
			if args.Limit == nil {
				return nil, ErrBudget
			}
			result.limited = true
			break
		}
		var r apmRowResolver
		var resource, source uuid.UUID
		if err := rows.Scan(&resource, &source, &r.service, &r.instanceID, &r.instanceName, &r.operation, &r.at, &r.observed, &r.samples, &r.errors, &r.mean, &r.quantiles); err != nil {
			return nil, err
		}
		r.sourceID, r.resourceID = source.String(), resource.String()
		r.seconds = state.request.End.Sub(state.request.Start).Seconds()
		if group == "red" {
			r.seconds = min(float64(bucket), state.request.End.Sub(r.at).Seconds())
		}
		if r.seconds <= 0 {
			return nil, fmt.Errorf("invalid APM evaluation interval")
		}
		result.rows = append(result.rows, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := state.recordRows(len(result.rows)); err != nil {
		return nil, err
	}
	return result, nil
}
