package tooltemplatefixture

import (
	"fmt"
	"strings"
)

const LongMarker = "Boundary long name 名称 "
const SpecialMarker = "<script>alert('xss')</script> & \"中文\""

// Mutate the actual nested business fields. The original fixture remains
// immutable, and relations/IDs survive name-only variants.
func transform(value any, change func(string, any) (any, bool)) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if next, ok := change(key, child); ok {
				v[key] = next
			} else {
				transform(child, change)
			}
		}
	case []any:
		for _, child := range v {
			transform(child, change)
		}
	}
}
func displayField(key string) bool {
	switch key {
	case "name", "display_name", "hostname", "policy_name", "service_name", "rootService", "serviceName", "rootOperation", "operationName", "title", "description", "metric", "__name__", "expression", "body", "message", "subject", "text", "pod", "container", "kind":
		return true
	}
	return false
}
func Variants(entry string, base map[string]any) map[string]map[string]any {
	cases := map[string]map[string]any{"normal": base}
	empty := map[string]any{"view": base["view"], "tool": base["tool"], "tool_title": base["tool_title"], "resources": []any{}, "items": []any{}, "panels": []any{}, "events": []any{}, "observed_at": now, "status": "complete", "spec": map[string]any{"panels": []any{}}, "issues": []any{}, "data": map[string]any{"result": []any{}}, "catalog_kind": base["catalog_kind"]}
	cases["empty"] = empty
	for _, variant := range []string{"id-only", "missing-name", "long-name", "special-characters"} {
		data := clone(base)
		transform(data, func(key string, value any) (any, bool) {
			if !displayField(key) {
				return nil, false
			}
			if _, ok := value.(string); !ok {
				return nil, false
			}
			switch variant {
			case "long-name":
				return strings.Repeat(LongMarker, 12), true
			case "special-characters":
				return SpecialMarker, true
			default:
				return "", true
			}
		})
		if variant == "id-only" {
			transform(data, func(key string, value any) (any, bool) {
				switch key {
				case "inventory", "labels", "spec", "capabilities", "state":
					return map[string]any{}, true
				}
				return nil, false
			})
		}
		cases[variant] = data
	}
	failure := clone(base)
	failure["status"], failure["error_code"] = "failed", "CONTROLLED_FIXTURE_ERROR"
	cases["error"] = failure
	partial := clone(base)
	partial["partial"] = true
	cases["partial"] = partial
	large := clone(base)
	// Expand each real collection using rows of its actual type, rather than
	// replacing metric/log/alert rows with generic host objects.
	transform(large, func(key string, value any) (any, bool) {
		if entry == "trace-result" && key == "result" {
			return nil, false
		}
		if key != "items" && key != "result" && key != "rows" && key != "spans" && key != "rules" && key != "events" && key != "resources" && key != "issues" {
			return nil, false
		}
		rows, ok := value.([]any)
		if !ok || len(rows) == 0 {
			return nil, false
		}
		count := 350
		if key == "rules" {
			count = 32
		}
		expanded := make([]any, 0, count)
		for i := 0; i < count; i++ {
			row := cloneValue(rows[i%len(rows)])
			if m, ok := row.(map[string]any); ok {
				if _, exists := m["id"]; exists {
					m["id"] = fmt.Sprintf("10000000-0000-4000-8000-%012d", i+1)
				}
			}
			expanded = append(expanded, row)
		}
		return expanded, true
	})
	cases["large"] = large
	return cases
}
func clone(data map[string]any) map[string]any { return object(data) }
func cloneValue(value any) any                 { return object(map[string]any{"value": value})["value"] }

// Business expectations are content contracts, independent of geometry. They
// are consumed by runtime and real Chatbox browser tests.
func Expected(entry string, data map[string]any) []string {
	if entry == "resource-list" || entry == "resource-detail" {
		if name, ok := data["name"].(string); ok && name != "" {
			return []string{name}
		}
		if items, ok := data["items"].([]any); ok && len(items) > 0 {
			if row, ok := items[0].(map[string]any); ok {
				if definition, ok := row["definition"].(map[string]any); ok {
					if name, ok := definition["name"].(string); ok && name != "" {
						return []string{name}
					}
				}
				for _, key := range []string{"name", "service_name", "kind", "key"} {
					if name, ok := row[key].(string); ok && name != "" {
						return []string{name}
					}
				}
			}
		}
	}
	switch entry {
	case "collector-detail", "collector-list":
		return []string{"db-prod-01", "7 / 6"}
	case "cluster-detail", "cluster-list":
		return []string{"orders-production"}
	case "connector-detail", "connector-list":
		return []string{"db-prod-connector", "db-prod-01", "v1.2.3"}
	case "kubernetes-resource-list":
		return []string{"orders-api"}
	case "metric-query":
		return []string{"db_connections", "orders", "warehouse", "connections"}
	case "log-query":
		return []string{"Database connection timed out", "* | limit 10"}
	case "pod-logs":
		return []string{"database timeout"}
	case "trace-result":
		return []string{"orders-api", "POST /orders", "queryTrace"}
	case "alert-policy-detail":
		return []string{"host_cpu_usage_percent", "host_memory_usage_percent", "95"}
	case "alert-event-detail":
		return []string{"Host resource pressure / 3", "91 % / 85 %", "maintenance"}
	case "alert-events", "alert-history":
		return []string{"Host resource pressure", "db-prod-01", "91 % / 85 %"}
	case "alert-notifications":
		return []string{"CPU pressure on db-prod-01", "SMTP connection refused"}
	case "alert-policy-list", "alert-settings-list":
		return []string{"Host resource pressure"}
	case "telemetry-catalog":
		if data["catalog_kind"] == "resources" {
			return []string{"db-prod-01", "hostmetrics"}
		}
		return []string{"db_connections", "connections", "database"}
	case "tool-search", "tool-description":
		return []string{"host"}
	case "dashboard-draft-validation":
		return []string{"panels[0].targets[0]", "Metric source binding is required"}
	case "dashboard-definition", "dashboard-list", "dashboard-result":
		return []string{"Database overview"}
	case "change-preview", "action-detail", "action-list":
		return []string{"Update orders host labels"}
	case "telemetry-summary":
		return []string{"240", "82"}
	default:
		return []string{"db-prod-01"}
	}
}
