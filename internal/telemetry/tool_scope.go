package telemetry

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/resourceref"
)

// ModelScopeSchema is the explicit subset of the shared portal query scope.
// Listing authorized objects never silently grants an all-resource query.
func ModelScopeSchema() map[string]any {
	text := func(max int) map[string]any {
		return map[string]any{"type": "string", "minLength": 1, "maxLength": max}
	}
	kinds := map[string]any{"type": "string", "enum": []string{"host", "kubernetes_cluster", "kubernetes_namespace", "application", "monitoring_target"}}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"mode", "resources"}, "properties": map[string]any{
		"mode":           map[string]any{"const": "explicit"},
		"resources":      map[string]any{"type": "array", "minItems": 1, "maxItems": 1000, "uniqueItems": true, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"type", "id"}, "properties": map[string]any{"type": kinds, "id": map[string]any{"type": "string", "format": "uuid"}}}},
		"resource_types": map[string]any{"type": "array", "maxItems": 5, "uniqueItems": true, "items": kinds},
		"source_type":    text(128), "source_keys": map[string]any{"type": "array", "minItems": 1, "maxItems": 10000, "uniqueItems": true, "items": text(256)},
	}}
}

func modelQueryScope(input map[string]any) ([]uuid.UUID, *QueryScope, error) {
	raw, present := input["scope"]
	if !present {
		ids, err := inputUUIDs(input["resource_ids"])
		return ids, nil, err
	}
	if _, exists := input["resource_ids"]; exists {
		return nil, nil, ErrQueryInvalid
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, nil, ErrQueryInvalid
	}
	var scope QueryScope
	if json.Unmarshal(encoded, &scope) != nil || scope.Mode != "explicit" || len(scope.Resources) == 0 || scope.Validate() != nil {
		return nil, nil, ErrQueryInvalid
	}
	return nil, &scope, nil
}

func modelScopeAlternatives() []any {
	return []any{
		map[string]any{"required": []string{"scope"}, "not": map[string]any{"required": []string{"resource_ids"}}},
		map[string]any{"required": []string{"resource_ids"}, "not": map[string]any{"required": []string{"scope"}}},
	}
}

func queryResourceIDs(input map[string]any) ([]uuid.UUID, error) {
	ids, scope, err := modelQueryScope(input)
	if err != nil {
		return nil, err
	}
	if scope != nil {
		return resourceref.IDs(scope.Resources), nil
	}
	return ids, nil
}
