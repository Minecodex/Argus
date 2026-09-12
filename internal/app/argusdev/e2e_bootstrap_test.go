package argusdev

import (
	"testing"
	"time"
)

func TestP4OneTimeResultChecksWorkerBootstrapPolicy(t *testing.T) {
	bootstrapSHA := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	command := "curl --insecure --header 'X-Argus-Enrollment-Token: test-token' https://example.test/bootstrap-script?scope=linux-system; printf '%s' " + bootstrapSHA + " | sha256sum -c -"
	set := map[string]any{"platform": "linux_amd64", "shell": "posix_sh", "privilege": "system", "command": command,
		"bootstrap_tls_mode": "insecure-first-fetch", "bootstrap_sha256": bootstrapSHA,
		"installer_sha256": bootstrapSHA, "trust_bundle_sha256": bootstrapSHA}
	result := map[string]any{"schema_version": "argus.action_one_time_result/v3", "result_kind": "connector_install_command",
		"execution_id": "execution", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339), "instruction_sets": []any{set}}
	confirmation := map[string]any{"execution": map[string]any{"execution_id": "execution"}}
	if actual, err := validateP4OneTimeResult(result, "connector_install_command", confirmation); err != nil || actual != command {
		t.Fatalf("structured command rejected: %v", err)
	}
	set["bootstrap_tls_mode"] = "strict"
	if _, err := validateP4OneTimeResult(result, "connector_install_command", confirmation); err == nil {
		t.Fatal("Worker silently losing the managed first-fetch policy passed E2E validation")
	}
	set["bootstrap_tls_mode"] = "insecure-first-fetch"
	set["command"] = "curl --header 'X-Argus-Enrollment-Token: test-token' https://example.test/bootstrap-script?scope=linux-system"
	if _, err := validateP4OneTimeResult(result, "connector_install_command", confirmation); err == nil {
		t.Fatal("command without its configured first-fetch exception passed E2E validation")
	}
}
