package toolgateway

import (
	"encoding/json"
	"fmt"
	"github.com/getkin/kin-openapi/openapi3"
	argusopenapi "github.com/kakj-go/Argus/api/openapi"
	"strings"
	"sync"
)

var nativePreviewDocument = sync.OnceValues(func() (*openapi3.T, error) { return openapi3.NewLoader().LoadFromData(argusopenapi.BundledJSON) })

// Native Preview inputs use the same resolved public contract as HTTP. Only
// request schemas are inlined; response/private action schemas never leak in.
func previewSchema(document *openapi3.T, name, idField string) (map[string]any, error) {
	ref := document.Components.Schemas[name]
	if name == "EmptyInput" {
		ref = &openapi3.SchemaRef{Value: openapi3.NewObjectSchema().WithoutAdditionalProperties()}
	}
	if ref == nil || ref.Value == nil {
		return nil, fmt.Errorf("preview input schema %s is missing", name)
	}
	value, err := inlinePreviewSchema(ref, map[*openapi3.Schema]bool{})
	if err != nil {
		return nil, err
	}
	if idField != "" {
		properties, _ := value["properties"].(map[string]any)
		if properties == nil {
			properties = map[string]any{}
			value["properties"] = properties
		}
		properties[idField] = map[string]any{"type": "string", "format": "uuid"}
		required, _ := value["required"].([]any)
		value["required"] = append(required, idField)
	}
	resolved, err := resolvePreviewExtensions(value, document, map[string]bool{})
	if err != nil {
		return nil, err
	}
	return resolved.(map[string]any), nil
}

// JSON Schema keywords such as propertyNames are OpenAPI extension values in
// kin-openapi; walk those too, without permitting any network schema loading.
func resolvePreviewExtensions(value any, document *openapi3.T, active map[string]bool) (any, error) {
	switch value := value.(type) {
	case map[string]any:
		if ref, ok := value["$ref"].(string); ok {
			const prefix = "#/components/schemas/"
			if !strings.HasPrefix(ref, prefix) || active[ref] {
				return nil, fmt.Errorf("unsupported Preview schema reference %s", ref)
			}
			active[ref] = true
			defer delete(active, ref)
			name := strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(ref, prefix), "~1", "/"), "~0", "~")
			resolved, err := inlinePreviewSchema(document.Components.Schemas[name], map[*openapi3.Schema]bool{})
			if err != nil {
				return nil, err
			}
			for key, child := range value {
				if key != "$ref" {
					resolved[key] = child
				}
			}
			return resolvePreviewExtensions(resolved, document, active)
		}
		for key, child := range value {
			resolved, err := resolvePreviewExtensions(child, document, active)
			if err != nil {
				return nil, err
			}
			value[key] = resolved
		}
	case []any:
		for i, child := range value {
			resolved, err := resolvePreviewExtensions(child, document, active)
			if err != nil {
				return nil, err
			}
			value[i] = resolved
		}
	}
	return value, nil
}

func inlinePreviewSchema(ref *openapi3.SchemaRef, active map[*openapi3.Schema]bool) (map[string]any, error) {
	if ref == nil || ref.Value == nil {
		return nil, fmt.Errorf("unresolved Preview schema")
	}
	schema := ref.Value
	if active[schema] {
		return nil, fmt.Errorf("recursive Preview request schemas are unsupported")
	}
	active[schema] = true
	defer delete(active, schema)
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err = json.Unmarshal(encoded, &value); err != nil {
		return nil, err
	}
	if len(schema.Properties) > 0 {
		properties := map[string]any{}
		for key, child := range schema.Properties {
			nested, err := inlinePreviewSchema(child, active)
			if err != nil {
				return nil, err
			}
			properties[key] = nested
		}
		value["properties"] = properties
	}
	if schema.Items != nil {
		nested, err := inlinePreviewSchema(schema.Items, active)
		if err != nil {
			return nil, err
		}
		value["items"] = nested
	}
	if schema.AdditionalProperties.Schema != nil {
		nested, err := inlinePreviewSchema(schema.AdditionalProperties.Schema, active)
		if err != nil {
			return nil, err
		}
		value["additionalProperties"] = nested
	}
	for key, refs := range map[string]openapi3.SchemaRefs{"allOf": schema.AllOf, "anyOf": schema.AnyOf, "oneOf": schema.OneOf} {
		if len(refs) == 0 {
			continue
		}
		children := []any{}
		for _, child := range refs {
			nested, err := inlinePreviewSchema(child, active)
			if err != nil {
				return nil, err
			}
			children = append(children, nested)
		}
		value[key] = children
	}
	if schema.Not != nil {
		nested, err := inlinePreviewSchema(schema.Not, active)
		if err != nil {
			return nil, err
		}
		value["not"] = nested
	}
	return value, nil
}
