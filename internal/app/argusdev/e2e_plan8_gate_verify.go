package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Verification consumes explicit per-case observations. Test package success,
// a prior revision and an unrelated component manifest never close a gate.
func (a *App) verifyPlan8Evidence(ctx context.Context, args []string) error {
	f := flag.NewFlagSet("e2e-plan8 verify", flag.ContinueOnError)
	f.SetOutput(a.stderr)
	dir := f.String("evidence", "", "owned evidence directory containing cases/P8-Axx.json")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *dir == "" || f.NArg() != 0 {
		return fmt.Errorf("%w: e2e-plan8 verify --evidence artifacts/<owned-run>", errUsage)
	}
	root, err := filepath.Abs(filepath.Join(a.root, *dir))
	if filepath.IsAbs(*dir) {
		root = filepath.Clean(*dir)
	}
	rel, e := filepath.Rel(filepath.Join(a.root, "artifacts"), root)
	if err != nil || e != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("Plan8 evidence must be inside a specific artifacts run directory")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Plan8 evidence root is unavailable or linked")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || filepath.Clean(resolved) != root {
		return errors.New("Plan8 evidence root contains a linked parent")
	}
	current, err := a.plan8Snapshot(ctx)
	if err != nil {
		return err
	}
	materials, err := a.plan8Materials(current)
	if err != nil {
		return err
	}
	materialJSON, _ := json.MarshalIndent(materials, "", "  ")
	if err = writePrivate(filepath.Join(root, "frozen-materials.json"), materialJSON); err != nil {
		return err
	}
	contracts, err := a.plan8GateContracts()
	if err != nil {
		return err
	}
	statuses := make([]map[string]any, 0, len(contracts))
	passed := 0
	for _, contract := range contracts {
		row := map[string]any{"id": contract.ID, "status": "not_run", "contract_sha256": plan8ContractHash(contract)}
		body, err := plan8ReadOwnedEvidence(root, filepath.Join("cases", contract.ID+".json"))
		if err == nil {
			var artifact plan8GateArtifact
			if err = json.Unmarshal(body, &artifact); err == nil {
				err = plan8ValidateGateObservation(root, current.Fingerprint, contract, artifact, time.Now().UTC())
			}
			if err == nil {
				row["status"], row["evidence"] = artifact.Status, "cases/"+contract.ID+".json"
				if artifact.Status == "passed" {
					passed++
				}
			} else {
				row["status"], row["error"] = "failed_evidence_validation", err.Error()
			}
		} else if !os.IsNotExist(err) {
			row["status"], row["error"] = "unavailable", err.Error()
		}
		statuses = append(statuses, row)
	}
	// Matrices are separately required. Base cases cannot stand in for every
	// exact target/Collector/template combination or all 12 built-in abilities.
	matrixResults := map[string]any{}
	matricesPassed := true
	for _, name := range []string{"support-matrix", "builtin-matrix", "release-and-preservation"} {
		result, err := plan8VerifyMatrix(root, name, current.Fingerprint, materials)
		matrixResults[name] = result
		matricesPassed = matricesPassed && err == nil
	}
	complete := passed == len(contracts) && matricesPassed
	status := "incomplete"
	if complete {
		status = "passed"
	}
	report := map[string]any{"schema_version": "argus.plan8_acceptance_summary/v1", "status": status, "source_fingerprint": current.Fingerprint, "required_base_gates": len(contracts), "passed_base_gates": passed, "cases": statuses, "matrices": matrixResults, "plan_complete": complete, "release_eligible": complete, "windows_real_testing": "deferred_by_user_not_verified", "verified_at": time.Now().UTC()}
	body, _ := json.MarshalIndent(report, "", "  ")
	path := filepath.Join(root, "acceptance-summary.json")
	if err := writePrivate(path, body); err != nil {
		return err
	}
	fmt.Fprintf(a.stdout, "Plan8 evidence %s: %d/%d base gates; status=%s\n", path, passed, len(contracts), status)
	if !complete {
		return errors.New("Plan8 required case, exact support combination, built-in or release evidence remains incomplete")
	}
	return nil
}

func (a *App) plan8GateContracts() ([]plan8GateContract, error) {
	body, err := os.ReadFile(filepath.Join(a.root, "api", "contracts", "plan8-acceptance.json"))
	if err != nil {
		return nil, err
	}
	var contract struct {
		SchemaVersion string              `json:"schema_version"`
		Cases         []plan8GateContract `json:"cases"`
	}
	if json.Unmarshal(body, &contract) != nil || contract.SchemaVersion != "argus.plan8_acceptance_contract/v1" || len(contract.Cases) != 76 {
		return nil, errors.New("Plan8 requires exactly 76 versioned base contracts")
	}
	for i, c := range contract.Cases {
		if c.ID != fmt.Sprintf("P8-A%02d", i+1) || len(c.Assertions) < 2 || len(c.Levels) == 0 {
			return nil, errors.New("Plan8 base assertion contract is incomplete or duplicated")
		}
	}
	return contract.Cases, nil
}

