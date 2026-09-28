package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
	"github.com/prometheus/prometheus/promql/parser"
)

type CompiledTarget struct {
	ID         string               `json:"id"`
	Language   queryengine.Language `json:"language"`
	Query      DSL                  `json:"query"`
	Hash       string               `json:"query_hash"`
	ResultType string               `json:"result_type"`
	Deferred   bool                 `json:"deferred"`
}

func languageForSignal(signal string) queryengine.Language {
	switch signal {
	case "metrics":
		return queryengine.LanguagePromQL
	case "logs":
		return queryengine.LanguageKQL
	case "traces":
		return queryengine.LanguageTrace
	}
	return ""
}

func compileConcreteTarget(target Target, checkReferences bool) (CompiledTarget, error) {
	compiled := CompiledTarget{ID: target.ID, Language: target.Language}
	if (target.SourceDefinition.Builder == nil) == (target.SourceDefinition.DSL == nil) {
		return compiled, fmt.Errorf("exactly one query source required")
	}
	if target.SourceDefinition.DSL != nil {
		compiled.Query = *target.SourceDefinition.DSL
	} else {
		query, err := compileBuilder(target.Language, *target.SourceDefinition.Builder)
		if err != nil {
			return compiled, err
		}
		compiled.Query = query
	}
	if len(compiled.Query.Expression) > 65536 || len(compiled.Query.Pipeline) > 65536 {
		return compiled, fmt.Errorf("query exceeds length budget")
	}
	if checkReferences {
		if err := validateQueryReferences(target, compiled.Query); err != nil {
			return compiled, err
		}
	}
	switch target.Language {
	case queryengine.LanguagePromQL:
		if compiled.Query.Pipeline != "" || compiled.Query.Operation != "" || len(compiled.Query.Variables) > 0 {
			return compiled, fmt.Errorf("PromQL accepts only expression")
		}
		expr, err := parser.NewParser(parser.Options{}).ParseExpr(compiled.Query.Expression)
		if err != nil {
			return compiled, err
		}
		compiled.ResultType = string(expr.Type())
		if target.QueryMode != "instant" && target.QueryMode != "range" {
			return compiled, fmt.Errorf("metrics query mode required")
		}
		if target.QueryMode == "range" {
			if expr.Type() != parser.ValueTypeVector && expr.Type() != parser.ValueTypeScalar {
				return compiled, fmt.Errorf("range query requires vector or scalar")
			}
			if _, err := target.RangeStepPolicy.Resolve(time.Unix(0, 0), time.Unix(3600, 0)); err != nil {
				return compiled, fmt.Errorf("range step policy required")
			}
			compiled.ResultType = "matrix"
		}
	case queryengine.LanguageKQL:
		if compiled.Query.Operation != "" || len(compiled.Query.Variables) > 0 {
			return compiled, fmt.Errorf("KQL does not accept GraphQL parameters")
		}
		plan, err := kql.CompileQuery("validation_logs", kql.Request{Expression: compiled.Query.Expression, Pipeline: compiled.Query.Pipeline, Start: time.Unix(0, 0), End: time.Unix(3600, 0), Scope: kql.Scope{ResourceIDs: []uuid.UUID{uuid.Nil}}, Budget: kql.Budget{MaxRows: 100000}})
		if err != nil {
			return compiled, err
		}
		compiled.ResultType = plan.ResultType
	case queryengine.LanguageTrace:
		if compiled.Query.Pipeline != "" {
			return compiled, fmt.Errorf("GraphQL does not accept a pipeline")
		}
		if err := skywalking.ValidateInputs(compiled.Query.Expression, compiled.Query.Operation, compiled.Query.Variables); err != nil {
			return compiled, err
		}
		kind, err := skywalking.ResultKind(compiled.Query.Expression, compiled.Query.Operation)
		if err != nil {
			return compiled, err
		}
		if err := skywalking.ValidateAPMProjection(compiled.Query.Expression, compiled.Query.Operation, kind); err != nil {
			return compiled, err
		}
		compiled.ResultType = kind
	default:
		return compiled, fmt.Errorf("unsupported query language")
	}
	encoded, _ := json.Marshal(struct {
		Target   Target
		Query    DSL
		Compiler string
	}{target, compiled.Query, CompilerVersion})
	digest := sha256.Sum256(encoded)
	compiled.Hash = hex.EncodeToString(digest[:])
	return compiled, nil
}

