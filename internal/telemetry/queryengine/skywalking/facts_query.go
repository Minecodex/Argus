package skywalking

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Facts are deduplicated before aggregation, independently of ClickHouse's
// asynchronous merges. Config revisions share an installation identity;
// reinstallations and distinct resources/sources remain separate traces.
func (state *executionState) spanFacts(where string) string {
	return `SELECT resource_id,source_id,trace_id,span_id,
 latest.1 AS parent_span_id,latest.2 AS service_name,latest.3 AS operation,
 latest.4 AS status,latest.5 AS start_time,latest.6 AS end_time,latest.7 AS duration_ns,
 latest.8 AS attributes,latest.9 AS events,latest.10 AS links,latest.11 AS resource_attributes,
 latest.12 AS scope_name,latest.13 AS span_kind,latest.14 AS status_message,latest.15 AS trace_state,latest.16 AS source_type
 FROM (SELECT resource_id,source_id,trace_id,span_id,
 argMax(tuple(parent_span_id,service_name,operation,status,start_time,end_time,duration_ns,attributes,events,links,resource_attributes,scope_name,span_kind,status_message,trace_state,source_type),
 tuple(source_revision,kafka_partition,kafka_offset,record_sequence,ingest_key)) AS latest
 FROM ` + "`" + state.tables.spans + "`" + ` WHERE ` + where + ` GROUP BY resource_id,source_id,trace_id,span_id)`
}

func (state *executionState) traceSummary(where string) string {
	return traceSummaryFacts(state.spanFacts(where))
}
func traceSummaryFacts(facts string) string {
	return `SELECT resource_id,source_id,trace_id,
 argMin(service_name,tuple(parent_span_id!='',facts.start_time,span_id)) AS root_service,
 argMin(operation,tuple(parent_span_id!='',facts.start_time,span_id)) AS root_operation,
 min(facts.start_time) AS start_time, max(facts.start_time) AS latest_sample_time,
 toUInt64(greatest(0,max(toUnixTimestamp64Nano(end_time))-min(toUnixTimestamp64Nano(facts.start_time)))) AS duration_ns,
 toUInt32(count()) AS span_count,toUInt32(countIf(facts.status='error')) AS error_count,
 if(countIf(facts.status='error')>0,'error',if(countIf(facts.status='unset')>0,'unset','ok')) AS status,
 countIf(parent_span_id='')>0 AS root_present,
 toUInt32(arrayCount(p -> p!='',arrayExcept(groupUniqArray(parent_span_id),groupUniqArray(span_id)))) AS missing_parent_count,
 toUInt32(countIf(parent_span_id='')) AS root_count
 FROM (` + facts + `) AS facts GROUP BY resource_id,source_id,trace_id`
}

const traceColumns = "resource_id,source_id,trace_id,root_service,root_operation,start_time,duration_ns,span_count,error_count,status,root_present,missing_parent_count,root_count,latest_sample_time"

