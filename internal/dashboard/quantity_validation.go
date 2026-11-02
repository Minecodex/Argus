package dashboard

import (
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/telemetry/metricmeta"
	"slices"
)

// Validate only complete, original metric metadata bound to resolved sources.
// Arbitrary derived PromQL units and unknown quantities remain unclassified.
func sampleQuantityIssues(spec Spec, sample Execution) []Issue {
	issues := []Issue{}
	for _, definition := range spec.Panels {
		if definition.Signal != "metrics" || !slices.Contains([]string{"timeseries", "stat", "gauge", "bar_gauge", "bar"}, definition.Type) {
			continue
		}
		if definition.Unit != "percent" && definition.Unit != "percent_ratio" && definition.Unit != "%" {
			continue
		}
		for _, panel := range sample.Panels {
			if panel.ID != definition.ID {
				continue
			}
			byKey := map[string]ResolvedSource{}
			for _, source := range panel.Sources {
				byKey[source.Key()] = source
			}
			mismatch := ""
			for _, target := range panel.Targets {
				meta := target.Meta
				if meta.InputMetricsOrigin != metricmeta.Origin || !meta.InputMetricsComplete {
					continue
				}
				if definition.Unit != "percent_ratio" && knownRatioRecipe(definition, target, byKey) {
					mismatch = "This exact published query recipe returns a 0-1 ratio. Use its percent_ratio output unit, or explicitly scale the query by 100 before percent display."
					break
				}
				if !meta.ResultIsOriginalMetric {
					continue
				}
				for _, metric := range meta.InputMetrics {
					source, ok := byKey[metric.SourceKey]
					if !ok {
						continue
					}
					if metric.Unit == "By" && (metric.Type == "gauge" || metric.Type == "counter") {
						mismatch = "A direct raw byte metric cannot use a percentage formatter. Choose the utilization ratio or explicitly derive a dimensionless percentage."
						break
					}
					if source.ResourceType != "host" || definition.Unit == "percent_ratio" {
						continue
					}
					descriptor := telemetry.MetricDescriptor{Name: metric.Name, Type: metric.Type, Unit: metric.Unit, SourceType: source.Type, SourceKeys: []string{metric.SourceKey}}
					meaning := telemetry.BuiltinMetricSemanticsForSources(descriptor, []telemetry.ObservationSource{{SourceType: source.Type, Keys: []string{source.Key()}}})
					if meaning != nil && meaning.QuantityKind == "ratio" {
						mismatch = "This direct utilization metric is a raw 0-1 ratio. Use percent_ratio display, or explicitly multiply the query by 100 before using percent display."
						break
					}
				}
				if mismatch != "" {
					break
				}
			}
			if mismatch != "" {
				issues = append(issues, Issue{Path: "panels." + definition.ID + ".unit", Code: "QUANTITY_DISPLAY_MISMATCH", Message: mismatch})
			}
		}
	}
	return issues
}
