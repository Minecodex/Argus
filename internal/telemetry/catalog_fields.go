package telemetry

import (
	"context"
	"strings"
)

// Fields describe stored values, not types inferred from a few samples. Native
// optional fields appear only when observed; map keys come from scoped rows.
// Parsed JSON/logfmt fields belong to their explicit KQL pipeline stages.
func catalogFieldPairs(signal string) (string, error) {
	var fixed []string
	var maps []string
	switch signal {
	case "metrics":
		fixed = []string{"tuple('__name__','string')"}
		maps = []string{"arrayMap(k -> tuple(k,'string'), mapKeys(labels))"}
	case "logs":
		fixed = []string{"tuple('timestamp','datetime')", "tuple('severity_number','number')"}
		for _, field := range []string{"body", "severity_text", "service_name", "trace_id", "span_id", "event_id", "source_type", "source_key"} {
			fixed = append(fixed, "if(notEmpty("+field+"),tuple('"+field+"','string'),tuple('','string'))")
		}
		for _, field := range []string{"stream_labels", "structured_metadata", "resource_attributes"} {
			maps = append(maps, "arrayMap(k -> tuple(concat('"+field+".',k),'string'), mapKeys("+field+"))")
		}
	case "traces":
		fixed = []string{"tuple('span_kind','number')", "tuple('source_id','string')", "tuple('resource_id','string')"}
		for _, field := range []string{"service_name", "operation", "status", "trace_id", "span_id"} {
			fixed = append(fixed, "if(notEmpty("+field+"),tuple('"+field+"','string'),tuple('','string'))")
		}
		for _, field := range []string{"attributes", "resource_attributes"} {
			maps = append(maps, "arrayMap(k -> tuple(concat('"+field+".',k),'string'), mapKeys("+field+"))")
		}
	default:
		return "", ErrQueryInvalid
	}
	return "arrayConcat([" + strings.Join(fixed, ",") + "]," + strings.Join(maps, ",") + ")", nil
}

func (query ClickHouseQuery) discoverCatalogFields(ctx context.Context, request DataCatalogRequest, table, where string, args []any, after string, result *DataCatalogResult) error {
	pairs, err := catalogFieldPairs(request.Signal)
	if err != nil {
		return err
	}
	// Scope and predicates apply before ARRAY JOIN and DISTINCT. There is no
	// sample limit: max_bytes_to_read and the timeout bound the whole discovery.
	sql := "SELECT DISTINCT pair.1 AS name,pair.2 AS type FROM (SELECT arrayJoin(" + pairs + ") AS pair FROM `" + table + "` FINAL WHERE " + where + ") WHERE name!='' AND length(name)<=256"
	if request.Signal == "metrics" {
		sql += " AND match(name,'^[A-Za-z_][A-Za-z0-9_]*$')"
	} else {
		// The query editor's field grammar must be able to address the result.
		sql += " AND match(name,'^[A-Za-z_][A-Za-z0-9_.-]*$')"
	}
	if request.Search != "" {
		sql += " AND positionCaseInsensitive(name,?)>0"
		args = append(args, request.Search)
	}
	if after != "" {
		sql += " AND name>?"
		args = append(args, after)
	}
	rows, err := query.Conn.Query(ctx, sql+" ORDER BY name LIMIT ?", append(args, request.Limit+1)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var field CatalogField
		if err := rows.Scan(&field.Name, &field.Type); err != nil {
			return err
		}
		if len(result.Fields) == request.Limit {
			result.Complete, result.HasMore = false, true
			result.NextCursor = writeCatalogCursor(request, result.Fields[len(result.Fields)-1].Name)
			break
		}
		result.Fields = append(result.Fields, field)
	}
	return rows.Err()
}
