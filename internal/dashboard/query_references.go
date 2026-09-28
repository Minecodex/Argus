package dashboard

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"github.com/prometheus/prometheus/promql/parser"
)

var queryParameter = regexp.MustCompile(`^\$(?:\{([A-Za-z_][A-Za-z0-9_]*)\}|([A-Za-z_][A-Za-z0-9_]*))$`)

func validateQueryReferences(target Target, query DSL) error {
	var names []string
	inspect := func(value string) {
		parts := queryParameter.FindStringSubmatch(value)
		if parts == nil {
			return
		}
		name := parts[1]
		if name == "" {
			name = parts[2]
		}
		names = append(names, name)
	}
	switch target.Language {
	case queryengine.LanguagePromQL:
		expr, err := parser.NewParser(parser.Options{}).ParseExpr(query.Expression)
		if err != nil {
			return err
		}
		for _, selectors := range parser.ExtractSelectors(expr) {
			for _, matcher := range selectors {
				inspect(matcher.Value)
			}
		}
	case queryengine.LanguageKQL:
		stages, err := kql.SplitStages(query.Expression)
		if err != nil {
			return err
		}
		if query.Pipeline != "" {
			more, e := kql.SplitStages(query.Pipeline)
			if e != nil {
				return e
			}
			stages = append(stages, more...)
		}
		var walk func(kql.Expr)
		walk = func(expr kql.Expr) {
			switch node := expr.(type) {
			case kql.Predicate:
				inspect(node.Value)
			case kql.BoolExpr:
				walk(node.Left)
				walk(node.Right)
			case kql.NotExpr:
				walk(node.Inner)
			}
		}
		for i, stage := range stages {
			if i > 0 && strings.HasPrefix(strings.ToLower(stage), "context ") {
				value, e := kql.ParseContext(stage)
				if e != nil {
					return e
				}
				inspect(value.EventID)
				continue
			}
			if i == 0 && stage != "*" {
				expr, e := kql.Parse(stage)
				if e != nil {
					return e
				}
				walk(expr)
			} else if strings.HasPrefix(strings.ToLower(stage), "where ") {
				expr, e := kql.Parse(strings.TrimSpace(stage[6:]))
				if e != nil {
					return e
				}
				walk(expr)
			}
		}
	}
	for _, name := range names {
		if !slices.ContainsFunc(target.ParameterBindings, func(binding ParameterBinding) bool { return binding.Parameter == name }) {
			return fmt.Errorf("undefined query parameter %q", name)
		}
	}
	if target.Language != queryengine.LanguageTrace && target.SourceDefinition.DSL != nil {
		for _, binding := range target.ParameterBindings {
			if !slices.Contains(names, binding.Parameter) {
				return fmt.Errorf("unused query parameter %q", binding.Parameter)
			}
		}
	}
	return nil
}
