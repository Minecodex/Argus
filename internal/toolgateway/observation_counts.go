package toolgateway

import (
	"strings"

	"github.com/google/uuid"
)

// Count only the already authorized returned page. No lookup, scan or global
// inventory count is implied, including when complete=false or has_more=true.
func observationPageCounts(data map[string]any) map[string]any {
	metrics, _ := data["metrics"].([]any)
	sources, _ := data["sources"].([]any)
	fields, _ := data["fields"].([]any)
	values, _ := data["values"].([]any)
	names, sourceIDs, sourceKeys := map[string]bool{}, map[string]bool{}, map[string]bool{}
	key := func(raw any) {
		text, ok := raw.(string)
		id, _, _ := strings.Cut(text, ":")
		if !ok || uuid.Validate(id) != nil {
			return
		}
		sourceIDs[id], sourceKeys[text] = true, true
	}
	rows := append(append([]any{}, metrics...), sources...)
	rows = append(append(rows, fields...), values...)
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		key(row["source_key"])
		keys, _ := row["source_keys"].([]any)
		for _, value := range keys {
			key(value)
		}
	}
	for _, raw := range metrics {
		if row, ok := raw.(map[string]any); ok {
			if name, ok := row["name"].(string); ok && name != "" {
				names[name] = true
			}
		}
	}
	return map[string]any{"scope": "returned_page", "metric_entries": len(metrics), "distinct_metric_names": len(names),
		"source_entries": len(sources), "distinct_source_ids": len(sourceIDs), "distinct_source_versions": len(sourceKeys),
		"field_entries": len(fields), "value_entries": len(values)}
}
