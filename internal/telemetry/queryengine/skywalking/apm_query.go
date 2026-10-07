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
	state.mu.Lock()
	remaining := state.request.Budget.MaxRows - state.rows - state.relations - 1
	state.mu.Unlock()
	limit := remaining
	if args.Limit != nil {
		limit = int(*args.Limit)
	}
	if limit < 1 || limit > remaining {
		return nil, ErrBudget
	}
	base, values := state.apmSamples(args)
	query, arguments := apmWindowQuery(base, group, bucket, state.request.Start)
	values = append(values, arguments...)
	rows, err := state.engine.Conn.Query(queryContext(ctx, state.request.Budget), query, append(values, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c := &result.coverage
	for rows.Next() {
		var r apmRowResolver
		var resource, source uuid.UUID
		var latestSample *time.Time
		var count uint64
		var visible uint8
		if err := rows.Scan(&resource, &source, &r.service, &r.instanceID, &r.instanceName, &r.operation, &r.at, &r.observed, &r.samples, &r.errors, &r.mean, &r.quantiles,
			&c.observed, &c.requests, &c.missingService, &c.missingInstance, &c.missingOperation, &c.unknownSource, &latestSample, &count, &visible); err != nil {
			return nil, err
		}
		if latestSample != nil {
			state.request.Budget.progress.ObserveEvent(*latestSample)
		}
		if count > uint64(limit) {
			if args.Limit == nil {
				return nil, ErrBudget
			}
			result.limited = true
		}
		if visible == 0 {
			continue
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
	if err := state.recordRows(len(result.rows) + 1); err != nil {
		return nil, err
	}
	if c.observed == 0 {
		result.status = "no_data"
	} else if c.requests == 0 {
		result.status = "insufficient_request_samples"
	}
	if c.missingService > 0 || group == "instances" && c.missingInstance > 0 || group == "endpoints" && c.missingOperation > 0 {
		result.status = "incomplete_dimensions"
	}
	if c.unknownSource > 0 {
		result.status = "unknown_source"
	}
	return result, nil
}
