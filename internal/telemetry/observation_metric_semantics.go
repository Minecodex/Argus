package telemetry

import (
	"slices"

	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/observationscope"
	"github.com/kakj-go/Argus/internal/telemetry/metricmeta"
	"github.com/kakj-go/Argus/internal/telemetry/sourcekey"
)

type QueryInputMetricSemantics struct {
	Name      string                            `json:"name"`
	SourceKey string                            `json:"source_key"`
	Unit      string                            `json:"unit"`
	Type      string                            `json:"type"`
	Semantics builtincollection.MetricSemantics `json:"semantics"`
}

// Resolve input definitions against the exact already-authorized source row.
// Similar names on SDK/custom/unknown sources cannot inherit host semantics.
func queryInputMetricSemantics(inputs []metricmeta.Descriptor, snapshot observationscope.Snapshot) []QueryInputMetricSemantics {
	result := []QueryInputMetricSemantics{}
	if snapshot.Query == nil {
		return result
	}
	for _, input := range inputs {
		if !slices.Contains(snapshot.Query.Keys, input.SourceKey) {
			continue
		}
		id, _, _, valid := sourcekey.Parse(input.SourceKey)
		if !valid {
			continue
		}
		for _, row := range snapshot.Query.Rows {
			if row.ID != id || row.ResourceType != "host" {
				continue
			}
			meaning := builtincollection.InputMetricSemantics(row.SourceType, input.Name, input.Type, input.Unit)
			if meaning != nil {
				result = append(result, QueryInputMetricSemantics{Name: input.Name, SourceKey: input.SourceKey, Unit: input.Unit, Type: input.Type, Semantics: *meaning})
			}
		}
	}
	return result
}

func observedMetricSemantics(metric MetricDescriptor, sources []ObservationSource) *builtincollection.MetricSemantics {
	return BuiltinMetricSemanticsForSources(metric, sources)
}

// BuiltinMetricSemanticsForSources binds metadata to source-version keys already
// resolved and authorized by the caller. Unknown or conflicting inputs retain
// unknown meaning; it does not authorize those sources or infer derived units.
func BuiltinMetricSemanticsForSources(metric MetricDescriptor, sources []ObservationSource) *builtincollection.MetricSemantics {
	meaning := builtincollection.InputMetricSemantics(metric.SourceType, metric.Name, metric.Type, metric.Unit)
	if meaning == nil || len(metric.SourceKeys) == 0 {
		return nil
	}
	for _, key := range metric.SourceKeys {
		found := false
		for _, source := range sources {
			if source.SourceType == metric.SourceType && slices.Contains(source.Keys, key) {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return meaning
}
