package main

import (
	"encoding/json"
	"os"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine/skywalking"
)

func main() {
	raw, err := json.MarshalIndent(skywalking.ApplicationAPMQueries(), "", "  ")
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile("api/contracts/application-apm-queries.json", append(raw, '\n'), 0644); err != nil {
		panic(err)
	}
	if err = os.WriteFile("web/packages/api-client/src/generated/application-apm-queries.ts", []byte("/** Generated from the engine-validated application query registry. DO NOT EDIT. */\n// prettier-ignore\nexport const applicationAPMQueries = "+string(raw)+" as const;\n"), 0644); err != nil {
		panic(err)
	}
}