func (state *executionState) queryTracePage(ctx context.Context, filter traceFilter) (uint64, []traceRecord, error) {
	predicate, args := state.spanFilterScope()
	for _, f := range []struct {
		column string
		value  *string
	}{{"source_id", filter.sourceID}, {"resource_id", filter.resourceID}, {"trace_id", filter.traceID}} {
		if f.value != nil {
			predicate += " AND " + f.column + "=?"
			args = append(args, *f.value)
		}
	}
	facts := state.spanFacts(predicate)
	attributes, attributeValues := apmAttributeSQL(filter.filters)
	if attributes != "" {
		facts = "SELECT * FROM (" + facts + ") WHERE 1" + attributes
		args = append(args, attributeValues...)
	}
	summary := traceSummaryFacts(facts)
	factsArgs := append([]any{}, args...)
	where := "1"
	for _, f := range []struct {
		field string
		value *string
	}{{"root_service", filter.serviceName}, {"root_operation", filter.operationName}, {"status", filter.status}} {
		if f.value != nil {
			value := *f.value
			if f.field == "status" {
				value = strings.ToLower(value)
			}
			where += " AND " + f.field + "=?"
			args = append(args, value)
		}
	}
	if filter.durationMin != nil {
		if *filter.durationMin < 0 {
			return 0, nil, fmt.Errorf("durationMin must be non-negative")
		}
		where += " AND duration_ns>=?"
		args = append(args, uint64(*filter.durationMin*1e6))
	}
	if filter.durationMax != nil {
		if *filter.durationMax < 0 {
			return 0, nil, fmt.Errorf("durationMax must be non-negative")
		}
		where += " AND duration_ns<=?"
		args = append(args, uint64(*filter.durationMax*1e6))
	}
	if filter.durationMin != nil && filter.durationMax != nil && *filter.durationMin > *filter.durationMax {
		return 0, nil, fmt.Errorf("invalid duration range")
	}
	if filter.serviceInstanceName != nil || filter.tags != nil && len(*filter.tags) > 0 {
		where += " AND (resource_id,source_id,trace_id) IN (SELECT resource_id,source_id,trace_id FROM (" + facts + ") WHERE 1"
		args = append(args, factsArgs...)
		if filter.serviceInstanceName != nil {
			value := *filter.serviceInstanceName
			where += " AND (resource_attributes['service.instance.name']=? OR resource_attributes['service.instance.id']=?)"
			args = append(args, value, value)
		}
		if filter.tags != nil {
			for _, tag := range *filter.tags {
				if tag.Key == "" || len(tag.Key) > 128 || len(tag.Value) > 1024 {
					return 0, nil, fmt.Errorf("invalid trace tag")
				}
				where += " AND attributes[?]=?"
				args = append(args, tag.Key, tag.Value)
			}
		}
		where += ")"
	}
	order := "start_time DESC,trace_id,resource_id,source_id"
	switch strings.ToUpper(stringValue(filter.order)) {
	case "", "START_TIME_DESC":
	case "START_TIME_ASC":
		order = "start_time ASC,trace_id,resource_id,source_id"
	default:
		return 0, nil, fmt.Errorf("unsupported trace order")
	}
	limit := state.request.Budget.MaxRows
	if filter.pageSize != nil {
		if *filter.pageSize < 1 || int(*filter.pageSize) > limit {
			return 0, nil, ErrBudget
		}
		limit = int(*filter.pageSize)
	}
	page := 1
	if filter.pageNum != nil {
		if *filter.pageNum < 1 || *filter.pageNum > 1000000 {
			return 0, nil, ErrBudget
		}
		page = int(*filter.pageNum)
	}
	base := " FROM (" + summary + ") WHERE " + where
	var total uint64
	if err := state.engine.Conn.QueryRow(queryContext(ctx, state.request.Budget), "SELECT count()"+base, args...).Scan(&total); err != nil {
		return 0, nil, err
	}
	rows, err := state.engine.Conn.Query(queryContext(ctx, state.request.Budget), "SELECT "+traceColumns+base+" ORDER BY "+order+" LIMIT ? OFFSET ?", append(args, limit, (page-1)*limit)...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	items := []traceRecord{}
	for rows.Next() {
		item, err := scanTrace(rows)
		if err != nil {
			return 0, nil, err
		}
		state.request.Budget.progress.ObserveEvent(item.latestSampleTime)
		items = append(items, item)
	}
	if err := state.recordRows(len(items)); err != nil {
		return 0, nil, err
	}
	remaining := total - min(total, uint64(page-1)*uint64(limit))
	if filter.pageSize == nil && remaining > uint64(len(items)) {
		state.mu.Lock()
		state.partial = true
		state.warnings = append(state.warnings, "system row budget truncated the trace query result")
		state.mu.Unlock()
	}
	return total, items, rows.Err()
}

type traceScanner interface{ Scan(...any) error }

func scanTrace(row traceScanner) (traceRecord, error) {
	var item traceRecord
	var resource, source uuid.UUID
	err := row.Scan(&resource, &source, &item.traceID, &item.rootService, &item.rootOperation, &item.startTime, &item.duration, &item.spanCount, &item.errorCount, &item.status, &item.rootPresent, &item.missingParents, &item.rootCount, &item.latestSampleTime)
	item.resourceID, item.sourceID = resource.String(), source.String()
	return item, err
}

func (state *executionState) queryTrace(ctx context.Context, traceID, sourceID, resourceID string) (traceRecord, error) {
	where, args := state.spanFilterScope()
	where += " AND trace_id=?"
	args = append(args, traceID)
	if sourceID != "" {
		where += " AND source_id=?"
		args = append(args, sourceID)
	}
	if resourceID != "" {
		where += " AND resource_id=?"
		args = append(args, resourceID)
	}
	rows, err := state.engine.Conn.Query(queryContext(ctx, state.request.Budget), "SELECT "+traceColumns+" FROM ("+state.traceSummary(where)+") ORDER BY resource_id,source_id LIMIT 2", args...)
	if err != nil {
		return traceRecord{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return traceRecord{}, err
		}
		return traceRecord{}, sql.ErrNoRows
	}
	item, err := scanTrace(rows)
	if err != nil {
		return item, err
	}
	state.request.Budget.progress.ObserveEvent(item.latestSampleTime)
	if rows.Next() {
		return traceRecord{}, fmt.Errorf("trace identity is ambiguous; select sourceId and resourceId from the trace list")
	}
	if err := state.recordRows(1); err != nil {
		return traceRecord{}, err
	}
	return item, rows.Err()
}

func (state *executionState) recordRows(count int) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.rows+state.relations+count > state.request.Budget.MaxRows {
		return ErrBudget
	}
	state.rows += count
	return nil
}
