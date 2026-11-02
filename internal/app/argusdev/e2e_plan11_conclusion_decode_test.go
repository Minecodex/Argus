package argusdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConclusionEnvelopePreservesFactsAndRejectsConflicts(t *testing.T) {
	flat := []byte(`{"resource_id":"host","metric":"network","point_count":2,"first_point_at":"2026-10-31T00:00:00Z","last_point_at":"2026-10-31T00:01:00Z","unit":"By","value_min":1,"value_max":2,"display_unit":"GiB","display_transform":"bytes_to_gib","display_min":0.0000000009313225746154785,"display_max":0.000000001862645149230957,"limitations":["sparse"],"dashboard_evidence":[{"job_id":"job","attempt_id":"attempt","panels":[]}]}`)
	var fields map[string]any
	if json.Unmarshal(flat, &fields) != nil {
		t.Fatal("fixture")
	}
	dashboards := fields["dashboard_evidence"]
	delete(fields, "dashboard_evidence")
	nested, _ := json.Marshal(map[string]any{"metric_evidence": fields, "dashboard_evidence": dashboards})
	a, err := plan11DecodeConclusion(flat)
	if err != nil {
		t.Fatal(err)
	}
	b, err := plan11DecodeConclusion(nested)
	if err != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("equivalent envelope changed facts", err)
	}
	singleton, _ := json.Marshal(map[string]any{"metric_evidence": []any{fields}, "dashboard_evidence": dashboards})
	c, err := plan11DecodeConclusion(singleton)
	if err != nil || !reflect.DeepEqual(a, c) {
		t.Fatal("unambiguous singleton evidence changed facts", err)
	}
	for _, raw := range []string{`{"metric_evidence":[null]}`, `{"metric_evidence":[{},{}]}`, `{"metric_evidence":["facts"]}`} {
		if _, err := plan11DecodeConclusion([]byte(raw)); err == nil {
			t.Fatal("ambiguous or invalid metric array accepted", raw)
		}
	}
	for _, raw := range []string{`{"metric_evidence":null}`, `{"metric":"other","metric_evidence":{"metric":"network"}}`, `{"metric_evidence":[],"dashboard_evidence":[]}`, `{"metric_evidence":{"dashboard_evidence":[{"job_id":"inner"}]},"dashboard_evidence":[{"job_id":"outer"}]}`} {
		if _, err := plan11DecodeConclusion([]byte(raw)); err == nil {
			t.Fatal("ambiguous or invalid envelope accepted", raw)
		}
	}
	if err := plan11MatchReportConclusion([]byte("```json\n"+string(nested)+"\n```"), a); err != nil {
		t.Fatal("report rejects equivalent evidence envelope", err)
	}
}

func TestRecordedComparisonConclusionEnvelopeReview(t *testing.T) {
	directory := os.Getenv("ARGUS_PLAN11_CONCLUSION_REVIEW_DIR")
	if directory == "" {
		t.Skip("requires explicitly selected owned comparison evidence")
	}
	files, err := filepath.Glob(filepath.Join(directory, "plan11-compare-candidate-dashboard-*-evidence.json"))
	if err != nil || len(files) == 0 {
		t.Fatal("recorded evidence unavailable", err)
	}
	for _, path := range files {
		var record struct {
			Reply string `json:"reply"`
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if json.Unmarshal(raw, &record) != nil {
			t.Fatal("invalid recorded case")
		}
		claim, err := plan11ParseConclusion(record.Reply)
		if err != nil || claim.ResourceID == "" || len(claim.Dashboards) == 0 {
			t.Fatal("recorded reply did not decode", path, err)
		}
		reports, err := filepath.Glob(filepath.Join(directory, "plan11-report-*.md"))
		if err != nil {
			t.Fatal(err)
		}
		matched := 0
		for _, report := range reports {
			body, err := os.ReadFile(report)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), claim.Dashboards[0].JobID) {
				continue
			}
			if err = plan11MatchReportConclusion(body, claim); err != nil {
				t.Fatal("actual delivered report differs from decoded evidence", report, err)
			}
			matched++
		}
		if matched != 1 {
			t.Fatal("expected one matching immutable report", path, matched)
		}
		t.Logf("recorded reply/report decoded consistently: %s", filepath.Base(path))
	}
}
