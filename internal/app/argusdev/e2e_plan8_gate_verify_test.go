package argusdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlan8FormalContractsRetainEveryOriginalCase(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := (&App{root: root}).plan8GateContracts()
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := os.ReadFile(filepath.Join(root, "docs", "plan8", "09-acceptance-matrix.md"))
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Cases []struct{ ID, Name, Actions, Expected, Environment string } `json:"cases"`
	}
	body, err := os.ReadFile(filepath.Join(root, "api", "contracts", "plan8-acceptance.json"))
	if err != nil || json.Unmarshal(body, &source) != nil {
		t.Fatal("contract source unavailable")
	}
	for i, c := range contracts {
		line := "| " + c.ID + " | " + source.Cases[i].Name + " | " + source.Cases[i].Actions + " | " + source.Cases[i].Expected + " | " + source.Cases[i].Environment + " |"
		if !strings.Contains(string(matrix), line) {
			t.Fatal("frozen case differs from original matrix", c.ID)
		}
	}
}

func TestPlan8MatrixCannotPassFromOnePlaceholderOrMissingVersions(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"support-matrix", "builtin-matrix", "release-and-preservation"} {
		payload := map[string]any{"schema_version": "argus.plan8_" + strings.ReplaceAll(name, "-", "_") + "/v1", "source_fingerprint": strings.Repeat("a", 64), "locked": true, "rows": []any{map[string]any{"id": "all", "version": "placeholder", "gates": map[string]bool{"CONFIG": true, "DATA": true, "VIEW": true, "AUTH": true, "LIFECYCLE": true}}}}
		body, _ := json.Marshal(payload)
		if err := os.WriteFile(filepath.Join(root, name+".json"), body, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := plan8VerifyMatrix(root, name, strings.Repeat("a", 64), plan8VerificationMaterials{}); err == nil {
			t.Fatal("placeholder matrix passed", name)
		}
	}
}
