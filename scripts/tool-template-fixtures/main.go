package main

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/tooltemplatefixture"
	"os"
)

func main() {
	catalog, err := tooltemplatefixture.Build()
	if err != nil {
		panic(err)
	}
	encoded, err := json.Marshal(catalog)
	if err != nil {
		panic(err)
	}
	path := "web/apps/template-runtime/src/fixtures.generated.json"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	if err = os.WriteFile(path, encoded, 0644); err != nil {
		panic(err)
	}
}
