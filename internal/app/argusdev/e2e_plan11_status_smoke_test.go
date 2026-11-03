package argusdev

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPlan11OwnedStatusFixtureRunsWithoutPython(t *testing.T) {
	if os.Getenv("ARGUS_PLAN11_STATUS_SMOKE") != "1" {
		t.Skip("requires explicit owned Docker status fixture preflight")
	}
	image := os.Getenv("ARGUS_P7_SYSTEMD_IMAGE")
	if !strings.HasPrefix(image, "sha256:") {
		t.Fatal("requires the exact immutable acceptance host image")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	binary := filepath.Join(directory, "status-server")
	build := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-o", binary, "./cmd/argus-e2e-status")
	build.Dir = root
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build owned Linux status fixture: %v %s", err, out)
	}
	statusFile := filepath.Join(directory, "status.conf")
	if err = os.WriteFile(statusFile, []byte("status=broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	owner := "plan11-status-smoke-" + uuid.NewString()
	const mount = "/tmp/argus-chat-interactions-smoke"
	start := exec.CommandContext(t.Context(), "docker", "run", "--rm", "-d", "--label", "argus.io/owner="+owner,
		"--network", "none", "--read-only", "--cap-drop", "ALL", "--memory", "128m", "--cpus", "0.5",
		"--mount", "type=bind,source="+directory+",target="+mount+",readonly", "--entrypoint", mount+"/status-server",
		image, "--listen", "127.0.0.1:18431", "--status-file", mount+"/status.conf")
	out, err := start.CombinedOutput()
	if err != nil {
		t.Fatalf("start owned no-network status fixture: %v %s", err, out)
	}
	id := strings.TrimSpace(string(out))
	if len(id) != 64 {
		t.Fatal("Docker did not return the exact owned fixture ID")
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		actual, err := exec.CommandContext(cleanup, "docker", "inspect", "--format", `{{index .Config.Labels "argus.io/owner"}}`, id).CombinedOutput()
		if err != nil || strings.TrimSpace(string(actual)) != owner {
			t.Error("refusing status fixture cleanup after ownership change")
			return
		}
		if out, err := exec.CommandContext(cleanup, "docker", "stop", "--time", "2", id).CombinedOutput(); err != nil {
			t.Errorf("owned fixture stop: %v %s", err, out)
		}
	})
	for _, tc := range []struct{ state, code string }{{"status=broken\n", "503"}, {"status=healthy\n", "200"}, {"status=broken\n", "503"}} {
		if err = os.WriteFile(statusFile, []byte(tc.state), 0600); err != nil {
			t.Fatal(err)
		}
		ready := false
		for attempt := 0; attempt < 20; attempt++ {
			code, callErr := exec.CommandContext(t.Context(), "docker", "exec", id, "curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "http://127.0.0.1:18431/").CombinedOutput()
			if callErr == nil && strings.TrimSpace(string(code)) == tc.code {
				ready = true
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !ready {
			t.Fatalf("actual loopback fixture did not observe %s", tc.code)
		}
	}
	if report := os.Getenv("ARGUS_PLAN11_STATUS_SMOKE_EVIDENCE"); report != "" {
		if !filepath.IsAbs(report) {
			t.Fatal("status smoke evidence path must be absolute")
		}
		raw, _ := json.MarshalIndent(map[string]any{"passed": true, "image": image, "owner": owner, "container_id": id,
			"network": "none", "root_read_only": true, "python_required": false, "actual_status_sequence": []string{"503", "200", "503"}, "scope": "same immutable host image, actual owned Go binary and curl; no platform/model claim"}, "", "  ")
		if err = os.WriteFile(report, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
