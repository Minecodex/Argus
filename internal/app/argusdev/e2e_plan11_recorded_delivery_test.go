package argusdev

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func plan11RecordedDeliveryConclusion(directory string, row plan11RecordedReviewCase, reply string, summary bool) (plan11Conclusion, error) {
	if !summary {
		return plan11ParseConclusion(reply)
	}
	reports := append([]plan11ReportFile(nil), row.Reports...)
	for i := range reports {
		body, err := os.ReadFile(filepath.Join(directory, "plan11-report-"+reports[i].ID+".md"))
		if err != nil {
			return plan11Conclusion{}, err
		}
		reports[i].Body = body
	}
	return plan11DeliveryConclusion(reply, reports, true)
}

func TestRecordedR44DeliveryFactsAndProcessReceiptsWithoutRewritingResults(t *testing.T) {
	directory := os.Getenv("ARGUS_PLAN11_R44_REVIEW_DIR")
	if directory == "" {
		t.Skip("requires explicitly selected immutable owned R44 cohort")
	}
	var profile struct {
		HostID   string                     `json:"host_id"`
		Units    map[string]string          `json:"reference_catalog_units"`
		Cases    []plan11RecordedReviewCase `json:"cases"`
		Contract string                     `json:"delivery_contract"`
	}
	body, err := os.ReadFile(filepath.Join(directory, "plan11-profile-comparison.json"))
	if err != nil || json.Unmarshal(body, &profile) != nil || len(profile.Cases) != 18 || profile.Contract != "delivery_summary_v2" {
		t.Fatal("complete original v2 cohort unavailable", err)
	}
	originalPassed, passed := 0, 0
	records := []map[string]any{}
	for _, row := range profile.Cases {
		err := plan11ReplayRecordedDeliveryCase(directory, row, profile.HostID, profile.Units, true)
		if err != nil {
			t.Errorf("original evidence/process gate failed: %s: %v", row.Name, err)
		} else {
			passed++
		}
		if row.Sample.Passed {
			originalPassed++
		}
		records = append(records, map[string]any{"case": row.Name, "original_machine_passed": row.Sample.Passed, "original_facts_and_process_gates_passed": err == nil, "error": fmt.Sprint(err)})
	}
	if passed != 18 || originalPassed != 17 {
		t.Fatal("original classification or complete facts cohort changed", originalPassed, passed)
	}
	out := map[string]any{"scope": "Corrected literal cwd process receipt, sealed original metric/report/panel and display facts; independent prose review remains separate and retains dashboard-en-2 semantic failure", "original_results_rewritten": false, "extra_model_calls": 0, "original_machine_passed": originalPassed, "facts_and_process_passed": passed, "full_independent_quality": "17/18 per expanded-quality-review.json, not18", "cases": records}
	encoded, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(filepath.Join(directory, "corrected-path-receipt-recorded-replay.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
}
