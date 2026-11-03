package argusdev

import (
	"encoding/json"
	"testing"
)

func TestPlan11NumericOracleRejectsWrongExtremaAndDecimalGBLabeledGiB(t *testing.T) {
	body := []byte(`{"data":{"result":[{"metric":{"__name__":"system_memory_usage"},"values":[[1,"13010493440"],[2,"13388550144"],[3,"12972355584"]]}]}}`)
	values, err := plan11OriginalValueEvidence(body, "system_memory_usage")
	if err != nil || values.Min != 12972355584 || values.Max != 13388550144 {
		t.Fatal("raw extrema were not independently computed", values, err)
	}
	min, max := values.Min, values.Max
	dmin, dmax := min/(1<<30), max/(1<<30)
	c := plan11Conclusion{Unit: "By", ValueMin: &min, ValueMax: &max, DisplayUnit: "GiB", Transform: "bytes_to_gib", DisplayMin: &dmin, DisplayMax: &dmax}
	if err := plan11MatchNumericConclusion(c, "By", values); err != nil {
		t.Fatal(err)
	}
	wrongMin := 13010493440.0
	wrong := c
	wrong.ValueMin = &wrongMin
	if plan11MatchNumericConclusion(wrong, "By", values) == nil {
		t.Fatal("first value passed as the minimum")
	}
	wrong = c
	decimalMin, decimalMax := min/1e9, max/1e9
	wrong.DisplayMin, wrong.DisplayMax = &decimalMin, &decimalMax
	if plan11MatchNumericConclusion(wrong, "By", values) == nil {
		t.Fatal("decimal GB passed as binary GiB")
	}
	wrong = c
	wrong.Unit = "1"
	if plan11MatchNumericConclusion(wrong, "By", values) == nil {
		t.Fatal("invented original unit passed")
	}
	if _, err := plan11OriginalValueEvidence(body, "other_metric"); err == nil {
		t.Fatal("different metric values passed")
	}
}

func TestPlan11NumericOracleInvertsPercentBounds(t *testing.T) {
	min, max, dmin, dmax := 0.89, 0.96, 4.0, 11.0
	c := plan11Conclusion{Metric: "system_cpu_utilization", Unit: "1", ValueMin: &min, ValueMax: &max, DisplayUnit: "%", Transform: "invert_percent", DisplayMin: &dmin, DisplayMax: &dmax}
	if err := plan11MatchNumericConclusion(c, "1", plan11ValueEvidence{Min: min, Max: max, AllIdle: true}); err != nil {
		t.Fatal(err)
	}
	c.DisplayUnit = "percent"
	if err := plan11MatchNumericConclusion(c, "1", plan11ValueEvidence{Min: min, Max: max, AllIdle: true}); err != nil {
		t.Fatal("equivalent percent display unit was rejected", err)
	}
	if plan11MatchNumericConclusion(c, "1", plan11ValueEvidence{Min: min, Max: max}) == nil {
		t.Fatal("mixed CPU states were inverted as idle")
	}
	c.DisplayMin, c.DisplayMax = &dmax, &dmin
	if plan11MatchNumericConclusion(c, "1", plan11ValueEvidence{Min: min, Max: max, AllIdle: true}) == nil {
		t.Fatal("inverse conversion did not swap extrema")
	}
	for _, script := range []string{"cat /workspace/query.json", "date"} {
		raw, _ := json.Marshal([]any{map[string]any{"tool": "bash", "status": "succeeded", "input": map[string]any{"command": script}}})
		if plan11ComputedStatistics(string(raw), "/workspace/query.json") {
			t.Fatal("non-statistical command passed as calculation")
		}
	}
}

func TestPlan11HostDisplayUsesMetricSemanticsAndEquivalentArithmetic(t *testing.T) {
	min, max := 2.0, 8.0
	c := plan11Conclusion{Metric: "system_cpu_logical_count", Unit: "1", ValueMin: &min, ValueMax: &max, DisplayUnit: "1", Transform: "identity", DisplayMin: &min, DisplayMax: &max}
	raw := plan11ValueEvidence{Min: min, Max: max}
	if err := plan11MatchNumericConclusion(c, "1", raw); err != nil {
		t.Fatal("unit 1 count was falsely required to become a percentage", err)
	}
	dmin, dmax := min*100, max*100
	c.Transform, c.DisplayUnit, c.DisplayMin, c.DisplayMax = "percent", "%", &dmin, &dmax
	if plan11MatchNumericConclusion(c, "1", raw) == nil {
		t.Fatal("unit 1 count was accepted as a ratio")
	}
	c.Metric = "system_memory_utilization"
	for _, spelling := range []string{"percent", "percent (value × 100)", "percent (x100)", "identity_x100_percent"} {
		c.Transform = spelling
		if err := plan11MatchNumericConclusion(c, "1", raw); err != nil {
			t.Fatal("equivalent explicit ratio arithmetic was rejected", spelling, err)
		}
	}
	c.Transform = "percent (value × 1000)"
	if plan11MatchNumericConclusion(c, "1", raw) == nil {
		t.Fatal("unknown formula was accepted")
	}
	c.Metric, c.Transform = "system_cpu_utilization", "invert_percent"
	if plan11MatchNumericConclusion(c, "1", plan11ValueEvidence{Min: min, Max: max, AllIdle: true}) == nil {
		t.Fatal("out of range idle values were inverted")
	}
}

func TestPlan11FileAnalysisRequiresSuccessfulCompleteProcess(t *testing.T) {
	for _, tc := range []struct {
		outcome string
		want    bool
	}{
		{`"exit_code":0,"partial":false`, true},
		{`"exit_code":1,"partial":false`, false},
		{`"exit_code":0,"partial":true`, false},
		{`"exit_code":null,"partial":false`, false},
		{`"exit_code":0,"partial":null`, false},
	} {
		for _, command := range []string{"argus-metric-evidence", "argus-evidence-report"} {
			raw := `[{"tool":"bash","status":"succeeded",` + tc.outcome + `,"input":{"command":"` + command + ` queries/metrics/query.json --unit 1"}}]`
			if plan11ReadActualQuery(raw, "/workspace/queries/metrics/query.json") != tc.want || plan11ComputedStatistics(raw, "/workspace/queries/metrics/query.json") != tc.want {
				t.Fatal("process outcome was not enforced", tc, command)
			}
		}
	}
}
