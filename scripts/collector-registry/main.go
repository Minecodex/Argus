package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
)

func main() {
	path := "web/packages/api-client/src/generated/collector-registry.ts"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	components := map[string][]string{}
	for _, platform := range []string{"linux_amd64", "linux_arm64", "windows_amd64"} {
		components[platform] = configbundle.DistributionComponents(platform)
	}
	raw, _ := json.MarshalIndent(components, "", "  ")
	text := fmt.Sprintf("/** Generated from internal/otelcol/configbundle. DO NOT EDIT. */\nexport const collectorDistributionVersion = %q;\nexport const collectorVersion = %q;\nexport const collectorConfigSchemaVersion = %q;\nexport const collectorComponents = %s as const;\n", configbundle.DistributionVersion, configbundle.CollectorVersion, configbundle.CatalogConfigSchemaVersion, raw)
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		panic(err)
	}
}
