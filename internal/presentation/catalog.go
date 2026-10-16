package presentation

// NormalizeCatalog freezes the queried collection, including scalar values.
// Empty metrics must not hide nonempty fields/values. An explicit query kind
// wins even for an empty result; the template decides its business layout.
func NormalizeCatalog(data map[string]any, kind string) {
	keys := []string{"metrics", "fields", "values", "resources", "candidates", "signals"}
	valid := func(s string) bool {
		for _, key := range keys {
			if key == s {
				return true
			}
		}
		return false
	}
	if !valid(kind) {
		kind, _ = data["catalog_kind"].(string)
	}
	if !valid(kind) {
		kind = ""
		for _, key := range keys {
			if rows, ok := data[key].([]any); ok && len(rows) > 0 {
				kind = key
				break
			}
		}
		if kind == "" {
			for _, key := range keys {
				if _, ok := data[key]; ok {
					kind = key
					break
				}
			}
		}
	}
	if kind == "" {
		return
	}
	rows, _ := data[kind].([]any)
	items := make([]any, 0, len(rows))
	for _, row := range rows {
		if kind == "values" {
			items = append(items, map[string]any{"value": row})
		} else {
			items = append(items, row)
		}
	}
	data["catalog_kind"], data["items"] = kind, items
}
