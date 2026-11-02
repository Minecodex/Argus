package dashboard

import (
	"slices"

	"github.com/kakj-go/Argus/internal/telemetry"
)

// Authoring and observation use the same collection definition and published
// query recipes. An engine response cannot self-certify its quantity hints.
func enrichAuthoringMetricGuidance(data *telemetry.DataCatalogResult, sources []ResolvedSource) {
	authorized := []telemetry.ObservationSource{}
	for _, source := range sources {
		if source.ResourceType == "host" {
			authorized = append(authorized, telemetry.ObservationSource{SourceType: source.Type, Keys: []string{source.Key()}})
		}
	}
	for i := range data.Metrics {
		metric := &data.Metrics[i]
		metric.Semantics, metric.Recipes = nil, nil
		metric.Semantics = telemetry.BuiltinMetricSemanticsForSources(*metric, authorized)
		if metric.Semantics == nil {
			continue
		}
		for _, recipe := range ObservationRecipes() {
			if recipe.SourceType == metric.SourceType && slices.Contains(recipe.Metrics, metric.Name) {
				metric.Recipes = append(metric.Recipes, recipe)
			}
		}
	}
}
