package argusctl

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The sandbox configuration is produced by argusctl, not by chart defaults.
// Exercise Helm with those actual install values and reject incomplete inputs.
func TestSandboxChartLint(t *testing.T) {
	helmbin, err := exec.LookPath("helm")
	if err != nil {
		t.Fatal("sandbox chart acceptance requires Helm:", err)
	}
	root, err := findRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(filepath.Join(root, "deploy", "profiles", "local-formal.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	chart := filepath.Join(root, "deploy", "helm", "argus-sandbox")
	values := sandboxValues(cfg, "ci-only-public-test-key")
	encoded, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "values.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(helmbin, "lint", chart, "--values", path).CombinedOutput(); err != nil {
		t.Fatalf("generated install values failed Helm lint: %v\n%s", err, output)
	}
	// lint permits a required value to be omitted, so rendering is the
	// fail-closed check for someone attempting a bare Helm installation.
	if output, err := exec.Command(helmbin, "template", "sandbox", chart).CombinedOutput(); err == nil || !strings.Contains(string(output), "serverConfig is required") {
		t.Fatalf("bare install did not reject missing server configuration: %v\n%s", err, output)
	}
}
