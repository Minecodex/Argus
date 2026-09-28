package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/chstats"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
)

type CatalogFilter struct {
	Field    string   `json:"field"`
	Operator string   `json:"operator,omitempty"`
	Values   []string `json:"values"`
}
type DataCatalogRequest struct {
	SubjectID                           uuid.UUID
	SubjectType                         string
	EnterpriseID                        uuid.UUID
	ResourceIDs                         []uuid.UUID
	SourceKeys                          []string
	AuthorizationVersion                int64
	Signal, Kind, Metric, Field, Search string
	Cursor                              string
	SelectedValues                      []string
	Filters                             []CatalogFilter
	From, To                            time.Time
	Limit                               int
	Budget                              queryengine.Budget
}
type MetricDescriptor struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Unit   string   `json:"unit"`
	Labels []string `json:"labels"`
}
type CatalogField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type DataCatalogResult struct {
	Metrics    []MetricDescriptor    `json:"metrics"`
	Fields     []CatalogField        `json:"fields"`
	Values     []string              `json:"values"`
	Complete   bool                  `json:"complete"`
	HasMore    bool                  `json:"has_more"`
	NextCursor string                `json:"next_cursor,omitempty"`
	Membership map[string]bool       `json:"selected_exists"`
	Meta       queryengine.QueryMeta `json:"meta"`
}

type DataCatalogBackend interface {
	DiscoverData(context.Context, DataCatalogRequest) (DataCatalogResult, error)
}

