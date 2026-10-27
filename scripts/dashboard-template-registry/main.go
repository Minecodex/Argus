package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kakj-go/Argus/internal/dashboard"
)

func main() {
	definitions := dashboard.BuiltinTemplates()
	raw, err := json.MarshalIndent(definitions, "", "  ")
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile("api/contracts/dashboard-templates.json", append(raw, '\n'), 0644); err != nil {
		panic(err)
	}
	const header = "/** Generated from locked dashboard template definitions. DO NOT EDIT. */\n"
	base := "web/packages/api-client/src/generated"
	directory := filepath.Join(base, "dashboard-templates")
	if err = os.MkdirAll(directory, 0755); err != nil {
		panic(err)
	}
	imports, names := []string{header}, []string{}
	keep := map[string]bool{}
	for i, definition := range definitions {
		// Each immutable template stays independently reviewable and below the
		// repository's per-file line limit as the registry grows.
		name := strings.ReplaceAll(definition.Key, ".", "_")
		if keep[name+".ts"] {
			panic("duplicate generated template filename")
		}
		keep[name+".ts"] = true
		value, marshalErr := json.MarshalIndent(definition, "", "  ")
		if marshalErr != nil {
			panic(marshalErr)
		}
		if err = os.WriteFile(filepath.Join(directory, name+".ts"), []byte(header+"// prettier-ignore\nexport const template = "+string(value)+" as const;\n"), 0644); err != nil {
			panic(err)
		}
		alias := "template" + strconv.Itoa(i)
		imports = append(imports, "import { template as "+alias+" } from \"./dashboard-templates/"+name+"\";\n")
		names = append(names, alias)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		panic(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || keep[entry.Name()] || !strings.HasSuffix(entry.Name(), ".ts") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		value, readErr := os.ReadFile(path)
		if readErr != nil {
			panic(readErr)
		}
		if !strings.HasPrefix(string(value), header) {
			panic("unowned file in generated template directory")
		}
		if err = os.Remove(path); err != nil {
			panic(err)
		}
	}
	index := strings.Join(imports, "") + "\n// prettier-ignore\nexport const managedDashboardTemplates = [" + strings.Join(names, ", ") + "] as const;\n"
	if err = os.WriteFile(filepath.Join(base, "dashboard-templates.ts"), []byte(index), 0644); err != nil {
		panic(err)
	}
}
