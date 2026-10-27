package promqlengine

// Physical source/revision series retain exact historical provenance. Query
// samples from those rows are joined only after source authorization/filtering.
// A collection keeps one logical series when configuration or executor changes.
// Unrelated sources remain distinct, even when their service names coincide.
func logicalMetricLabels(raw map[string]string) map[string]string {
	result := make(map[string]string, len(raw))
	for key, value := range raw {
		if key == "argus_source_revision" || key == "argus_source_id" && raw["argus_collection_id"] != "" {
			continue
		}
		result[key] = value
	}
	return result
}
