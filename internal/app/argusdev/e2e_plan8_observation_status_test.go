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

func TestPlan8UnpassedObservationsRemainUnpassedWhileInvalidProofIsRejected(t *testing.T) {
	root := t.TempDir()
	data := []byte("unit structure only, no acceptance claim")
	if err := os.WriteFile(filepath.Join(root, "observed.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	now := time.Now().UTC()
	contract := plan8GateContract{ID: "P8-A07", Assertions: []string{"transaction_rolled_back", "outbox_empty"}, Levels: []string{"real_component"}}
	base := plan8GateArtifact{SchemaVersion: "argus.plan8_gate_evidence/v1", ID: contract.ID, RunID: "unit-contract-only", Level: "real_component", SourceFingerprint: strings.Repeat("a", 64), ContractSHA256: plan8ContractHash(contract), StartedAt: now.Add(-time.Second), FinishedAt: now, References: []plan8EvidenceReference{{Path: "observed.json", SHA256: hex.EncodeToString(sum[:])}}, Assertions: map[string]bool{"transaction_rolled_back": false, "outbox_empty": false}}
	for _, state := range []string{"not_run", "incomplete", "failed", "blocked", "deferred"} {
		item := base
		item.Status = state
		if err := plan8ValidateGateObservation(root, base.SourceFingerprint, contract, item, now); err != nil {
			t.Fatal("valid nonpassing observation misreported as malformed", state, err)
		}
		if err := plan8ValidateGateArtifact(root, base.SourceFingerprint, contract, item, now); err == nil {
			t.Fatal("nonpassing observation became acceptance", state)
		}
		item.SourceFingerprint = strings.Repeat("b", 64)
		if err := plan8ValidateGateObservation(root, base.SourceFingerprint, contract, item, now); err == nil {
			t.Fatal("nonpassing state bypassed source validation")
		}
	}
	base.Status = "passed"
	if err := plan8ValidateGateObservation(root, base.SourceFingerprint, contract, base, now); err == nil {
		t.Fatal("partial assertions became passing evidence")
	}
}
