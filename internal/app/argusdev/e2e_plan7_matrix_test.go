package argusdev

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlan7TestEvidenceRejectsFailureAndDoesNotPromoteSkips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tests.jsonl")
	if err := os.WriteFile(path, []byte("{\"Action\":\"pass\",\"Test\":\"executed\"}\n{\"Action\":\"skip\",\"Test\":\"not_executed\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	passed, err := plan7PassedTests(path)
	if err != nil || !passed["executed"] || passed["not_executed"] {
		t.Fatalf("skips were promoted: %v %v", passed, err)
	}
	if err = os.WriteFile(path, []byte("{\"Action\":\"pass\",\"Test\":\"earlier\"}\n{\"Action\":\"fail\",\"Test\":\"later\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = plan7PassedTests(path); err == nil {
		t.Fatal("failed test log was accepted")
	}
}

func TestPlan7ProductionWebRejectsFixtureConfiguration(t *testing.T) {
	for _, mode := range []string{"production", "loopback_override", "mock", "missing_egress", "other_run", "other_source", "changed_digest"} {
		t.Run(mode, func(t *testing.T) {
			env := &E2EEnvironment{Options: E2EOptions{RunID: "frozen-run", Artifacts: t.TempDir()}, State: NewScenarioState("frozen-run")}
			env.State.Values["p6_source_fingerprint"] = "frozen-source"
			digest := "sha256:" + strings.Repeat("a", 64)
			receipt := map[string]any{"run_id": env.Options.RunID, "source_fingerprint": "frozen-source", "profile": "production", "api_mode": "real", "api_base_url": "/", "direct_egress_addresses": "", "image_digest": digest}
			switch mode {
			case "loopback_override":
				receipt["direct_egress_addresses"] = "127.0.0.1"
			case "mock":
				receipt["api_mode"] = "mock"
			case "missing_egress":
				delete(receipt, "direct_egress_addresses")
			case "other_run":
				receipt["run_id"] = "another-run"
			case "other_source":
				receipt["source_fingerprint"] = "another-source"
			case "changed_digest":
				receipt["image_digest"] = "sha256:" + strings.Repeat("b", 64)
			}
			for name, value := range map[string]any{"web-build-config.json": receipt, "image-provenance.json": map[string]any{"core_images": []plan6Image{{Name: "web", ImageID: digest}}}} {
				raw, _ := json.Marshal(value)
				if err := os.WriteFile(filepath.Join(env.Options.Artifacts, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := validatePlan7ProductionWeb(env); (err == nil) != (mode == "production") {
				t.Fatalf("%s: %v", mode, err)
			}
		})
	}
}

func TestPlan7CoverageIncludesEveryRequiredMatrixIDOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, entry := range plan7CoverageRequirements() {
		if seen[entry.ID] || (len(entry.Tests) == 0 && len(entry.Phases) == 0) {
			t.Fatalf("duplicate or evidence-free row: %s", entry.ID)
		}
		seen[entry.ID] = true
	}
	for _, group := range []struct {
		prefix string
		count  int
	}{{"A", 38}, {"M", 11}, {"U", 8}} {
		for n := 1; n <= group.count; n++ {
			id := fmt.Sprintf("P7-%s%02d", group.prefix, n)
			if !seen[id] {
				t.Fatalf("required matrix item omitted: %s", id)
			}
			delete(seen, id)
		}
	}
	if len(seen) != 0 {
		t.Fatalf("unexpected matrix rows: %v", seen)
	}
}
