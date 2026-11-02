package argusdev

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/dashboard"
)

type plan11RecordedReviewCase struct {
	Name     string                   `json:"name"`
	Question string                   `json:"question"`
	Sample   p5BenchmarkSample        `json:"sample"`
	Calls    json.RawMessage          `json:"calls"`
	Queries  []plan11QueryFile        `json:"original_query_files"`
	Reports  []plan11ReportFile       `json:"original_report_files"`
	Jobs     []dashboard.QueryJobView `json:"dashboard_jobs"`
	Hard     bool                     `json:"hard_failure"`
}

func TestRecordedR38CohortReplaysFactsWithoutRewritingOriginalResults(t *testing.T) {
	directory := os.Getenv("ARGUS_PLAN11_R38_REVIEW_DIR")
	if directory == "" {
		t.Skip("requires explicit immutable owned R38 evidence")
	}
	var profile struct {
		HostID string                     `json:"host_id"`
		Units  map[string]string          `json:"reference_catalog_units"`
		Cases  []plan11RecordedReviewCase `json:"cases"`
	}
	raw, err := os.ReadFile(filepath.Join(directory, "plan11-profile-comparison.json"))
	if err != nil || json.Unmarshal(raw, &profile) != nil || len(profile.Cases) != 18 {
		t.Fatal("full recorded cohort unavailable", err)
	}
	realFailures := map[string]bool{
		"plan11-compare-candidate-metric-en-2":    true,
		"plan11-compare-candidate-dashboard-zh-2": true,
		"plan11-compare-candidate-dashboard-zh-3": true,
		"plan11-compare-candidate-dashboard-en-1": true,
		"plan11-compare-candidate-dashboard-en-2": true,
	}
	passed := 0
	receipts := []map[string]any{}
	for _, row := range profile.Cases {
		err := plan11ReplayRecordedCase(directory, row, profile.HostID, profile.Units)
		if (err != nil) != realFailures[row.Name] {
			t.Errorf("recorded classification differs: %s: %v", row.Name, err)
		}
		if err == nil {
			passed++
		}
		receipts = append(receipts, map[string]any{"case": row.Name, "independent_fact_gates_passed": err == nil, "error": fmt.Sprint(err), "original_machine_passed": row.Sample.Passed})
		t.Logf("recorded source/report/conditions/process facts: %s passed=%v error=%v", row.Name, err == nil, err)
	}
	if passed != 13 {
		t.Errorf("expected 13 confirmed cases and five retained failures, got %d", passed)
	}
	result := map[string]any{"scope": "Corrected shared verifier replay of immutable R38 original query/panel bytes, call/process receipts, requested display policy and published report facts. Manual prose/type reviews remain separate.", "original_results_rewritten": false, "extra_model_calls": 0, "cases": receipts, "confirmed": passed, "total": len(profile.Cases), "quality_target_met": false, "whole_plan_complete": false}
	body, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(filepath.Join(directory, "corrected-verifier-recorded-replay.json"), append(body, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func plan11ReplayRecordedCase(directory string, row plan11RecordedReviewCase, host string, units map[string]string) error {
	return plan11ReplayRecordedDeliveryCase(directory, row, host, units, false)
}

func plan11ReplayRecordedDeliveryCase(directory string, row plan11RecordedReviewCase, host string, units map[string]string, summary bool) error {
	if row.Hard || row.Sample.RunStatus != "succeeded" || !row.Sample.UsageComplete || !row.Sample.CachedUsageComplete || row.Sample.ModelCalls < 1 || row.Sample.ModelCalls > 32 {
		return errors.New("recorded run/accounting/boundary gate failed")
	}
	var evidence struct {
		Reply string `json:"reply"`
	}
	raw, err := os.ReadFile(filepath.Join(directory, row.Name+"-evidence.json"))
	if err != nil || json.Unmarshal(raw, &evidence) != nil {
		return errors.New("original reply unavailable")
	}
	c, err := plan11RecordedDeliveryConclusion(directory, row, evidence.Reply, summary)
	if err != nil || c.ResourceID != host || len(c.Limitations) == 0 {
		return errors.New("original reply lacks complete exact-identity JSON")
	}
	if len(row.Reports) != 1 {
		return errors.New("requested report was not published exactly once")
	}
	texts := []string{evidence.Reply}
	for _, report := range row.Reports {
		body, err := os.ReadFile(filepath.Join(directory, "plan11-report-"+report.ID+".md"))
		if err != nil || len(body) != report.Bytes || fmt.Sprintf("%x", sha256.Sum256(body)) != report.Hash {
			return errors.New("immutable report original/hash unavailable")
		}
		if err = plan11MatchReportConclusion(body, c); err != nil {
			return err
		}
		texts = append(texts, string(body))
	}
	matched := false
	for _, file := range row.Queries {
		if !strings.Contains(file.Query, c.Metric) || file.FileID == nil {
			continue
		}
		body, err := os.ReadFile(filepath.Join(directory, "comparison-original-"+file.ID+".json"))
		if err != nil || len(body) != file.Bytes || fmt.Sprintf("%x", sha256.Sum256(body)) != file.Hash {
			return errors.New("sealed query original/hash unavailable")
		}
		points, err := plan11ComparisonPointEvidence(body, host)
		if err == nil {
			err = plan11MatchPointConclusion(c, points)
		}
		values, valueErr := plan11OriginalValueEvidence(body, c.Metric)
		if err == nil {
			err = valueErr
		}
		if err == nil {
			err = plan11MatchNumericConclusion(c, units[c.Metric], values)
		}
		path := "/workspace/queries/metrics/" + file.ID + ".json"
		if err == nil && (!plan11ReadActualQuery(string(row.Calls), path) || !plan11ComputedStatistics(string(row.Calls), path)) {
			err = errors.New("original file analysis process incomplete")
		}
		if err != nil {
			continue
		}
		if row.Question != "dashboard" {
			if err = plan11VerifyExplicitPointClaims(body, c, texts...); err != nil {
				return err
			}
		}
		matched = true
	}
	if !matched {
		return errors.New("no complete original query matches requested numerical evidence and display policy")
	}
	if row.Question == "dashboard" {
		if len(c.Dashboards) != len(row.Jobs) {
			return errors.New("structured facts omit an executed dashboard job")
		}
		for _, job := range row.Jobs {
			var claim *plan11DashboardClaim
			for i := range c.Dashboards {
				if c.Dashboards[i].JobID == job.ID.String() {
					claim = &c.Dashboards[i]
				}
			}
			if job.Manifest == nil || claim == nil || claim.AttemptID != job.Manifest.AttemptID.String() {
				return errors.New("dashboard attempt evidence mismatch")
			}
			files := 0
			for _, file := range job.Files {
				if file.Kind != "data" {
					continue
				}
				files++
				body, err := os.ReadFile(filepath.Join(directory, "panel-original-"+file.ID.String()+".json"))
				if err != nil || int64(len(body)) != file.Bytes || fmt.Sprintf("%x", sha256.Sum256(body)) != file.Hash {
					return errors.New("original panel hash mismatch")
				}
				found := false
				for _, panel := range claim.Panels {
					if panel.PanelID == file.PanelID && panel.TargetID == file.TargetID {
						unit := ""
						for _, definition := range job.Manifest.Definition.Panels {
							if definition.ID == file.PanelID {
								unit = definition.Unit
							}
						}
						if err := plan11MatchPanelClaim(panel, unit, file.Hash, body); err != nil {
							return err
						}
						found = true
					}
				}
				if !found {
					return errors.New("panel facts missing")
				}
			}
			if files != len(claim.Panels) {
				return errors.New("extra or omitted panel facts")
			}
		}
	}
	return nil
}
