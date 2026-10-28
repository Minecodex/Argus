package dashboard

import (
	"slices"
	"sync"

	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

// Recipes are derived from the versioned, validated built-in definitions; no
// observation is inferred from their presence. Unknown descriptions stay empty.
var ObservationRecipes = sync.OnceValue(func() []telemetry.ObservationRecipe {
	result := []telemetry.ObservationRecipe{}
	for _, definition := range BuiltinTemplates() {
		for _, panel := range definition.Spec.Panels {
			for _, target := range panel.Targets {
				if panel.Signal != "metrics" || target.SourceDefinition.DSL == nil {
					continue
				}
				expression := target.SourceDefinition.DSL.Expression
				node, err := parser.NewParser(parser.Options{}).ParseExpr(expression)
				if err != nil {
					continue
				}
				names := []string{}
				for _, selector := range parser.ExtractSelectors(node) {
					for _, matcher := range selector {
						if matcher.Name == labels.MetricName && matcher.Type == labels.MatchEqual && !slices.Contains(names, matcher.Value) {
							names = append(names, matcher.Value)
						}
					}
				}
				for _, integration := range definition.IntegrationKeys {
					result = append(result, telemetry.ObservationRecipe{ID: definition.Key + "/" + panel.ID + "/" + target.ID, Version: definition.Version, Title: panel.Title, Description: panel.Description, Expression: expression, Unit: panel.Unit, SourceType: panel.SourceBinding.SourceType, IntegrationKey: integration, Metrics: names})
				}
			}
		}
	}
	return result
})
