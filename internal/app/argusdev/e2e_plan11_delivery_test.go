package argusdev

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSummaryDeliveryContractRequiresExplicitCandidateOnly(t *testing.T) {
	for _, row := range []struct {
		suite, phase string
		profiles     []string
		allowed      bool
	}{
		{"plan8", "plan11-compare", []string{"candidate"}, true},
		{"plan8", "plan11-compare", nil, false},
		{"plan8", "plan11-compare", []string{"candidate", "legacy"}, false},
		{"plan8", "plan11-compare", []string{"legacy"}, false},
		{"plan8", "plan11-review", []string{"candidate"}, false},
		{"plan7", "plan11-compare", []string{"candidate"}, false},
	} {
		if err := plan11ValidateDeliverySummary(true, row.suite, row.phase, row.profiles); (err == nil) != row.allowed {
			t.Errorf("unexpected delivery contract admission: %+v: %v", row, err)
		}
		if err := plan11ValidateDeliverySummary(false, row.suite, row.phase, row.profiles); err != nil {
			t.Fatal("original output contract admission changed", err)
		}
	}
	if plan11DeliveryContract(false) != "strict_json_v1" || plan11DeliveryContract(true) != "delivery_summary_v2" {
		t.Fatal("delivery contracts must remain separately identified")
	}
}

func TestSummaryDeliveryUsesSealedReportAndRejectsConflictingFinalEvidence(t *testing.T) {
	fields := map[string]any{"resource_id": uuid.NewString(), "metric": "network", "point_count": 2, "first_point_at": "2026-10-31T00:00:00Z", "last_point_at": "2026-10-31T00:01:00Z", "unit": "By", "value_min": 1, "value_max": 2, "display_unit": "By", "display_transform": "identity", "display_min": 1, "display_max": 2, "limitations": []string{"Only returned observations."}}
	raw, _ := json.Marshal(fields)
	body := []byte("# Actual report\n```json\n" + string(raw) + "\n```")
	report := plan11ReportFile{ID: uuid.NewString(), Name: "actual-report.md", Hash: fmt.Sprintf("%x", sha256.Sum256(body)), Bytes: len(body), Body: body}
	reply := "The returned observations are summarized in actual-report.md. Sampling completeness remains unknown."
	c, err := plan11DeliveryConclusion(reply, []plan11ReportFile{report}, true)
	if err != nil || c.ResourceID != fields["resource_id"] || c.PointCount == nil || *c.PointCount != 2 {
		t.Fatal("sealed report facts lost", err)
	}
	if _, err := plan11DeliveryConclusion(reply, []plan11ReportFile{report}, false); err == nil {
		t.Fatal("new contract retroactively accepted an old strict-JSON failure")
	}
	if _, err := plan11DeliveryConclusion(reply+"\n```json\n"+string(raw)+"\n```", []plan11ReportFile{report}, true); err != nil {
		t.Fatal("complete consistent optional JSON refused", err)
	}
	changed := strings.Replace(string(raw), `"point_count":2`, `"point_count":3`, 1)
	for _, bad := range []string{"", "The report will be assembled later.", "See fabricated-report.md", reply + "\n```json\n{\"resource_id\":\"missing\"}\n```", reply + "\n```\n{\"metric_evidence\":{}}\n```", reply + "\n```json\n" + changed + "\n```"} {
		if _, err := plan11DeliveryConclusion(bad, []plan11ReportFile{report}, true); err == nil {
			t.Fatal("incomplete/fabricated/conflicting final delivery accepted", bad)
		}
	}
	for _, reports := range [][]plan11ReportFile{nil, {report, report}, {{ID: report.ID, Name: report.Name, Hash: report.Hash, Bytes: report.Bytes, Body: append([]byte("altered"), body...)}}, {{Name: report.Name, Hash: report.Hash, Bytes: report.Bytes, Body: body}}} {
		if _, err := plan11DeliveryConclusion(reply, reports, true); err == nil {
			t.Fatal("missing/ambiguous/tampered delivery accepted")
		}
	}
}
