package toolgateway

import (
	"encoding/json"
	"fmt"

	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/mcp"
)

// Native model calls must make a bounded resource choice. The portal retains
// its ordinary execution-input contract and forwards its visible selected scope.
func modelAuthoringScopeSchema(input map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var schema map[string]any
	if err = json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	properties, _ := schema["properties"].(map[string]any)
	parameters, _ := properties["parameters"].(map[string]any)
	fields, _ := parameters["properties"].(map[string]any)
	if fields == nil {
		return nil, fmt.Errorf("authoring parameters schema unresolved")
	}
	fields["resource_mode"] = map[string]any{"type": "string", "enum": []string{"explicit"}, "description": "Explicit bounded choice; do not drop resources while repairing another field. Proposed shortcut bindings are not data scope."}
	fields["resource_ids"] = map[string]any{"type": "array", "items": map[string]any{"type": "string", "format": "uuid"}, "uniqueItems": true, "maxItems": 1000}
	fields["resource_types"] = map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"host", "kubernetes_cluster", "kubernetes_namespace", "application", "monitoring_target"}}, "uniqueItems": true, "minItems": 1, "maxItems": 5}
	appendRequired := func(object map[string]any, names ...string) {
		values, _ := object["required"].([]any)
		for _, name := range names {
			found := false
			for _, prior := range values {
				if prior == name {
					found = true
				}
			}
			if !found {
				values = append(values, name)
			}
		}
		object["required"] = values
	}
	appendRequired(schema, "parameters")
	appendRequired(parameters, "resource_mode", "resource_ids", "resource_types")
	return schema, nil
}

func modelAuthoringParameters(input map[string]any) (dashboard.ExecutionInput, error) {
	value, err := dashboardToolDecode[struct {
		Parameters dashboard.ExecutionInput `json:"parameters"`
	}](input)
	if err != nil || value.Parameters.ResourceMode != "explicit" || value.Parameters.ResourceIDs == nil || len(value.Parameters.ResourceTypes) == 0 {
		return dashboard.ExecutionInput{}, mcp.ErrInputInvalid
	}
	return value.Parameters, nil
}
