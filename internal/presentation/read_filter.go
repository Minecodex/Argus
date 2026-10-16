package presentation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// These are public scalar fields, never an expression, path, URL or a query DSL.
var resultFilterFields = []string{"status", "connection_status", "severity", "level", "environment", "namespace", "resource_type", "service_name", "signal", "type"}
var resultSortFields = []string{"name", "title", "hostname", "id", "status", "severity", "namespace", "environment", "updated_at", "created_at", "started_at", "timestamp"}

func containsField(fields []string, key string) bool {
	for _, field := range fields {
		if field == key {
			return true
		}
	}
	return false
}

func scalar(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case bool, float64, int:
		return fmt.Sprint(v)
	}
	return ""
}

func searchPublic(value any, query string, depth int) bool {
	if depth > 5 {
		return false
	}
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if !strings.HasPrefix(key, "_") && !privatePresentationKey(key) && searchPublic(child, query, depth+1) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if searchPublic(child, query, depth+1) {
				return true
			}
		}
	default:
		return strings.Contains(strings.ToLower(scalar(value)), query)
	}
	return false
}

func filterPageItems(items []any, parameters map[string]any) ([]any, map[string][]string, string, error) {
	search, order, direction := "", "", "asc"
	for key, target := range map[string]*string{"search": &search, "sort": &order, "direction": &direction} {
		if raw, exists := parameters[key]; exists {
			value, ok := raw.(string)
			if !ok || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 256 {
				return nil, nil, "", ErrReadInvalid
			}
			*target = strings.TrimSpace(value)
		}
	}
	if (order != "" && !containsField(resultSortFields, order)) || (direction != "asc" && direction != "desc") {
		return nil, nil, "", ErrReadInvalid
	}
	filters := map[string]string{}
	if raw, exists := parameters["filters"]; exists {
		object, ok := raw.(map[string]any)
		if !ok || len(object) > len(resultFilterFields) {
			return nil, nil, "", ErrReadInvalid
		}
		for key, raw := range object {
			value, ok := raw.(string)
			if !ok || utf8.RuneCountInString(value) > 256 || !containsField(resultFilterFields, key) {
				return nil, nil, "", ErrReadInvalid
			}
			if value != "" {
				filters[key] = value
			}
		}
	}
	facets := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, raw := range items {
		row, _ := raw.(map[string]any)
		for _, key := range resultFilterFields {
			value := scalar(row[key])
			if value == "" || utf8.RuneCountInString(value) > 256 {
				continue
			}
			if seen[key] == nil {
				seen[key] = map[string]bool{}
			}
			if len(seen[key]) > 50 {
				continue
			}
			seen[key][value] = true
		}
	}
	for key, values := range seen {
		// High-cardinality fields remain searchable; never publish a truncated dropdown.
		if len(values) > 50 {
			continue
		}
		for value := range values {
			facets[key] = append(facets[key], value)
		}
		sort.Strings(facets[key])
	}
	query := strings.ToLower(search)
	filtered := make([]any, 0, len(items))
	for _, raw := range items {
		row, _ := raw.(map[string]any)
		matches := query == "" || searchPublic(raw, query, 0)
		for key, value := range filters {
			if scalar(row[key]) != value {
				matches = false
				break
			}
		}
		if matches {
			filtered = append(filtered, raw)
		}
	}
	if order != "" {
		sort.SliceStable(filtered, func(i, j int) bool {
			a, _ := filtered[i].(map[string]any)
			b, _ := filtered[j].(map[string]any)
			if left, ok := a[order].(float64); ok {
				if right, ok := b[order].(float64); ok {
					if direction == "desc" {
						return left > right
					}
					return left < right
				}
			}
			left, right := scalar(a[order]), scalar(b[order])
			if direction == "desc" {
				return left > right
			}
			return left < right
		})
	}
	encoded, _ := json.Marshal([]any{search, filters, order, direction})
	return filtered, facets, string(encoded), nil
}
