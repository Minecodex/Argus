package dashboard

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/kql"
	"github.com/prometheus/prometheus/promql/parser"
	"slices"
)

func analysisTargetBindings(target Target) []ParameterBinding {
	if _, err := CompileTarget(target); err == nil {
		if templated, err := targetToDSL(target); err == nil {
			return templated.ParameterBindings
		}
	}
	bindings := slices.Clone(target.ParameterBindings)
	if builder := target.SourceDefinition.Builder; builder != nil {
		for _, f := range append(slices.Clone(builder.Filters), builder.ErrorFilters...) {
			if f.Variable != "" || f.LocalParameter != "" {
				bindings = append(bindings, ParameterBinding{Parameter: f.Field, Variable: f.Variable, LocalParameter: f.LocalParameter})
			}
		}
	}
	return bindings
}

// Bindings alone are not a semantic contract: hand-written DSL can move the
// same parameter to another field without renaming its binding. Fingerprint the
// normalized query too; unsupported normalization falls back conservatively.
func analysisQueryContract(target Target) string {
	fallback := func() string {
		raw, _ := json.Marshal([]any{target.Language, target.SourceDefinition})
		return queryHash(raw)
	}
	_, err := CompileTarget(target)
	if err != nil {
		return fallback()
	}
	templated, err := targetToDSL(target)
	if err != nil || templated.SourceDefinition.DSL == nil {
		return fallback()
	}
	query := *templated.SourceDefinition.DSL
	var meaning any
	switch target.Language {
	case queryengine.LanguagePromQL:
		expr, err := parser.NewParser(parser.Options{}).ParseExpr(query.Expression)
		if err != nil {
			return fallback()
		}
		parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
			if selector, ok := node.(*parser.VectorSelector); ok {
				for _, matcher := range selector.LabelMatchers {
					if name := parameterName(matcher.Value); name != "" {
						matcher.Value = "$" + name
					}
				}
			}
			return nil
		})
		meaning = unparen(expr).String()
	case queryengine.LanguageKQL:
		expression, pipeline, err := logDocument(query)
		if err != nil {
			return fallback()
		}
		for _, filter := range pipeline.Filters {
			if expression == nil {
				expression = filter
			} else {
				expression = kql.BoolExpr{Left: expression, Op: "AND", Right: filter}
			}
		}
		pipeline.Filters = nil
		if pipeline.Context != nil {
			if name := parameterName(pipeline.Context.EventID); name != "" {
				pipeline.Context.EventID = "$" + name
			}
		}
		meaning = []any{normalizedLogFilter(expression), pipeline}
	case queryengine.LanguageTrace:
		canonical, err := canonicalGraphQuery(query)
		if err != nil {
			return fallback()
		}
		meaning = []any{canonical, query.Operation, query.Variables}
	default:
		return fallback()
	}
	raw, _ := json.Marshal([]any{target.Language, meaning})
	return queryHash(raw)
}