var metricName = regexp.MustCompile(`^[A-Za-z_:][A-Za-z0-9_:]*$`)
var labelName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func compileBuilder(language queryengine.Language, builder Builder) (DSL, error) {
	if len(builder.ErrorFilters) > 0 && (language != queryengine.LanguagePromQL || builder.Operation != "error_rate") {
		return DSL{}, fmt.Errorf("error filters only apply to metric error rate")
	}
	if (builder.ContextBefore != 0 || builder.ContextAfter != 0) && (language != queryengine.LanguageKQL || builder.Operation != "log_context") {
		return DSL{}, fmt.Errorf("context bounds only apply to log context queries")
	}
	if builder.Limit < 0 || builder.Limit > 50000 || builder.Limit != 0 && !((language == queryengine.LanguageKQL && builder.Operation != "log_context") || language == queryengine.LanguageTrace && (strings.HasPrefix(builder.Operation, "apm_") || builder.Operation == "list")) {
		return DSL{}, fmt.Errorf("explicit builder limit is unsupported or outside budget")
	}
	switch language {
	case queryengine.LanguagePromQL:
		return compileMetricBuilder(builder)
	case queryengine.LanguageKQL:
		parts := []string{}
		contextEvent := ""
		for _, field := range builder.GroupBy {
			if !queryFieldName.MatchString(field) {
				return DSL{}, fmt.Errorf("invalid grouping field")
			}
		}
		for _, filter := range builder.Filters {
			if builder.Operation == "log_context" && filter.Field == "event_id" {
				if contextEvent != "" || filter.Operator != "=" || len(filter.Values) > 1 {
					return DSL{}, fmt.Errorf("context needs one event identity")
				}
				contextEvent = filter.Value
				if len(filter.Values) == 1 {
					contextEvent = filter.Values[0]
				}
				continue
			}
			if filter.Variable != "" || filter.LocalParameter != "" {
				return DSL{}, fmt.Errorf("dynamic filters require parameter bindings")
			}
			if !strings.Contains(" = != > >= < <= : ", " "+filter.Operator+" ") {
				return DSL{}, fmt.Errorf("unsupported log filter operator")
			}
			predicate, err := compileLogFilter(filter)
			if err != nil {
				return DSL{}, err
			}
			parts = append(parts, predicate)
		}
		query := DSL{Expression: "*"}
		if len(parts) > 0 {
			query.Expression = strings.Join(parts, " AND ")
		}
		switch builder.Operation {
		case "log_context":
			if contextEvent == "" || builder.ContextBefore < 0 || builder.ContextAfter < 0 || builder.ContextBefore > 200 || builder.ContextAfter > 200 {
				return DSL{}, fmt.Errorf("invalid log context bounds")
			}
			query.Pipeline = kql.FormatContext(kql.LogContext{EventID: contextEvent, Before: builder.ContextBefore, After: builder.ContextAfter})
		case "records":
		case "count":
			query.Pipeline = "stats count()"
		case "count_by":
			if len(builder.GroupBy) != 1 {
				return DSL{}, fmt.Errorf("one grouping field required")
			}
			query.Pipeline = "stats count() by " + builder.GroupBy[0]
		case "count_over_time":
			if builder.BucketSeconds < 1 || builder.BucketSeconds > 86400 {
				return DSL{}, fmt.Errorf("invalid time bucket")
			}
			query.Pipeline = fmt.Sprintf("stats count() by bin(timestamp, %ds)", builder.BucketSeconds)
			if len(builder.GroupBy) > 1 {
				return DSL{}, fmt.Errorf("at most one grouping field")
			}
			if len(builder.GroupBy) == 1 {
				query.Pipeline += ", " + builder.GroupBy[0]
			}
		default:
			return DSL{}, fmt.Errorf("unsupported log builder operation")
		}
		if builder.Limit > 0 {
			if query.Pipeline != "" {
				query.Pipeline += " | "
			}
			query.Pipeline += fmt.Sprintf("limit %d", builder.Limit)
		}
		return query, nil
	case queryengine.LanguageTrace:
		if strings.HasPrefix(builder.Operation, "apm_") {
			return compileAPMBuilder(builder)
		}
		return compileTraceBuilder(builder)
	default:
		return DSL{}, fmt.Errorf("unsupported builder language")
	}
}

