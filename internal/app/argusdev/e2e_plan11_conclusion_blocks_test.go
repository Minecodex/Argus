package argusdev

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConclusionBlocksRequireOneConsistentPrimaryAndCompletePanelFacts(t *testing.T) {
	primary := `{"resource_id":"host","metric":"network","point_count":2,"first_point_at":"2026-10-31T00:00:00Z","last_point_at":"2026-10-31T00:01:00Z","unit":"By","value_min":1,"value_max":2,"display_unit":"GiB","display_transform":"bytes_to_gib","display_min":0.0000000009313225746154785,"display_max":0.000000001862645149230957,"limitations":["coverage unknown"]}`
	panels := `[{"job_id":"job","attempt_id":"attempt","panels":[{"panel_id":"cpu","target_id":"main","input_sha256":"seal","unit":"%","point_count":2,"first_point_at":"2026-10-31T00:00:00.000Z","last_point_at":"2026-10-31T00:01:00.000Z","value_min":1,"value_max":2}]}]`
	fence := func(body string) string { return "```json\n" + body + "\n```\n" }
	separate := fence(primary) + "dashboard_evidence\n" + fence(panels)
	claim, err := plan11ParseConclusion(separate)
	if err != nil || claim.Metric != "network" || len(claim.Dashboards) != 1 || len(claim.Dashboards[0].Panels) != 1 {
		t.Fatal("complete equivalent blocks did not combine", err)
	}
	var metric, dashboards any
	_ = json.Unmarshal([]byte(primary), &metric)
	_ = json.Unmarshal([]byte(panels), &dashboards)
	wrapped, _ := json.Marshal(map[string]any{"metric_evidence": metric, "dashboard_evidence": dashboards})
	report := fence(string(wrapped))
	if err := plan11MatchReportConclusion([]byte(report), claim); err != nil {
		t.Fatal("equivalent complete immutable report rejected", err)
	}
	if _, err := plan11ParseConclusion(separate + report); err != nil {
		t.Fatal("identical repeated facts treated as conflicting", err)
	}
	if _, err := plan11ParseConclusion(fence(`[1,2]`) + separate); err != nil {
		t.Fatal("unrelated raw-data array displaced explicit typed evidence", err)
	}
	for _, text := range []string{
		separate + fence(strings.Replace(primary, `"network"`, `"different"`, 1)),
		separate + fence(strings.Replace(panels, `"attempt"`, `"another-attempt"`, 1)),
		separate + fence(strings.Replace(panels, `"seal"`, `"different-seal"`, 1)),
		fence(primary) + fence(`[{"job_id":"job","attempt_id":"attempt","panels":[]},"missing identity"]`),
		fence(primary) + fence(`{"metric_evidence":[{},{}]}`),
		fence(primary) + fence(`{"metric_evidence":null}`),
		fence(panels),
		"- resource_id: host\n- metric: network",
		separate + fence(`{"metric":"broken"`),
	} {
		if _, err := plan11ParseConclusion(text); err == nil {
			t.Fatal("missing, contradictory or non-JSON evidence accepted", text)
		}
	}
}
