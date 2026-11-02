package argusdev

import (
	"encoding/json"
	"testing"
)

func TestModelLimitationsAcceptEquivalentProseAndLists(t *testing.T) {
	for _, body := range []string{`"Returned points do not prove continuous coverage"`, `["Returned points do not prove continuous coverage"]`} {
		var value plan11Limitations
		if err := json.Unmarshal([]byte(body), &value); err != nil || len(value) != 1 {
			t.Fatal("equivalent model prose was rejected", err, value)
		}
	}
	for _, body := range []string{`1`, `true`, `{}`, `["text",3]`} {
		var value plan11Limitations
		if json.Unmarshal([]byte(body), &value) == nil {
			t.Fatal("non-text limitations accepted", body)
		}
	}
	for _, body := range []string{`"  "`, `["", " "]`, `null`, `[]`} {
		var value plan11Limitations
		if err := json.Unmarshal([]byte(body), &value); err != nil || len(value) != 0 {
			t.Fatal("empty limitations were not kept empty", body, err)
		}
	}
}

func TestRawTargetReportDoesNotInheritHostDisplayConversion(t *testing.T) {
	count := 1
	number := 200.0
	for _, unit := range []string{"1", "By", "s"} {
		expected := plan11Conclusion{ResourceID: "target", Metric: "raw_metric", PointCount: &count, FirstPointAt: "2026-10-30T23:50:50Z", LastPointAt: "2026-10-30T23:50:50Z", Limitations: []string{"limited sample"}, Unit: unit, ValueMin: &number, ValueMax: &number, Transform: "identity", DisplayUnit: unit, DisplayMin: &number, DisplayMax: &number}
		value, _ := json.Marshal(expected)
		var report map[string]any
		_ = json.Unmarshal(value, &report)
		report["limitations"] = "limited sample"
		value, _ = json.Marshal(report)
		body := []byte("```json\n" + string(value) + "\n```")
		if err := plan11MatchReportConclusion(body, expected); err != nil {
			t.Fatal("raw identity report rejected", unit, err)
		}
		report["display_max"] = 20000.0
		value, _ = json.Marshal(report)
		if plan11MatchReportConclusion([]byte("```json\n"+string(value)+"\n```"), expected) == nil {
			t.Fatal("wrong numeric display passed", unit)
		}
	}
}
