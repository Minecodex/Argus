package toolgateway

// The portal's union DTO serves several domains. Model discovery exposes only
// the collection branch, with operation-specific requirements, so a missing
// enabled flag cannot silently become false at the Go decoding boundary.
func collectionMutationSchema(schema map[string]any) {
	props := schema["properties"].(map[string]any)
	for key := range props {
		switch key {
		case "kind", "operation", "id", "expected_version", "collection":
		default:
			delete(props, key)
		}
	}
	props["operation"] = map[string]any{"type": "string", "enum": []string{"create", "update", "archive", "set_enabled"}}
	schema["required"] = []string{"kind", "operation", "collection"}
	schema["allOf"] = []any{
		map[string]any{"if": map[string]any{"properties": map[string]any{"operation": map[string]any{"enum": []string{"update", "archive", "set_enabled"}}}}, "then": map[string]any{"required": []string{"id", "expected_version"}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"operation": map[string]any{"const": "set_enabled"}}}, "then": map[string]any{"properties": map[string]any{"collection": objectSchema(map[string]any{"enabled": map[string]any{"type": "boolean"}}, "enabled")}}},
	}
}
