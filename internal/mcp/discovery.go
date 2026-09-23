package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/kakj-go/Argus/api/schemas"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Discovery is tool-owned business documentation. It has no executor or
// authorization fields and never grants permission to call a tool.
type Discovery struct {
	SchemaVersion     string             `json:"schema_version"`
	Revision          string             `json:"revision"`
	Title             string             `json:"title"`
	Description       string             `json:"description"`
	Keywords          []string           `json:"keywords"`
	ResultDescription string             `json:"result_description"`
	Preconditions     []string           `json:"preconditions"`
	Examples          []DiscoveryExample `json:"examples"`
}

// Examples explain intent and argument sourcing, not fabricated resource IDs.
type DiscoveryExample struct {
	Request  string `json:"request"`
	Guidance string `json:"guidance"`
}

var discoverySchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	var schema map[string]any
	if err := json.Unmarshal(schemas.ToolDiscovery, &schema); err != nil {
		return nil, err
	}
	return toolruntime.CompileInputSchema(schema)
})

func NormalizeDiscovery(value Discovery) (Discovery, string, error) {
	// Own the slices so a registration caller cannot mutate a frozen catalog.
	value.Keywords = append([]string{}, value.Keywords...)
	for i, k := range value.Keywords {
		value.Keywords[i] = strings.ToLower(strings.TrimSpace(k))
	}
	slices.Sort(value.Keywords)
	value.Keywords = slices.Compact(value.Keywords)
	value.Preconditions = append([]string{}, value.Preconditions...)
	value.Examples = append([]DiscoveryExample{}, value.Examples...)
	data, err := json.Marshal(value)
	if err != nil {
		return Discovery{}, "", err
	}
	if len(data) > 16<<10 {
		return Discovery{}, "", fmt.Errorf("tool discovery exceeds byte limit")
	}
	schema, err := discoverySchema()
	if err != nil {
		return Discovery{}, "", err
	}
	var document any
	if err = json.Unmarshal(data, &document); err != nil {
		return Discovery{}, "", err
	}
	if err = schema.Validate(document); err != nil {
		return Discovery{}, "", fmt.Errorf("invalid tool discovery: %w", err)
	}
	hash := sha256.Sum256(data)
	return value, hex.EncodeToString(hash[:]), nil
}
