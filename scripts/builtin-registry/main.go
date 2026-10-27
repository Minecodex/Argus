package main

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"os"
)

func main() {
	raw, err := json.MarshalIndent(builtincollection.All(), "", "  ")
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile("web/packages/api-client/src/generated/builtin-collections.ts", []byte("/** Generated from fixed collection definitions. DO NOT EDIT. */\n// prettier-ignore\nexport const builtinCollections = "+string(raw)+" as const;\n"), 0644); err != nil {
		panic(err)
	}
	if err = syncSchemaKeys(); err != nil {
		panic(err)
	}
}

// The public API and generated client must admit exactly the same fixed keys
// as the runtime catalog. Keep the source contract in the generation boundary.
func syncSchemaKeys() error {
	const path = "api/openapi/components/monitoring.yaml"
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var document map[string]any
	if err = json.Unmarshal(raw, &document); err != nil {
		return err
	}
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	keys := []string{}
	for _, definition := range builtincollection.All() {
		keys = append(keys, definition.Key)
	}
	for _, name := range []string{"MonitoringBuiltinDefinition", "MonitoringBuiltinInput"} {
		props := schemas[name].(map[string]any)["properties"].(map[string]any)
		props["key"].(map[string]any)["enum"] = keys
	}
	raw, err = json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0644)
}