func plan8VerifyMatrix(root, name, source string, materials plan8VerificationMaterials) (map[string]any, error) {
	failure := func(message string) (map[string]any, error) {
		return map[string]any{"status": "incomplete", "reason": message}, errors.New(message)
	}
	body, err := plan8ReadOwnedEvidence(root, name+".json")
	if err != nil {
		return failure("matrix evidence missing")
	}
	var matrix struct {
		Schema      string `json:"schema_version"`
		Source      string `json:"source_fingerprint"`
		Locked      bool   `json:"locked"`
		Assessments []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"candidate_assessments"`
		Rows []struct {
			ID               string                   `json:"id"`
			Integration      string                   `json:"integration_id"`
			RunID            string                   `json:"run_id"`
			StartedAt        time.Time                `json:"started_at"`
			FinishedAt       time.Time                `json:"finished_at"`
			Version          string                   `json:"version"`
			CollectorSHA     string                   `json:"collector_sha256"`
			Platform         string                   `json:"platform"`
			Distribution     string                   `json:"distribution_version"`
			CollectorVersion string                   `json:"collector_version"`
			ViewKey          string                   `json:"template_key"`
			ViewVersion      int64                    `json:"template_version"`
			TemplateSHA      string                   `json:"template_sha256"`
			Gates            map[string]bool          `json:"gates"`
			References       []plan8EvidenceReference `json:"references"`
		} `json:"rows"`
	}
	if json.Unmarshal(body, &matrix) != nil || matrix.Schema != "argus.plan8_"+strings.ReplaceAll(name, "-", "_")+"/v1" || matrix.Source != source || !matrix.Locked || len(matrix.Rows) == 0 {
		return failure("matrix is unlocked, foreign or empty")
	}
	if name == "builtin-matrix" && len(matrix.Rows) != 12 {
		return failure("12 exact built-in capability records required")
	}
	deferred := map[string]bool{}
	for _, n := range []int{6, 7, 8, 13, 14, 15, 21, 23, 24, 25, 26, 33} {
		deferred[fmt.Sprintf("P8-I%02d", n)] = true
	}
	userDeferred := map[string]bool{"P8-I19": true, "P8-I39": true}
	if name == "support-matrix" {
		if len(matrix.Assessments) != 39 {
			return failure("all 39 candidate assessments required")
		}
		seen := map[string]bool{}
		for _, assessment := range matrix.Assessments {
			if !regexp.MustCompile(`^P8-I(0[1-9]|[12][0-9]|3[0-9])$`).MatchString(assessment.ID) || seen[assessment.ID] || assessment.Reason == "" {
				return failure("candidate assessment missing or duplicated")
			}
			seen[assessment.ID] = true
			expected := "selected"
			if deferred[assessment.ID] {
				expected = "deferred_upstream"
			}
			if userDeferred[assessment.ID] {
				expected = "deferred_by_user"
			}
			if assessment.Status != expected {
				return failure("unapproved candidate deferral or unverified selection")
			}
		}
	}
	seen := map[string]bool{}
	covered := map[string]bool{}
	hex := regexp.MustCompile(`^[a-f0-9]{64}$`)
	for _, row := range matrix.Rows {
		if row.ID == "" || seen[row.ID] || row.RunID == "" || row.Version == "" || !hex.MatchString(row.CollectorSHA) || !hex.MatchString(row.TemplateSHA) {
			return failure("matrix identity, exact version or artifact hash missing")
		}
		seen[row.ID] = true
		if err := materials.validate(row.Platform, row.Distribution, row.CollectorVersion, row.CollectorSHA, row.ViewKey, row.ViewVersion, row.TemplateSHA); err != nil {
			return failure(err.Error())
		}
		if name == "support-matrix" {
			if !regexp.MustCompile(`^P8-I(0[1-9]|[12][0-9]|3[0-9])$`).MatchString(row.Integration) || deferred[row.Integration] || userDeferred[row.Integration] {
				return failure("unknown or deferred support row")
			}
			covered[row.Integration] = true
		}
		if name == "builtin-matrix" {
			if !regexp.MustCompile(`^P8-(H0[1-4]|B0[1-8])$`).MatchString(row.Integration) || covered[row.Integration] {
				return failure("unknown or duplicate built-in capability")
			}
			covered[row.Integration] = true
		}
		keys := []string{"CONFIG", "DATA", "VIEW", "AUTH", "LIFECYCLE"}
		if name == "release-and-preservation" {
			keys = []string{"SOURCE", "DEPLOY", "SAMPLES", "ROLLBACK", "CLEANUP", "PROTECTED_DATA", "TARGET_READINESS"}
		}
		for _, key := range keys {
			if !row.Gates[key] {
				return failure("matrix gate remains unverified: " + row.ID + "/" + key)
			}
		}
		contract := plan8GateContract{ID: "P8-A45", Assertions: keys, Levels: []string{"full_e2e"}}
		if name == "release-and-preservation" {
			contract.ID = "P8-A64"
			contract.Levels = []string{"release"}
		}
		now := time.Now().UTC()
		artifact := plan8GateArtifact{SchemaVersion: "argus.plan8_gate_evidence/v1", ID: contract.ID, RunID: row.RunID, Status: "passed", Level: contract.Levels[0], SourceFingerprint: source, ContractSHA256: plan8ContractHash(contract), Assertions: row.Gates, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, References: row.References}
		if err := plan8ValidateGateArtifact(root, source, contract, artifact, now); err != nil {
			return failure(err.Error())
		}
	}
	if name == "support-matrix" && len(covered) != 25 {
		return failure("25 non-deferred integration families require exact support combinations")
	}
	return map[string]any{"status": "passed", "rows": len(matrix.Rows)}, nil
}

func plan8ReadOwnedEvidence(root, relative string) ([]byte, error) {
	target := filepath.Join(root, relative)
	info, err := os.Lstat(target)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil || resolved != target || !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return nil, errors.New("evidence is linked, oversized or not a regular file")
	}
	return os.ReadFile(target)
}
