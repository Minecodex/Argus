package argusdev

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type plan8EvidenceReference struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type plan8GateContract struct {
	ID         string   `json:"id"`
	Assertions []string `json:"assertions"`
	Levels     []string `json:"allowed_levels"`
}
type plan8GateArtifact struct {
	SchemaVersion     string                   `json:"schema_version"`
	ID                string                   `json:"gate_id"`
	RunID             string                   `json:"run_id"`
	Status            string                   `json:"status"`
	Level             string                   `json:"verification_level"`
	SourceFingerprint string                   `json:"source_fingerprint"`
	ContractSHA256    string                   `json:"contract_sha256"`
	Assertions        map[string]bool          `json:"assertions"`
	StartedAt         time.Time                `json:"started_at"`
	FinishedAt        time.Time                `json:"finished_at"`
	References        []plan8EvidenceReference `json:"references"`
}

func plan8ContractHash(contract plan8GateContract) string {
	body, _ := json.Marshal(contract)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// Loaders do not infer a case from package success. A named producer must
// state every frozen assertion and bind its evidence to this exact source.
func plan8ValidateGateArtifact(root, source string, contract plan8GateContract, artifact plan8GateArtifact, now time.Time) error {
	if err := plan8ValidateGateEnvelope(root, source, contract, artifact, now); err != nil {
		return err
	}
	if artifact.Status != "passed" {
		return fmt.Errorf("gate %s remains %s", contract.ID, artifact.Status)
	}
	for _, assertion := range contract.Assertions {
		if assertion == "" || !artifact.Assertions[assertion] {
			return fmt.Errorf("gate %s missing assertion %s", contract.ID, assertion)
		}
	}
	return nil
}

func plan8ValidateGateObservation(root, source string, contract plan8GateContract, artifact plan8GateArtifact, now time.Time) error {
	if artifact.Status == "passed" {
		return plan8ValidateGateArtifact(root, source, contract, artifact, now)
	}
	switch artifact.Status {
	case "not_run", "incomplete", "failed", "blocked", "deferred":
	default:
		return errors.New("unknown gate observation status")
	}
	return plan8ValidateGateEnvelope(root, source, contract, artifact, now)
}

func plan8ValidateGateEnvelope(root, source string, contract plan8GateContract, artifact plan8GateArtifact, now time.Time) error {
	if !regexp.MustCompile(`^P8-A(0[1-9]|[1-6][0-9]|7[0-6])$`).MatchString(contract.ID) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(source) {
		return errors.New("gate or source identity invalid")
	}
	if artifact.SchemaVersion != "argus.plan8_gate_evidence/v1" || artifact.ID != contract.ID || artifact.RunID == "" {
		return errors.New("gate evidence identity differs")
	}
	if source == "" || artifact.SourceFingerprint != source || artifact.ContractSHA256 != plan8ContractHash(contract) {
		return errors.New("gate source or assertion contract differs")
	}
	allowed := false
	for _, level := range contract.Levels {
		allowed = allowed || level == artifact.Level
	}
	if !allowed || len(contract.Assertions) == 0 {
		return errors.New("gate verification level or assertion contract unavailable")
	}
	if artifact.StartedAt.IsZero() || artifact.FinishedAt.Before(artifact.StartedAt) || artifact.FinishedAt.After(now.Add(time.Minute)) {
		return errors.New("gate evidence timing invalid")
	}
	if len(artifact.References) == 0 {
		return errors.New("gate evidence has no actual references")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	for _, reference := range artifact.References {
		if reference.Path == "" || filepath.IsAbs(reference.Path) || filepath.VolumeName(reference.Path) != "" {
			return errors.New("evidence reference is not relative")
		}
		clean := filepath.Clean(reference.Path)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return errors.New("evidence reference escapes its owned root")
		}
		target := filepath.Join(absolute, clean)
		relative, err := filepath.Rel(absolute, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("evidence reference escaped")
		}
		current := absolute
		for _, part := range strings.Split(clean, string(filepath.Separator)) {
			current = filepath.Join(current, part)
			info, e := os.Lstat(current)
			if e != nil || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("evidence reference unavailable or linked")
			}
		}
		info, err := os.Stat(target)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 32<<20 {
			return errors.New("evidence reference is not a bounded regular file")
		}
		body, err := os.ReadFile(target)
		if err != nil {
			return errors.New("evidence reference cannot be read")
		}
		sum := sha256.Sum256(body)
		if len(reference.SHA256) != 64 || reference.SHA256 != hex.EncodeToString(sum[:]) {
			return errors.New("evidence reference content differs")
		}
	}
	return nil
}
