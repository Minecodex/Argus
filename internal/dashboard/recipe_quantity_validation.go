package dashboard

import (
	"slices"

	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/prometheus/prometheus/promql/parser"
)

// Match a versioned known definition, not inferred units of arbitrary PromQL.
func knownRatioRecipe(panel Panel, target TargetExecution, byKey map[string]ResolvedSource) bool {
	var expression string
	for _, definition := range panel.Targets {
		if definition.ID != target.ID {
			continue
		}
		compiled, err := CompileTarget(definition)
		if err != nil || compiled.Deferred {
			return false
		}
		expression = compiled.Query.Expression
	}
	node, err := parser.NewParser(parser.Options{}).ParseExpr(expression)
	if err != nil || len(target.Meta.InputMetrics) == 0 {
		return false
	}
	for _, recipe := range ObservationRecipes() {
		if recipe.Unit != "percent_ratio" || recipe.SourceType != "hostmetrics" {
			continue
		}
		known, err := parser.NewParser(parser.Options{}).ParseExpr(recipe.Expression)
		if err != nil || known.String() != node.String() {
			continue
		}
		valid := true
		for _, input := range target.Meta.InputMetrics {
			source, ok := byKey[input.SourceKey]
			if !ok || source.ResourceType != "host" || source.Type != recipe.SourceType || !slices.Contains(recipe.Metrics, input.Name) {
				valid = false
				break
			}
			metric := telemetry.MetricDescriptor{Name: input.Name, Type: input.Type, Unit: input.Unit, SourceType: source.Type, SourceKeys: []string{input.SourceKey}}
			meaning := telemetry.BuiltinMetricSemanticsForSources(metric, []telemetry.ObservationSource{{SourceType: source.Type, Keys: []string{source.Key()}}})
			if meaning == nil || meaning.QuantityKind != "ratio" {
				valid = false
				break
			}
		}
		if valid {
			return true
		}
	}
	return false
}
