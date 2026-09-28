package dashboard

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

func metricBuilderFromDSL(target Target) (Builder, error) {
	b := Builder{Operation: "value", Filters: []Filter{}, GroupBy: []string{}}
	expr, err := parser.NewParser(parser.Options{}).ParseExpr(target.SourceDefinition.DSL.Expression)
	if err != nil {
		return b, err
	}
	expr = unparen(expr)
	if guard, ok := expr.(*parser.BinaryExpr); ok && guard.Op == parser.LUNLESS {
		if division, ok := unparen(guard.LHS).(*parser.BinaryExpr); ok && division.Op == parser.DIV {
			return errorRateBuilderFromDSL(target, division)
		}
	}
	if call, ok := expr.(*parser.Call); ok && call.Func.Name == "histogram_quantile" {
		if len(call.Args) != 2 {
			return b, fmt.Errorf("unsupported quantile")
		}
		quantile, ok := unparen(call.Args[0]).(*parser.NumberLiteral)
		if !ok || quantile.Val != 0.95 {
			return b, fmt.Errorf("only P95 is supported")
		}
		sum, ok := unparen(call.Args[1]).(*parser.AggregateExpr)
		if !ok || sum.Op != parser.SUM || sum.Without || len(sum.Grouping) == 0 || sum.Grouping[0] != "le" {
			return b, fmt.Errorf("P95 needs cumulative bucket grouping")
		}
		b.Operation, b.MetricType, b.GroupBy = "p95", "histogram", slices.Clone(sum.Grouping[1:])
		expr = unparen(sum.Expr)
	} else if agg, ok := expr.(*parser.AggregateExpr); ok {
		if agg.Without {
			return b, fmt.Errorf("without grouping is not supported")
		}
		switch agg.Op {
		case parser.SUM, parser.AVG, parser.MIN, parser.MAX:
			b.Operation = agg.Op.String()
			b.GroupBy = slices.Clone(agg.Grouping)
		case parser.TOPK:
			count, ok := unparen(agg.Param).(*parser.NumberLiteral)
			if !ok || math.Trunc(count.Val) != count.Val || count.Val < 1 || count.Val > 100 || len(agg.Grouping) > 0 {
				return b, fmt.Errorf("unsupported Top N")
			}
			b.Operation = "topk"
			b.TopN = int(count.Val)
		default:
			return b, fmt.Errorf("unsupported aggregation")
		}
		expr = unparen(agg.Expr)
	}
	if call, ok := expr.(*parser.Call); ok {
		if call.Func.Name != "rate" || len(call.Args) != 1 || b.Operation != "value" && b.Operation != "p95" {
			return b, fmt.Errorf("unsupported function composition")
		}
		matrix, ok := unparen(call.Args[0]).(*parser.MatrixSelector)
		if !ok || matrix.RangeExpr != nil || matrix.Range < time.Second || matrix.Range > 24*time.Hour || matrix.Range%time.Second != 0 {
			return b, fmt.Errorf("range cannot be represented in seconds")
		}
		b.WindowSeconds = int(matrix.Range / time.Second)
		if b.Operation == "value" {
			b.Operation, b.MetricType = "rate", "counter"
		}
		expr = matrix.VectorSelector
	} else if b.Operation == "p95" {
		return b, fmt.Errorf("P95 needs bucket rate")
	}
	selector, ok := unparen(expr).(*parser.VectorSelector)
	if !ok || selector.Name == "" || selector.OriginalOffset != 0 || selector.OriginalOffsetExpr != nil || selector.Timestamp != nil || selector.StartOrEnd != 0 || selector.Anchored || selector.Smoothed {
		return b, fmt.Errorf("selector contains unsupported modifiers")
	}
	b.Metric = selector.Name
	for _, matcher := range selector.LabelMatchers {
		if matcher.Name == labels.MetricName && matcher.Type == labels.MatchEqual && matcher.Value == selector.Name {
			continue
		}
		f := Filter{Field: matcher.Name, Operator: matcher.Type.String(), Value: matcher.Value}
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
func unparen(expr parser.Expr) parser.Expr {
	for {
		p, ok := expr.(*parser.ParenExpr)
		if !ok {
			return expr
		}
		expr = p.Expr
	}
}
