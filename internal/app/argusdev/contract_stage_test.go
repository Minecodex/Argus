package argusdev

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestContractStagePreservesWorkspaceAndRemovesOnlyOwnedDependencyAlias(t *testing.T) {
	root := t.TempDir()
	runner := Runner{Dir: root, Stdout: io.Discard, Stderr: io.Discard}
	if err := runner.Run(t.Context(), nil, "git", "init"); err != nil {
		t.Fatal(err)
	}
	fixtures := map[string]string{".gitignore": ".env\nnode_modules/\ninternal/gen/\n", ".env": "private-not-an-input", "go.mod": "module example.invalid/stage\n", "input.go": "package fixture\n", "node_modules/canary": "must-survive", "internal/gen/openapi/canary.go": "package fixture\n", "api/openapi/generated/canary.json": "{}", "web/packages/api-client/src/generated/canary.ts": "export const value=1;"}
	for name, value := range fixtures {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	app := App{root: root, runner: runner}
	stage, close, err := app.stageContracts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if stage != nil {
			_ = close()
		}
	}()
	if _, err = os.Stat(filepath.Join(stage.root, ".env")); !os.IsNotExist(err) {
		t.Fatal("ignored private input entered the contract stage")
	}
	if _, err = os.Stat(filepath.Join(stage.root, "internal/gen/openapi/canary.go")); err != nil {
		t.Fatal("ignored generated compile inputs were omitted", err)
	}
	if err = os.RemoveAll(filepath.Join(stage.root, "internal/gen/openapi")); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "internal/gen/openapi/canary.go")); err != nil {
		t.Fatal("staged generation changed live generated packages", err)
	}
	stageRoot := stage.root
	if err = close(); err != nil {
		t.Fatal(err)
	}
	stage = nil
	if _, err = os.Stat(stageRoot); !os.IsNotExist(err) {
		t.Fatal("owned contract stage was not removed")
	}
	value, err := os.ReadFile(filepath.Join(root, "node_modules/canary"))
	if err != nil || string(value) != "must-survive" {
		t.Fatal("dependency alias cleanup changed live dependencies", err)
	}
}
