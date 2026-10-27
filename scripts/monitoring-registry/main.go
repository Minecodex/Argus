package main

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"os"
)

func main() {
	raw, err := json.MarshalIndent(monitoringcatalog.All(), "", "  ")
	if err != nil {
		panic(err)
	}
	output := "/** Generated from internal/monitoringcatalog. DO NOT EDIT. */\nexport const monitoringIntegrations = " + string(raw) + " as const;\n"
	if err = os.WriteFile("web/packages/api-client/src/generated/monitoring-registry.ts", []byte(output), 0644); err != nil {
		panic(err)
	}
}
