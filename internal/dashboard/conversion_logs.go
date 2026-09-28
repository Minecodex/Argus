package dashboard

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
)

func logBuilderTemplate(builder Builder, names map[int]string) (DSL, error) {
	probe := builder
	probe.Filters = nil
	parts := []string{}
	for i, f := range builder.Filters {
		name := names[i]
		if builder.Operation == "log_context" && f.Field == "event_id" {
			if name != "" {
				f = Filter{Field: f.Field, Operator: f.Operator, Value: "$" + name}
			}
			probe.Filters = append(probe.Filters, f)
			continue
		}
		var text string
		var err error
		if name != "" {
			text = formatKQLPredicate(kql.Predicate{Field: f.Field, Op: kql.Op(f.Operator), Value: "$" + name, Quoted: true})
		} else {
			text, err = compileLogFilter(f)
		}
		if err != nil {
			return DSL{}, err
		}
		parts = append(parts, text)
	}
	query, err := compileBuilder(queryengine.LanguageKQL, probe)
	if err != nil {
		return query, err
	}
	if len(parts) > 0 {
		query.Expression = strings.Join(parts, " AND ")
	}
	return query, nil
}
func logDocument(query DSL) (kql.Expr, kql.Pipeline, error) {
	var expr kql.Expr
	var pipe kql.Pipeline
	stages, err := kql.SplitStages(query.Expression)
	if err != nil {
		return expr, pipe, err
	}
	if query.Pipeline != "" {
		if len(stages) > 1 {
			return expr, pipe, fmt.Errorf("pipeline supplied twice")
		}
		extra, e := kql.SplitStages(query.Pipeline)
		if e != nil {
			return expr, pipe, e
		}
		stages = append(stages, extra...)
	}
	if stages[0] != "*" {
		expr, err = kql.Parse(stages[0])
		if err != nil {
			return expr, pipe, err
		}
	}
	pipe, err = kql.ParsePipeline(strings.Join(stages[1:], " | "), 100000)
	return expr, pipe, err
}
func logBuilderFromDSL(target Target) (Builder, error) {
	b := Builder{Operation: "records", Filters: []Filter{}, GroupBy: []string{}}
	expr, pipe, err := logDocument(*target.SourceDefinition.DSL)
	if err != nil {
		return b, err
	}
	if pipe.Parser != "" || pipe.Unwrap != "" {
		return b, fmt.Errorf("parse and unwrap require statement mode")
	}
	if expr != nil {
		b.Filters, err = logFilters(expr, target.ParameterBindings)
		if err != nil {
			return b, err
		}
	}
	for _, expr := range pipe.Filters {
		filters, e := logFilters(expr, target.ParameterBindings)
		if e != nil {
			return b, e
		}
		b.Filters = append(b.Filters, filters...)
	}
	if pipe.Aggregate != nil {
		a := pipe.Aggregate
		if a.Function != "count" {
			return b, fmt.Errorf("aggregation requires statement mode")
		}
		b.Operation = "count"
		if a.Group != "" {
			b.Operation = "count_by"
			b.GroupBy = []string{a.Group}
		}
		if a.Bucket > 0 {
			if a.Bucket%time.Second != 0 {
				return b, fmt.Errorf("bucket cannot be represented in seconds")
			}
			b.Operation = "count_over_time"
			b.BucketSeconds = int(a.Bucket / time.Second)
		}
	}
	if pipe.ExplicitLimit && pipe.Context == nil {
		b.Limit = pipe.Limit
	}
	if pipe.Context != nil {
		b.Operation = "log_context"
		b.ContextBefore, b.ContextAfter = pipe.Context.Before, pipe.Context.After
		f := Filter{Field: "event_id", Operator: "=", Value: pipe.Context.EventID}
		if name := parameterName(f.Value); name != "" {
			f, err = restoreFilterParameter(f, name, target.ParameterBindings)
			if err != nil {
				return b, err
			}
		}
		b.Filters = append(b.Filters, f)
	}
	return b, nil
}
func logFilters(expr kql.Expr, bindings []ParameterBinding) ([]Filter, error) {
	switch node := expr.(type) {
	case kql.Predicate:
		if node.Op == kql.OpExists {
			return nil, fmt.Errorf("exists requires statement mode")
		}
		f := Filter{Field: node.Field, Operator: string(node.Op), Value: node.Value}
		if name := parameterName(node.Value); name != "" {
			var err error
			f, err = restoreFilterParameter(f, name, bindings)
			if err != nil {
				return nil, err
			}
		}
		return []Filter{f}, nil
	case kql.BoolExpr:
		left, err := logFilters(node.Left, bindings)
		if err != nil {
			return nil, err
		}
		right, err := logFilters(node.Right, bindings)
		if err != nil {
			return nil, err
		}
		if node.Op == "AND" {
			return append(left, right...), nil
		}
		values := append(left, right...)
		first := values[0]
		if first.Operator != "=" && first.Operator != ":" {
			return nil, fmt.Errorf("OR cannot be represented by this builder")
		}
		first.Values = nil
		for _, f := range values {
			if f.Field != first.Field || f.Operator != first.Operator || f.Variable != "" || f.LocalParameter != "" || f.DrilldownInput != "" {
				return nil, fmt.Errorf("OR spans different conditions")
			}
			if len(f.Values) > 0 {
				first.Values = append(first.Values, f.Values...)
			} else {
				first.Values = append(first.Values, f.Value)
			}
		}
		first.Value = ""
		return []Filter{first}, nil
	default:
		return nil, fmt.Errorf("boolean expression requires statement mode")
	}
}
func normalizedLogFilter(expr kql.Expr) any {
	if expr == nil {
		return nil
	}
	if predicate, ok := expr.(kql.Predicate); ok {
		if name := parameterName(predicate.Value); name != "" {
			predicate.Value = "$" + name
		}
		return predicate
	}
	if node, ok := expr.(kql.BoolExpr); ok {
		parts := []any{}
		var flatten func(kql.Expr)
		flatten = func(e kql.Expr) {
			if n, ok := e.(kql.BoolExpr); ok && n.Op == node.Op {
				flatten(n.Left)
				flatten(n.Right)
			} else {
				parts = append(parts, normalizedLogFilter(e))
			}
		}
		flatten(node.Left)
		flatten(node.Right)
		return struct {
			Op    string
			Parts []any
		}{node.Op, parts}
	}
	return expr
}
func equivalentLogQuery(a, b DSL) (bool, error) {
	ae, ap, err := logDocument(a)
	if err != nil {
		return false, err
	}
	be, bp, err := logDocument(b)
	if err != nil {
		return false, err
	}
	// where stages can be combined only before aggregation; ParsePipeline has already proved their order.
	for _, filter := range ap.Filters {
		if ae == nil {
			ae = filter
		} else {
			ae = kql.BoolExpr{Left: ae, Op: "AND", Right: filter}
		}
	}
	ap.Filters = nil
	for _, filter := range bp.Filters {
		if be == nil {
			be = filter
		} else {
			be = kql.BoolExpr{Left: be, Op: "AND", Right: filter}
		}
	}
	bp.Filters = nil
	for _, pipe := range []*kql.Pipeline{&ap, &bp} {
		if pipe.Context != nil {
			if name := parameterName(pipe.Context.EventID); name != "" {
				pipe.Context.EventID = "$" + name
			}
		}
	}
	return reflect.DeepEqual(normalizedLogFilter(ae), normalizedLogFilter(be)) && reflect.DeepEqual(ap, bp), nil
}
