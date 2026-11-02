package argusdev

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlan11DeliveredReportCannotHideStaleFactsBehindCorrectChat(t *testing.T) {
	n := 3
	min, max, displayMin, displayMax := 0.78, 0.79, 78.0, 79.0
	expected := plan11Conclusion{ResourceID: "host", Metric: "system_memory_utilization", PointCount: &n, FirstPointAt: "2026-10-30T00:30:16Z", LastPointAt: "2026-10-30T00:32:16Z", Limitations: []string{"evaluation points do not prove continuous raw coverage"}, Unit: "1", ValueMin: &min, ValueMax: &max, DisplayUnit: "%", Transform: "percent", DisplayMin: &displayMin, DisplayMax: &displayMax}
	encode := func(value plan11Conclusion) []byte {
		raw, _ := json.Marshal(value)
		return []byte("# Host report\n```json\n" + string(raw) + "\n```\n")
	}
	if err := plan11MatchReportConclusion(encode(expected), expected); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*plan11Conclusion){
		func(c *plan11Conclusion) { c.FirstPointAt = "2026-10-30T00:02:16Z" },
		func(c *plan11Conclusion) { c.ResourceID = "other" },
		func(c *plan11Conclusion) { c.Metric = "system_cpu_utilization" },
		func(c *plan11Conclusion) { count := 31; c.PointCount = &count },
		func(c *plan11Conclusion) { c.Limitations = nil },
	} {
		actual := expected
		change(&actual)
		if plan11MatchReportConclusion(encode(actual), expected) == nil {
			t.Fatal("report facts escaped the immutable-query conclusion")
		}
	}
	for _, body := range []string{"# Report without evidence", "```json\n{}", strings.ReplaceAll(string(encode(expected)), "```json", "```")} {
		if plan11MatchReportConclusion([]byte(body), expected) == nil {
			t.Fatal("report without a complete evidence summary passed")
		}
	}
}