func compileMetricBuilder(b Builder) (DSL, error) {
	if !metricName.MatchString(b.Metric) {
		return DSL{}, fmt.Errorf("invalid metric name")
	}
	labels := []string{}
	for _, f := range b.Filters {
		if !labelName.MatchString(f.Field) || f.Variable != "" || f.LocalParameter != "" || f.DrilldownInput != "" {
			return DSL{}, fmt.Errorf("invalid metric filter or missing parameter binding")
		}
		if f.Operator != "=" && f.Operator != "!=" && f.Operator != "=~" && f.Operator != "!~" {
			return DSL{}, fmt.Errorf("invalid label operator")
		}
		matcher, err := compileMetricFilter(f)
		if err != nil {
			return DSL{}, err
		}
		labels = append(labels, matcher)
	}
	for _, label := range b.GroupBy {
		if !labelName.MatchString(label) {
			return DSL{}, fmt.Errorf("invalid grouping label")
		}
	}
	selector := b.Metric + "{" + strings.Join(labels, ",") + "}"
	group := ""
	if len(b.GroupBy) > 0 {
		group = " by (" + strings.Join(b.GroupBy, ",") + ")"
	}
	expression := selector
	switch b.Operation {
	case "value":
	case "sum", "avg", "min", "max":
		expression = b.Operation + group + " (" + selector + ")"
	case "rate":
		if b.MetricType != "counter" || b.WindowSeconds < 1 || b.WindowSeconds > 86400 {
			return DSL{}, fmt.Errorf("rate requires a counter and valid window")
		}
		expression = fmt.Sprintf("rate(%s[%ds])", selector, b.WindowSeconds)
	case "error_rate":
		if b.MetricType != "counter" || b.WindowSeconds < 1 || b.WindowSeconds > 86400 || len(b.ErrorFilters) == 0 || len(b.ErrorFilters) > 32 {
			return DSL{}, fmt.Errorf("error rate requires a counter, valid window and error subset")
		}
		errorLabels := append([]string{}, labels...)
		for _, f := range b.ErrorFilters {
			// Error classification is fixed in the authored definition. Common
			// dynamic filters still constrain both sides through Filters.
			if !labelName.MatchString(f.Field) || f.Field == "__name__" || f.Variable != "" || f.LocalParameter != "" || f.DrilldownInput != "" || len(f.Values) > 200 || len(f.Value) > 4096 || f.Value != "" && len(f.Values) > 0 {
				return DSL{}, fmt.Errorf("error subset requires literal label filters")
			}
			for _, value := range f.Values {
				if len(value) > 4096 {
					return DSL{}, fmt.Errorf("error filter value exceeds budget")
				}
			}
			matcher, err := compileMetricFilter(f)
			if err != nil {
				return DSL{}, err
			}
			errorLabels = append(errorLabels, matcher)
		}
		errors := b.Metric + "{" + strings.Join(errorLabels, ",") + "}"
		total := fmt.Sprintf("sum%s (rate(%s[%ds]))", group, selector, b.WindowSeconds)
		failed := fmt.Sprintf("sum%s (rate(%s[%ds]))", group, errors, b.WindowSeconds)
		// A missing error series is zero only for an observed denominator.
		// No requests, absent counters or too few samples remain no data.
		incomplete := fmt.Sprintf("sum%s (count_over_time(%s[%ds]) < 2)", group, errors, b.WindowSeconds)
		expression = fmt.Sprintf("(((%s or (%s * 0)) / (%s > 0)) unless %s)", failed, total, total, incomplete)
	case "topk":
		if b.TopN < 1 || b.TopN > 100 {
			return DSL{}, fmt.Errorf("Top N outside budget")
		}
		expression = fmt.Sprintf("topk(%d, %s)", b.TopN, selector)
	case "p95":
		if b.MetricType != "histogram" || b.WindowSeconds < 1 || b.WindowSeconds > 86400 {
			return DSL{}, fmt.Errorf("P95 requires a histogram and valid window")
		}
		if !strings.HasSuffix(b.Metric, "_bucket") {
			selector = strings.Replace(selector, b.Metric, b.Metric+"_bucket", 1)
		}
		groups := append([]string{"le"}, b.GroupBy...)
		expression = fmt.Sprintf("histogram_quantile(0.95, sum by (%s) (rate(%s[%ds])))", strings.Join(groups, ","), selector, b.WindowSeconds)
	default:
		return DSL{}, fmt.Errorf("unsupported metric builder operation")
	}
	return DSL{Expression: expression}, nil
}