func (query ClickHouseQuery) DiscoverData(ctx context.Context, request DataCatalogRequest) (result DataCatalogResult, resultErr error) {
	defer func() { resultErr = queryengine.NormalizeExecutionError(resultErr) }()
	result = DataCatalogResult{Metrics: []MetricDescriptor{}, Fields: []CatalogField{}, Values: []string{}, Membership: map[string]bool{}, Complete: true}
	after, cursorErr := readCatalogCursor(request)
	if cursorErr != nil {
		return result, cursorErr
	}
	result.Complete = request.Cursor == "" && request.Search == ""
	if query.Conn == nil || request.EnterpriseID == uuid.Nil || len(request.ResourceIDs) == 0 || len(request.SourceKeys) == 0 || !request.To.After(request.From) || len(request.SelectedValues) > 200 || len(request.Filters) > 32 {
		return result, ErrQueryInvalid
	}
	// Do not place credential values (or a credential continuation key) in a
	// candidate response. This is a uniform data policy, never a role privilege.
	if request.Kind == "values" && datapolicy.CredentialKey(request.Field) {
		result.Complete = false
		result.Meta.Warnings = []string{"CREDENTIAL_VALUES_REDACTED"}
		return result, nil
	}
	if request.Limit <= 0 {
		request.Limit = 200
	}
	if request.Limit > 1000 {
		return result, ErrQueryBudget
	}
	if request.Budget.Timeout <= 0 {
		request.Budget.Timeout = DefaultTimeout
	}
	if request.Budget.MaxScanBytes <= 0 {
		request.Budget.MaxScanBytes = DefaultMaxScanBytes
	}
	if request.Budget.MaxScanBytes > HardMaxScanBytes || request.Budget.Timeout > HardTimeout {
		return result, ErrQueryBudget
	}
	ctx, cancel := context.WithTimeout(ctx, request.Budget.Timeout)
	defer cancel()
	started := time.Now()
	progress := &chstats.Tracker{}
	ctx = progress.Context(ctx, clickhouse.Settings{"max_bytes_to_read": request.Budget.MaxScanBytes, "max_execution_time": max(1, int(request.Budget.Timeout.Seconds())), "max_result_rows": max(request.Limit+1, len(request.SelectedValues))})
	tables, err := query.Router.Tables(request.EnterpriseID)
	if err != nil {
		return result, err
	}
	table, timestamp := tables.Logs, "timestamp"
	switch request.Signal {
	case "metrics":
		table = tables.MetricSeries
	case "logs":
	case "traces":
		table = tables.Traces
		timestamp = "start_time"
	default:
		return result, ErrQueryInvalid
	}
	where := "resource_id IN (?) AND source_key IN (?)"
	args := []any{request.ResourceIDs, request.SourceKeys}
	if request.Signal == "metrics" {
		where += " AND series_id IN (SELECT series_id FROM `" + tables.MetricSamples + "` WHERE resource_id IN (?) AND source_key IN (?) AND timestamp>=? AND timestamp<=?)"
		args = append(args, request.ResourceIDs, request.SourceKeys, request.From, request.To)
		if request.Metric != "" {
			where += " AND metric_name=?"
			args = append(args, request.Metric)
		}
	} else {
		where += " AND " + timestamp + ">=? AND " + timestamp + "<?"
		args = append(args, request.From, request.To)
	}
	for _, filter := range request.Filters {
		predicate, values, e := catalogFilterSQL(request.Signal, filter)
		if e != nil {
			return result, e
		}
		where += " AND " + predicate
		args = append(args, values...)
	}
	if request.Kind == "metrics" {
		if request.Signal != "metrics" {
			return result, ErrQueryInvalid
		}
		if request.Search != "" {
			where += " AND positionCaseInsensitive(metric_name,?)>0"
			args = append(args, request.Search)
		}
		if after != "" {
			where += " AND metric_name>?"
			args = append(args, after)
		}
		rows, e := query.Conn.Query(ctx, "SELECT metric_name, groupUniqArray(concat(metric_type,':',temporality,':',toString(is_monotonic))), any(unit), arraySort(arrayDistinct(arrayFlatten(groupArray(mapKeys(labels))))) FROM `"+table+"` FINAL WHERE "+where+" GROUP BY metric_name ORDER BY metric_name LIMIT ?", append(args, request.Limit+1)...)
		if e != nil {
			return result, e
		}
		defer rows.Close()
		for rows.Next() {
			var item MetricDescriptor
			var types []string
			if e := rows.Scan(&item.Name, &types, &item.Unit, &item.Labels); e != nil {
				return result, e
			}
			if len(result.Metrics) == request.Limit {
				result.Complete = false
				result.HasMore = true
				result.NextCursor = writeCatalogCursor(request, result.Metrics[len(result.Metrics)-1].Name)
				break
			}
			item.Type = metricDescriptorType(types)
			result.Metrics = append(result.Metrics, item)
		}
		if e := rows.Err(); e != nil {
			return result, e
		}
	} else if request.Kind == "fields" {
		if err := query.discoverCatalogFields(ctx, request, table, where, args, after, &result); err != nil {
			return result, err
		}
	} else if request.Kind == "values" {
		field, fieldArgs, e := catalogFieldSQL(request.Signal, request.Field)
		if e != nil {
			return result, e
		}
		base := "SELECT toString(" + field + ") AS value FROM `" + table + "` FINAL WHERE " + where
		baseArgs := append(append([]any{}, fieldArgs...), args...)
		selectSQL := "SELECT DISTINCT value FROM (" + base + ") WHERE value!='' AND length(value)<=4096"
		selectArgs := append([]any{}, baseArgs...)
		if request.Search != "" {
			selectSQL += " AND positionCaseInsensitive(value,?)>0"
			selectArgs = append(selectArgs, request.Search)
		}
		if after != "" {
			selectSQL += " AND value>?"
			selectArgs = append(selectArgs, after)
		}
		rows, e := query.Conn.Query(ctx, selectSQL+" ORDER BY value LIMIT ?", append(selectArgs, request.Limit+1)...)
		if e != nil {
			return result, e
		}
		for rows.Next() {
			var value string
			if e := rows.Scan(&value); e != nil {
				rows.Close()
				return result, e
			}
			if len(result.Values) == request.Limit {
				result.Complete = false
				result.HasMore = true
				result.NextCursor = writeCatalogCursor(request, result.Values[len(result.Values)-1])
				break
			}
			result.Values = append(result.Values, value)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return result, err
		}
		if len(request.SelectedValues) > 0 {
			remainingScan := request.Budget.MaxScanBytes - progress.Bytes()
			if remainingScan <= 0 {
				return result, ErrQueryBudget
			}
			ctx = progress.Context(ctx, clickhouse.Settings{"max_bytes_to_read": remainingScan, "max_execution_time": max(1, int(request.Budget.Timeout.Seconds())), "max_result_rows": len(request.SelectedValues)})
			for _, value := range request.SelectedValues {
				if len(value) > 4096 {
					return result, ErrQueryInvalid
				}
				result.Membership[value] = false
			}
			rows, e = query.Conn.Query(ctx, "SELECT DISTINCT value FROM ("+base+") WHERE value IN (?)", append(baseArgs, request.SelectedValues)...)
			if e != nil {
				return result, e
			}
			for rows.Next() {
				var value string
				if e := rows.Scan(&value); e != nil {
					rows.Close()
					return result, e
				}
				result.Membership[value] = true
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return result, err
			}
		}
	} else {
		return result, ErrQueryInvalid
	}
	result.Meta = queryengine.QueryMeta{Engine: "argus-catalog", EngineVersion: "v1", ScannedBytes: progress.Bytes(), ScannedRows: progress.Rows(), ReturnedRows: int64(len(result.Metrics) + len(result.Values) + len(result.Fields)), ElapsedMillis: time.Since(started).Milliseconds(), Partial: !result.Complete, Warnings: []string{}}
	if result.Meta.ScannedBytes > request.Budget.MaxScanBytes {
		return result, ErrQueryBudget
	}
	encoded, _ := json.Marshal(result)
	if request.Budget.MaxResultBytes > 0 && int64(len(encoded)) > request.Budget.MaxResultBytes || request.Budget.MaxRows > 0 && result.Meta.ReturnedRows+int64(len(result.Membership)) > int64(request.Budget.MaxRows) {
		return result, ErrQueryBudget
	}
	return result, nil
}

