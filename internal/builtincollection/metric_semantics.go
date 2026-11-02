package builtincollection

// MetricSemantics describes a known input quantity from this collection's
// receiver contract. It never changes measured units/type or describes the
// result of arbitrary arithmetic on that input.
type MetricSemantics struct {
	QuantityKind                string `json:"quantity_kind"`
	RecommendedDisplayTransform string `json:"recommended_display_transform"`
	DefinitionID                string `json:"definition_id"`
	DefinitionVersion           string `json:"definition_version"`
	Origin                      string `json:"origin"`
	Notes                       string `json:"notes"`
}

func InputMetricSemantics(sourceType, name, metricType, unit string) *MetricSemantics {
	host, ok := Lookup("host_metrics")
	if !ok || sourceType != host.SourceType || metricType != "gauge" || unit != "1" {
		return nil
	}
	if name != "system_cpu_utilization" && name != "system_memory_utilization" {
		return nil
	}
	return &MetricSemantics{QuantityKind: "ratio", RecommendedDisplayTransform: "percent",
		DefinitionID: host.Key + "/" + name, DefinitionVersion: "argus.builtin_metric_semantics/v1", Origin: "builtin_collection_definition",
		Notes: "This authorized hostmetrics receiver gauge is a utilization ratio; raw unit remains 1. Percent display is raw*100. Unit 1 alone does not identify a ratio for other metrics/sources. This defines the input quantity, not arbitrary transformed expression output; preserve labels and scope. Explicit requests for raw units may retain identity."}
}
