package toolruntime

import (
	"encoding/json"
	"errors"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type localSchemaLoader struct{}

func (localSchemaLoader) Load(string) (any, error) {
	return nil, errors.New("external schema references are not supported")
}

// CompileInputSchema never fetches a URL supplied by a tool provider. Local
// definitions are permitted; external references must be bundled by that tool.
func CompileInputSchema(schema map[string]any) (*jsonschema.Schema, error) {
	if schema == nil || schema["type"] != "object" {
		return nil, Error{Kind: "TOOL_SCHEMA_INVALID", Message: "tool input schema must be an object"}
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(localSchemaLoader{})
	compiler.AssertFormat()
	const location = "https://argus.invalid/tool-input"
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil, Error{Kind: "TOOL_SCHEMA_INVALID"}
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, Error{Kind: "TOOL_SCHEMA_INVALID"}
	}
	if err := compiler.AddResource(location, normalized); err != nil {
		return nil, Error{Kind: "TOOL_SCHEMA_INVALID"}
	}
	compiled, err := compiler.Compile(location)
	if err != nil {
		return nil, Error{Kind: "TOOL_SCHEMA_INVALID"}
	}
	return compiled, nil
}