// ValidateCatalogDefinition shares field/operator validation with discovery.
func ValidateCatalogDefinition(signal, field string, filters []CatalogFilter) error {
	if _, _, err := catalogFieldSQL(signal, field); err != nil {
		return err
	}
	return ValidateCatalogFilters(signal, filters)
}

func ValidateCatalogFilters(signal string, filters []CatalogFilter) error {
	if signal != "metrics" && signal != "logs" && signal != "traces" || len(filters) > 32 {
		return ErrQueryInvalid
	}
	for _, f := range filters {
		if _, _, err := catalogFilterSQL(signal, f); err != nil {
			return err
		}
	}
	return nil
}

func catalogFilterSQL(signal string, filter CatalogFilter) (string, []any, error) {
	if len(filter.Values) == 0 || len(filter.Values) > 200 {
		return "", nil, ErrQueryInvalid
	}
	for _, value := range filter.Values {
		if len(value) > 4096 {
			return "", nil, ErrQueryInvalid
		}
	}
	field, args, err := catalogFieldSQL(signal, filter.Field)
	if err != nil {
		return "", nil, err
	}
	switch filter.Operator {
	case "", "=", "!=":
		op := " IN (?)"
		if filter.Operator == "!=" {
			op = " NOT IN (?)"
		}
		return "toString(" + field + ")" + op, append(args, filter.Values), nil
	case ">", ">=", "<", "<=":
		if len(filter.Values) != 1 {
			return "", nil, ErrQueryInvalid
		}
		number, e := strconv.ParseFloat(filter.Values[0], 64)
		if e != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return "", nil, ErrQueryInvalid
		}
		return "toFloat64OrNull(toString(" + field + ")) " + filter.Operator + " ?", append(args, number), nil
	default:
		return "", nil, ErrQueryInvalid
	}
}

func catalogFieldSQL(signal, field string) (string, []any, error) {
	if len(field) == 0 || len(field) > 256 {
		return "", nil, ErrQueryInvalid
	}
	if signal == "metrics" {
		if field == "__name__" {
			return "metric_name", nil, nil
		}
		return "labels[?]", []any{field}, nil
	}
	if signal == "logs" {
		if field == "severity_number" {
			return "severity_number", nil, nil
		}
		compiler := kql.Compiler{}
		predicate, err := compiler.Compile(kql.Predicate{Field: field, Op: kql.OpEqual, Value: "catalog", Quoted: true})
		if err != nil {
			return "", nil, ErrQueryInvalid
		}
		return strings.TrimSuffix(predicate, " = ?"), compiler.Args[:len(compiler.Args)-1], nil
	}
	if signal == "traces" {
		switch field {
		case "service_name", "operation", "status", "trace_id", "span_id", "span_kind", "source_id", "resource_id":
			return field, nil, nil
		}
		for prefix, column := range map[string]string{"attributes.": "attributes", "resource_attributes.": "resource_attributes"} {
			if key, ok := strings.CutPrefix(field, prefix); ok && key != "" {
				return column + "[?]", []any{key}, nil
			}
		}
	}
	return "", nil, fmt.Errorf("%w: unsupported catalog field", ErrQueryInvalid)
}

func metricDescriptorType(types []string) string {
	if len(types) != 1 {
		return "mixed"
	}
	parts := strings.Split(types[0], ":")
	if len(parts) != 3 {
		return "unknown"
	}
	if parts[0] == "sum" {
		if strings.HasSuffix(strings.ToLower(parts[1]), "cumulative") && (parts[2] == "true" || parts[2] == "1") {
			return "counter"
		}
		return "gauge"
	}
	return parts[0]
}
