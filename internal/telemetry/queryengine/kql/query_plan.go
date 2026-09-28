package kql

import (
	"fmt"
	"strings"
)

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type QueryPlan struct {
	SQL        string
	Args       []any
	Pipeline   Pipeline
	ResultType string
	Columns    []Column
}

// CompileQuery is shared by validation and execution. Authorization is always
// applied to raw rows before aggregation; runtime conditions cannot alter the
// meaning of an aggregate or bypass resource scope.
func CompileQuery(table string, request Request) (QueryPlan, error) {
	var plan QueryPlan
	if !safeIdentifier(table) {
		return plan, fmt.Errorf("invalid tenant table")
	}
	if len(request.Scope.ResourceIDs) == 0 {
		return plan, fmt.Errorf("KQL resource scope required")
	}
	if !request.End.After(request.Start) {
		return plan, fmt.Errorf("invalid time range")
	}
	stages, err := SplitStages(request.Expression)
	if err != nil {
		return plan, err
	}
	pipeline := request.Pipeline
	if len(stages) > 1 {
		if pipeline != "" {
			return plan, fmt.Errorf("pipeline supplied twice")
		}
		pipeline = strings.Join(stages[1:], " | ")
	}
	plan.Pipeline, err = ParsePipeline(pipeline, request.Budget.MaxRows)
	if err != nil {
		return plan, err
	}
	where := "timestamp >= ? AND timestamp < ? AND resource_id IN (?)"
	whereArgs := []any{request.Start, request.End, request.Scope.ResourceIDs}
	if len(request.Scope.SourceKeys) > 0 {
		where += " AND source_key IN (?)"
		whereArgs = append(whereArgs, request.Scope.SourceKeys)
	}
	if stages[0] != "*" {
		expr, err := Parse(stages[0])
		if err != nil {
			return plan, err
		}
		compiler := Compiler{}
		filter, err := compiler.Compile(expr)
		if err != nil {
			return plan, err
		}
		where += " AND (" + filter + ")"
		whereArgs = append(whereArgs, compiler.Args...)
	}
	for _, expr := range plan.Pipeline.Filters {
		compiler := Compiler{Parser: plan.Pipeline.Parser}
		filter, err := compiler.Compile(expr)
		if err != nil {
			return plan, err
		}
		where += " AND (" + filter + ")"
		whereArgs = append(whereArgs, compiler.Args...)
	}
	selection := "timestamp, resource_id, severity_text, severity_number, service_name, body, trace_id, span_id, event_id, source_id, source_revision, source_type"
	plan.ResultType = "log_entries"
	plan.Columns = []Column{{"timestamp", "timestamp"}, {"resource_id", "uuid"}, {"severity_text", "string"}, {"severity_number", "uint8"}, {"service_name", "string"}, {"body", "string"}, {"trace_id", "string"}, {"span_id", "string"}, {"event_id", "string"}}
	plan.Columns = append(plan.Columns, Column{"source_id", "uuid"}, Column{"source_revision", "int64"}, Column{"source_type", "string"})
	if plan.Pipeline.Context != nil {
		plan.SQL, plan.Args = contextQuery(table, selection, where, whereArgs, *plan.Pipeline.Context)
		return plan, nil
	}
	grouping := ""
	if a := plan.Pipeline.Aggregate; a != nil {
		plan.ResultType = "table"
		plan.Columns = nil
		var selects, groups []string
		if a.Bucket > 0 {
			selects = append(selects, "fromUnixTimestamp64Milli(intDiv(toUnixTimestamp64Milli(timestamp), ?) * ?, 'UTC') AS timestamp")
			plan.Args = append(plan.Args, a.Bucket.Milliseconds(), a.Bucket.Milliseconds())
			groups = append(groups, "timestamp")
			plan.Columns = append(plan.Columns, Column{"timestamp", "timestamp"})
			plan.ResultType = "timeseries"
		}
		if a.Group != "" {
			field, key, _ := allowedFieldForParser(a.Group, plan.Pipeline.Parser)
			if key != "" {
				plan.Args = append(plan.Args, key)
			}
			selects = append(selects, "toString("+field+") AS group_value")
			groups = append(groups, "group_value")
			plan.Columns = append(plan.Columns, Column{"group_value", "string"})
		}
		if a.Function == "count" {
			selects = append(selects, "count() AS count")
			plan.Columns = append(plan.Columns, Column{"count", "uint64"})
		} else {
			field, key, _ := allowedFieldForParser(a.Field, plan.Pipeline.Parser)
			if strings.HasPrefix(a.Field, "json.") {
				field = "JSONExtractRaw(body, ?)"
			}
			if key != "" {
				plan.Args = append(plan.Args, key)
			}
			function := a.Function
			if function == "p95" {
				function = "quantile(0.95)"
			}
			selects = append(selects, function+"(toFloat64OrNull(toString("+field+"))) AS value")
			plan.Columns = append(plan.Columns, Column{"value", "nullable_float64"})
		}
		selection = strings.Join(selects, ", ")
		if len(groups) > 0 {
			grouping = " GROUP BY " + strings.Join(groups, ", ")
		}
	}
	order := plan.Pipeline.SortField + " " + plan.Pipeline.SortDirection
	if plan.Pipeline.Aggregate == nil {
		order += ", resource_id ASC, event_id ASC"
	} else if plan.Pipeline.Aggregate.Group != "" && plan.Pipeline.SortField != "group_value" {
		order += ", group_value ASC"
	}
	plan.SQL = "SELECT " + selection + " FROM `" + table + "` FINAL WHERE " + where + grouping + " ORDER BY " + order + " LIMIT ?"
	plan.Args = append(plan.Args, whereArgs...)
	limit := plan.Pipeline.Limit
	if !plan.Pipeline.ExplicitLimit {
		limit++
	} // detect system truncation without hiding it
	plan.Args = append(plan.Args, limit)
	return plan, nil
}

func safeIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}
