package argusdev

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlan8GateEvidenceCannotTurnPartialOrForeignFactsIntoPass(t *testing.T) {
	root := t.TempDir()
	body := []byte("actual owned assertion output")
	if err := os.WriteFile(filepath.Join(root, "case.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	contract := plan8GateContract{ID: "P8-A01", Assertions: []string{"scope_checked", "observations_verified"}, Levels: []string{"real_component"}}
	now := time.Now().UTC()
	valid := plan8GateArtifact{SchemaVersion: "argus.plan8_gate_evidence/v1", ID: contract.ID, RunID: "unit-contract-only", Status: "passed", Level: "real_component", SourceFingerprint: strings.Repeat("a", 64), ContractSHA256: plan8ContractHash(contract), Assertions: map[string]bool{"scope_checked": true, "observations_verified": true}, StartedAt: now.Add(-time.Second), FinishedAt: now, References: []plan8EvidenceReference{{Path: "case.json", SHA256: hex.EncodeToString(sum[:])}}}
	if err := plan8ValidateGateArtifact(root, strings.Repeat("a", 64), contract, valid, now); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*plan8GateArtifact){
		func(a *plan8GateArtifact) { a.Status = "skipped" }, func(a *plan8GateArtifact) { a.Level = "unit" }, func(a *plan8GateArtifact) { a.SourceFingerprint = "other-source" }, func(a *plan8GateArtifact) { a.ContractSHA256 = "other-contract" }, func(a *plan8GateArtifact) { a.Assertions = map[string]bool{"scope_checked": true} }, func(a *plan8GateArtifact) { a.References = nil }, func(a *plan8GateArtifact) {
			a.References = []plan8EvidenceReference{{Path: "../business.json", SHA256: hex.EncodeToString(sum[:])}}
		}, func(a *plan8GateArtifact) {
			a.References = []plan8EvidenceReference{{Path: "case.json", SHA256: "bad"}}
		}, func(a *plan8GateArtifact) { a.FinishedAt = now.Add(24 * time.Hour) },
	} {
		candidate := valid
		change(&candidate)
		if err := plan8ValidateGateArtifact(root, strings.Repeat("a", 64), contract, candidate, now); err == nil {
			t.Fatal("partial, weaker, foreign or missing evidence became a pass")
		}
	}
}
