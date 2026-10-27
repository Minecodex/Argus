package main

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"os"
)

func main() {
	raw, err := json.MarshalIndent(kubernetesinventory.Definitions(), "", "  ")
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile("web/packages/api-client/src/generated/kubernetes-inventory-registry.ts", []byte("/** Generated from internal/kubernetesinventory. DO NOT EDIT. */\nexport const kubernetesInventoryTypes = "+string(raw)+" as const;\n"), 0644); err != nil {
		panic(err)
	}
}
