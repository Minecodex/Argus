package collectormanager_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Optional real-browser layer; regular unit/native component runs do not start
// portals. The API/session/data remain live throughout the Playwright run.
func runNativeSDKPortal(t *testing.T, f *nativeSDKPipeline, apiURL, session, csrf, traceID string) {
	t.Helper()
	if os.Getenv("ARGUS_MONITORING_NATIVE_BROWSER") != "1" {
		return
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 150*time.Second)
	defer cancel()
	// The fixed port matches the API CORS allowlist. Binding proves that an
	// unrelated development server is not reused by this acceptance driver.
	listener, err := net.Listen("tcp", "127.0.0.1:4173")
	if err != nil {
		t.Fatal("native browser requires unoccupied desktop acceptance port 4173")
	}
	_ = listener.Close()
	origin := "http://127.0.0.1:4173"
	viteLogPath := filepath.Join(t.TempDir(), "real-portal.log")
	viteLogs, err := os.Create(viteLogPath)
	if err != nil {
		t.Fatal(err)
	}
	defer viteLogs.Close()
	vite := exec.CommandContext(ctx, node, filepath.Join(root, "scripts", "run-vite.mjs"), "enterprise", "4173", "real")
	vite.Dir = root
	vite.Env = append(os.Environ(), "VITE_API_BASE_URL="+apiURL, "VITE_TEMPLATE_ORIGIN="+origin)
	vite.Stdout, vite.Stderr = viteLogs, viteLogs
	hideNativeSDKWindow(vite)
	if err = vite.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = vite.Process.Kill(); _ = vite.Wait() }()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(20 * time.Second)
	for {
		response, err := client.Get(origin)
		if err == nil {
			response.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			logs, _ := os.ReadFile(viteLogPath)
			t.Fatalf("real portal readiness: %s", logs)
		}
		time.Sleep(100 * time.Millisecond)
	}
	playwright := exec.CommandContext(ctx, node, filepath.Join(root, "node_modules", "@playwright", "test", "cli.js"), "test", "e2e/plan8-application-native.spec.ts", "--workers=1", "--trace=off")
	playwright.Dir = filepath.Join(root, "web", "apps", "enterprise")
	playwright.Env = append(os.Environ(), "ARGUS_E2E_EXTERNAL=1", "ARGUS_E2E_ENTERPRISE_ORIGIN="+origin, "ARGUS_PLAN8_NATIVE_API="+apiURL, "ARGUS_PLAN8_NATIVE_SESSION="+session, "ARGUS_PLAN8_NATIVE_CSRF="+csrf, "ARGUS_PLAN8_NATIVE_APP="+f.apps[0].app.String(), "ARGUS_PLAN8_NATIVE_TRACE="+traceID)
	hideNativeSDKWindow(playwright)
	output, err := playwright.CombinedOutput()
	if err != nil {
		t.Fatalf("native SDK real portal: %v\n%s", err, output)
	}
	f.t.Logf("real portal Playwright: %s", output)
	runNativePickerPortal(t, f, ctx, node, root, origin, apiURL, session, csrf)
}
