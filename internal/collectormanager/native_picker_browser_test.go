package collectormanager_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Run after the application-only browser checks. Additional fixture grants
// exist only for the picker checks and are removed before the SDK revoke gate.
func runNativePickerPortal(t *testing.T, f *nativeSDKPipeline, ctx context.Context, node, root, origin, apiURL, session, csrf string) {
	t.Helper()
	metadata, control, nonce, cleanup := setupActualPickerMetadata(t, f, session)
	defer cleanup()
	dir := os.Getenv("ARGUS_PLAN8_PICKER_BROWSER_EVIDENCE")
	if dir == "" {
		dir = t.TempDir()
	}
	command := exec.CommandContext(ctx, node, filepath.Join(root, "node_modules", "@playwright", "test", "cli.js"), "test", "e2e/plan8-picker-native.spec.ts", "--workers=1", "--max-failures=1", "--trace=off", "--output", dir)
	command.Dir = filepath.Join(root, "web", "apps", "enterprise")
	command.Env = append(os.Environ(), "ARGUS_E2E_EXTERNAL=1", "ARGUS_E2E_ENTERPRISE_ORIGIN="+origin, "ARGUS_PLAN8_NATIVE_API="+apiURL, "ARGUS_PLAN8_NATIVE_SESSION="+session, "ARGUS_PLAN8_NATIVE_CSRF="+csrf, "ARGUS_PLAN8_PICKER_METADATA="+metadata, "ARGUS_PLAN8_PICKER_CONTROL="+control, "ARGUS_PLAN8_PICKER_NONCE="+nonce)
	hideNativeSDKWindow(command)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("real metadata picker browser: %v\n%s", err, output)
	}
	t.Logf("real metadata picker browser: %s", output)
}
