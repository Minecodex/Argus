package dashboard

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/prometheus/prometheus/promql/parser"
)

// Recognize only the guarded, same-counter ratio emitted by the builder. The
// caller recompiles and proves full AST equivalence, including vector matching,
// the zero fallback and denominator guard; arbitrary divisions remain DSL.
func errorRateBuilderFromDSL(target Target, division *parser.BinaryExpr) (Builder, error) {
	invalid := fmt.Errorf("ratio is outside the controlled error-rate builder")
	union, ok := unparen(division.LHS).(*parser.BinaryExpr)
	if !ok || union.Op != parser.LOR {
		return Builder{}, invalid
	}
	guard, ok := unparen(division.RHS).(*parser.BinaryExpr)
	if !ok || guard.Op != parser.GTR {
		return Builder{}, invalid
	}
	failed, ok := unparen(union.LHS).(*parser.AggregateExpr)
	if !ok || failed.Op != parser.SUM || failed.Without {
		return Builder{}, invalid
	}
	total, ok := unparen(guard.LHS).(*parser.AggregateExpr)
	if !ok || total.Op != parser.SUM || total.Without || !slices.Equal(total.Grouping, failed.Grouping) {
		return Builder{}, invalid
	}
	readRate := func(expr parser.Expr) (Builder, error) {
		copy := target
		copy.SourceDefinition = Definition{DSL: &DSL{Expression: expr.String()}}
		b, err := metricBuilderFromDSL(copy)
		if err != nil || b.Operation != "rate" {
			return Builder{}, invalid
		}
		return b, nil
	}
	denominator, err := readRate(total.Expr)
	if err != nil {
		return Builder{}, err
	}
	numerator, err := readRate(failed.Expr)
	if err != nil || denominator.Metric != numerator.Metric || denominator.WindowSeconds != numerator.WindowSeconds {
		return Builder{}, invalid
	}
	errors := slices.Clone(numerator.Filters)
	for _, common := range denominator.Filters {
		i := slices.IndexFunc(errors, func(f Filter) bool { return reflect.DeepEqual(f, common) })
		if i < 0 {
			return Builder{}, invalid
		}
		errors = slices.Delete(errors, i, i+1)
	}
	denominator.Operation, denominator.GroupBy, denominator.ErrorFilters = "error_rate", slices.Clone(total.Grouping), errors
	if _, err := compileMetricBuilder(denominator); err != nil {
		// Common bindings are reintroduced as template placeholders later.
		probe := denominator
		probe.Filters = nil
		if _, err := compileMetricBuilder(probe); err != nil {
			return Builder{}, invalid
		}
	}
	return denominator, nil
}
