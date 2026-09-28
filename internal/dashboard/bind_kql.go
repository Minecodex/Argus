package dashboard

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
)

type boundPredicate struct {
	text string
	all  bool
}

func bindKQLPredicate(expr kql.Expr, parameters map[string]Selection, conjunction bool) (boundPredicate, error) {
	switch node := expr.(type) {
	case kql.Predicate:
		name := parameterName(node.Value)
		if name == "" {
			return boundPredicate{text: formatKQLPredicate(node)}, nil
		}
		selection, ok := parameters[name]
		if !ok {
			return boundPredicate{}, fmt.Errorf("%w: missing query parameter %s", ErrInvalid, name)
		}
		if selection.All {
			if !conjunction {
				return boundPredicate{}, fmt.Errorf("%w: All cannot remove a parameter inside OR or NOT", ErrInvalid)
			}
			return boundPredicate{all: true}, nil
		}
		if len(selection.Values) == 0 {
			return boundPredicate{}, ErrInvalid
		}
		if len(selection.Values) > 1 && node.Op != kql.OpEqual && node.Op != kql.OpNotEqual && node.Op != kql.OpContains {
			return boundPredicate{}, fmt.Errorf("%w: comparison requires one value", ErrInvalid)
		}
		parts := []string{}
		for _, value := range selection.Values {
			bound := node
			bound.Value = value
			bound.Quoted = true
			if node.Field == "severity_number" || node.Op == kql.OpGT || node.Op == kql.OpGTE || node.Op == kql.OpLT || node.Op == kql.OpLTE {
				n, e := strconv.ParseFloat(value, 64)
				if e != nil || math.IsNaN(n) || math.IsInf(n, 0) {
					return boundPredicate{}, fmt.Errorf("%w: numeric parameter required", ErrInvalid)
				}
				bound.Quoted = false
			} else if !node.Quoted {
				if n, e := strconv.ParseFloat(value, 64); e == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
					bound.Quoted = false
				}
				if value == "true" || value == "false" || value == "null" {
					bound.Quoted = false
				}
			}
			parts = append(parts, formatKQLPredicate(bound))
		}
		join := " OR "
		if node.Op == kql.OpNotEqual {
			join = " AND "
		}
		return boundPredicate{text: "(" + strings.Join(parts, join) + ")"}, nil
	case kql.BoolExpr:
		allowed := conjunction && node.Op == "AND"
		left, err := bindKQLPredicate(node.Left, parameters, allowed)
		if err != nil {
			return left, err
		}
		right, err := bindKQLPredicate(node.Right, parameters, allowed)
		if err != nil {
			return right, err
		}
		if left.all {
			return right, nil
		}
		if right.all {
			return left, nil
		}
		return boundPredicate{text: "(" + left.text + " " + node.Op + " " + right.text + ")"}, nil
	case kql.NotExpr:
		inner, err := bindKQLPredicate(node.Inner, parameters, false)
		if err != nil {
			return inner, err
		}
		return boundPredicate{text: "NOT (" + inner.text + ")"}, nil
	default:
		return boundPredicate{}, ErrInvalid
	}
}

func formatKQLPredicate(node kql.Predicate) string {
	if node.Op == kql.OpExists {
		return node.Field + " exists"
	}
	value := node.Value
	if node.Quoted {
		value = strconv.Quote(value)
	}
	return node.Field + " " + string(node.Op) + " " + value
}

func bindKQL(query DSL, parameters map[string]Selection) (DSL, error) {
	stages, err := kql.SplitStages(query.Expression)
	if err != nil {
		return query, err
	}
	if query.Pipeline != "" {
		if len(stages) > 1 {
			return query, ErrInvalid
		}
		more, e := kql.SplitStages(query.Pipeline)
		if e != nil {
			return query, e
		}
		stages = append(stages, more...)
	}
	output := []string{}
	for index, stage := range stages {
		if index > 0 && strings.HasPrefix(strings.ToLower(stage), "context ") {
			value, e := kql.ParseContext(stage)
			if e != nil {
				return query, e
			}
			if name := parameterName(value.EventID); name != "" {
				selection, ok := parameters[name]
				if !ok || selection.All || len(selection.Values) != 1 {
					return query, ErrInvalid
				}
				value.EventID = selection.Values[0]
			}
			output = append(output, kql.FormatContext(value))
			continue
		}
		filter := stage
		if index > 0 {
			if !strings.HasPrefix(strings.ToLower(stage), "where ") {
				output = append(output, stage)
				continue
			}
			filter = strings.TrimSpace(stage[6:])
		}
		if index == 0 && filter == "*" {
			output = append(output, "*")
			continue
		}
		expr, e := kql.Parse(filter)
		if e != nil {
			return query, e
		}
		bound, e := bindKQLPredicate(expr, parameters, true)
		if e != nil {
			return query, e
		}
		if index == 0 {
			if bound.all {
				output = append(output, "*")
			} else {
				output = append(output, bound.text)
			}
		} else if !bound.all {
			output = append(output, "where "+bound.text)
		}
	}
	query.Expression = output[0]
	query.Pipeline = strings.Join(output[1:], " | ")
	return query, nil
}
