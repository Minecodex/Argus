package toolgateway

import (
	"encoding/json"
	"sync"

	"github.com/kakj-go/Argus/api/schemas"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Manifest struct {
	SchemaVersion string `json:"schema_version"`
	Category      string `json:"category"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	mcp.Discovery
	DiscoveryHash        string         `json:"discovery_hash"`
	Risk                 string         `json:"risk"`
	Required             []string       `json:"required_permissions"`
	InputSchema          map[string]any `json:"input_schema"`
	OutputSchema         map[string]any `json:"output_schema,omitempty"`
	RequiresConfirmation bool           `json:"requires_confirmation"`
	ToolID               string         `json:"-"`
}

var manifestSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	var schema map[string]any
	if err := json.Unmarshal(schemas.ToolManifest, &schema); err != nil {
		return nil, err
	}
	return toolruntime.CompileInputSchema(schema)
})

func validateManifest(manifest Manifest) error {
	schema, err := manifestSchema()
	if err != nil {
		return err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	var document any
	if err = json.Unmarshal(data, &document); err != nil {
		return err
	}
	return schema.Validate(document)
}
