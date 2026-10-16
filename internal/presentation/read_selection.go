package presentation

import (
	"fmt"
	"sort"
	"strings"
)

func selectPageData(data, parameters map[string]any) (map[string]any, string, error) {
	panel, panelOK := parameters["panel_id"].(string)
	target, targetOK := parameters["target_id"].(string)
	if parameters["panel_id"] == nil && parameters["target_id"] == nil {
		return data, "", nil
	}
	if !panelOK || !targetOK || panel == "" || target == "" {
		return nil, "", ErrReadInvalid
	}
	panels, _ := data["panels"].([]any)
	for _, raw := range panels {
		p, ok := raw.(map[string]any)
		if !ok || p["id"] != panel {
			continue
		}
		targets, _ := p["targets"].([]any)
		for _, raw := range targets {
			t, ok := raw.(map[string]any)
			if !ok || t["id"] != target {
				continue
			}
			// PromQL files seal the Matrix/Vector itself, while other providers
			// return object envelopes. Both are bound to this frozen target.
			if rows, ok := t["data"].([]any); ok {
				return map[string]any{"items": rows}, fmt.Sprintf(":%s:%s", panel, target), nil
			}
			value, _ := t["data"].(map[string]any)
			if nested, ok := value["data"].(map[string]any); ok {
				value = nested
			}
			if kind, _ := t["result_type"].(string); strings.HasPrefix(kind, "apm_") && parameters["collection"] == "result" {
				keys := make([]string, 0, len(value))
				for key := range value {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				roots := []any{}
				for _, key := range keys {
					root, ok := value[key].(map[string]any)
					if !ok {
						continue
					}
					for _, field := range []string{"rows", "nodes", "edges"} {
						if _, ok := root[field].([]any); ok {
							roots = append(roots, root)
							break
						}
					}
				}
				if len(roots) > 0 {
					return map[string]any{"result": roots}, fmt.Sprintf(":%s:%s", panel, target), nil
				}
			}
			for _, key := range []string{"result", "series", "rows", "traces"} {
				if rows, ok := value[key].([]any); ok {
					return map[string]any{"items": rows}, fmt.Sprintf(":%s:%s", panel, target), nil
				}
			}
		}
	}
	return nil, "", ErrReadInvalid
}
