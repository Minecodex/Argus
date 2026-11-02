package toolgateway

import "github.com/kakj-go/Argus/internal/toolruntime"

// Navigation suggestions are filtered by the exact current Gateway scope, not
// merely by a resource relationship or the broad provider source label.
func (g *Gateway) filterCapabilityReferences(data map[string]any, p toolruntime.Principal, allow func(string) bool) {
	rows, _ := data["items"].([]any)
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		sections, _ := row["sections"].(map[string]any)
		next, ok := sections["next_tools"].(map[string]any)
		if !ok {
			continue
		}
		refs, _ := next["items"].([]any)
		filtered := []any{}
		for _, raw := range refs {
			ref, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			category, _ := ref["category"].(string)
			name, _ := ref["name"].(string)
			m, ok := g.byName[category+"/"+name]
			if ok && m.allowed(p) && allow(m.ToolID) {
				filtered = append(filtered, ref)
			}
		}
		next["items"] = filtered
	}
}
