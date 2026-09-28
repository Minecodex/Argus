//go:build m4e2e

package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Test-only interpolation reads actual preceding tool outputs. It never
// invents runtime identities or replaces the gateway's authorization checks.
func replayResultArguments(args map[string]any, messages []message) (map[string]any, error) {
	outputs := []any{}
	for _, m := range messages {
		if m.Role != "tool" && m.Type != "function_call_output" {
			continue
		}
		raw := replayContentText(m.Content)
		if m.Type == "function_call_output" {
			raw = m.Output
		}
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, err
		}
		outputs = append(outputs, value)
	}
	var resolve func(any) (any, error)
	resolve = func(v any) (any, error) {
		switch value := v.(type) {
		case map[string]any:
			if path, ok := value["$result_pointer"].(string); ok && len(value) == 1 {
				parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
				var current any = outputs
				for _, part := range parts {
					part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
					switch node := current.(type) {
					case []any:
						i, e := strconv.Atoi(part)
						if e != nil || i < 0 || i >= len(node) {
							return nil, fmt.Errorf("missing replay output")
						}
						current = node[i]
					case map[string]any:
						var exists bool
						current, exists = node[part]
						if !exists {
							return nil, fmt.Errorf("missing replay output field")
						}
					default:
						return nil, fmt.Errorf("invalid replay output pointer")
					}
				}
				return current, nil
			}
			out := map[string]any{}
			for k, item := range value {
				resolved, err := resolve(item)
				if err != nil {
					return nil, err
				}
				out[k] = resolved
			}
			return out, nil
		case []any:
			out := make([]any, len(value))
			for i, item := range value {
				resolved, err := resolve(item)
				if err != nil {
					return nil, err
				}
				out[i] = resolved
			}
			return out, nil
		default:
			return v, nil
		}
	}
	result, err := resolve(args)
	if err != nil {
		return nil, err
	}
	return result.(map[string]any), nil
}
