package dashboard

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/telemetry"
)

func TestAuthoringCatalogGuidanceUsesCanonicalQuantityAndRecipes(t *testing.T) {
	source := ResolvedSource{ID: uuid.New(), Revision: 3, ResourceType: "host", Type: "hostmetrics"}
	for _, name := range []string{"system_cpu_utilization", "system_memory_utilization"} {
		data := telemetry.DataCatalogResult{Metrics: []telemetry.MetricDescriptor{{Name: name, Type: "gauge", Unit: "1", SourceType: "hostmetrics", SourceKeys: []string{source.Key()}}}}
		enrichAuthoringMetricGuidance(&data, []ResolvedSource{source})
		metric := data.Metrics[0]
		meaning := builtincollection.InputMetricSemantics("hostmetrics", name, "gauge", "1")
		if metric.Semantics == nil || meaning == nil || *metric.Semantics != *meaning || len(metric.Recipes) == 0 {
			t.Fatal("authoring lost shared source-qualified quantity/recipe", metric)
		}
		for _, recipe := range metric.Recipes {
			if recipe.SourceType != "hostmetrics" || recipe.Unit != "percent_ratio" || recipe.Expression == "" {
				t.Fatal("recipe does not preserve its published expression/unit", recipe)
			}
		}
		if metric.Unit != "1" || metric.Type != "gauge" || metric.SourceKeys[0] != source.Key() {
			t.Fatal("quantity guidance rewrote observed metadata")
		}
	}
	for _, change := range []string{"unknown", "bytes", "counter", "foreign_key", "mixed_keys", "other_source", "non_host"} {
		metric := telemetry.MetricDescriptor{Name: "system_memory_utilization", Type: "gauge", Unit: "1", SourceType: "hostmetrics", SourceKeys: []string{source.Key()}, Semantics: &builtincollection.MetricSemantics{QuantityKind: "forged"}, Recipes: []telemetry.ObservationRecipe{{ID: "forged"}}}
		current := source
		switch change {
		case "unknown":
			metric.Name = "status_code"
		case "bytes":
			metric.Unit = "By"
		case "counter":
			metric.Type = "counter"
		case "foreign_key":
			metric.SourceKeys = []string{uuid.NewString() + ":3"}
		case "mixed_keys":
			metric.SourceKeys = append(metric.SourceKeys, uuid.NewString()+":3")
		case "other_source":
			metric.SourceType = "otlp"
		case "non_host":
			current.ResourceType = "application"
		}
		data := telemetry.DataCatalogResult{Metrics: []telemetry.MetricDescriptor{metric}}
		enrichAuthoringMetricGuidance(&data, []ResolvedSource{current})
		if data.Metrics[0].Semantics != nil || len(data.Metrics[0].Recipes) != 0 {
			t.Fatal("unknown/foreign/conflicting data self-certified authoring hints", change, data.Metrics[0])
		}
	}
}
