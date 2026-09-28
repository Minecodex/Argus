package dashboard

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestDisplayValidationAndPublicationDiff(t *testing.T) {
	number := func(v float64) *float64 { return &v }
	for _, tc := range []struct {
		name   string
		change func(*Panel)
	}{
		{"reversed range", func(p *Panel) { p.Display = &DisplayOptions{Min: number(2), Max: number(1)} }},
		{"nonfinite range", func(p *Panel) { p.Display = &DisplayOptions{Min: number(math.Inf(1))} }},
		{"invalid reducer", func(p *Panel) { p.Type = "stat"; p.Display = &DisplayOptions{Reducer: "increase"} }},
		{"reducer on timeline", func(p *Panel) { p.Display = &DisplayOptions{Reducer: "mean"} }},
		{"style on stat", func(p *Panel) { p.Type = "stat"; p.Display = &DisplayOptions{Stack: true} }},
		{"bar smoothing", func(p *Panel) { p.Display = &DisplayOptions{DrawStyle: "bar", Smooth: true} }},
		{"duplicate threshold", func(p *Panel) { p.Thresholds = []Threshold{{Value: 1, Tone: "warning"}, {Value: 1, Tone: "danger"}} }},
		{"unknown tone", func(p *Panel) { p.Thresholds = []Threshold{{Value: 1, Tone: "red"}} }},
		{"nonfinite threshold", func(p *Panel) { p.Thresholds = []Threshold{{Value: math.NaN(), Tone: "danger"}} }},
		{"invalid decimals", func(p *Panel) { p.Decimals = 9 }},
		{"histogram thresholds", func(p *Panel) { p.Type = "histogram"; p.Thresholds = []Threshold{{Value: 1, Tone: "danger"}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := EmptySpec()
			s.Panels = []Panel{validPanel()}
			tc.change(&s.Panels[0])
			if Validate(s).Valid {
				t.Fatal("invalid presentation was accepted")
			}
		})
	}
	before, after := EmptySpec(), EmptySpec()
	before.Panels, after.Panels = []Panel{validPanel()}, []Panel{validPanel()}
	after.Panels[0].Display = &DisplayOptions{Min: number(0), Max: number(1), DrawStyle: "area", Stack: true, Smooth: true}
	after.Panels[0].Thresholds = []Threshold{{Value: .8, Tone: "danger"}}
	if report := Validate(after); !report.Valid {
		t.Fatal(report.Issues)
	}
	encoded, _ := json.Marshal(after)
	restored, err := DecodeSpec(encoded)
	if err != nil || restored.Panels[0].Display.Min == nil || *restored.Panels[0].Display.Min != 0 {
		t.Fatal("explicit zero bound lost", err)
	}
	diff, _ := json.Marshal(publicationDiff(before, after))
	if !strings.Contains(string(diff), "display") || !strings.Contains(string(diff), "thresholds") {
		t.Fatal("presentation missing from review", string(diff))
	}
	old, _ := CompileTarget(before.Panels[0].Targets[0])
	next, _ := CompileTarget(after.Panels[0].Targets[0])
	if old.Hash != next.Hash {
		t.Fatal("display edit modified the query")
	}
	for _, chart := range []string{"stat", "gauge", "bar_gauge", "bar", "pie", "table"} {
		for _, reducer := range []string{"last", "min", "max", "mean", "sum"} {
			after.Panels[0] = validPanel()
			after.Panels[0].Type = chart
			after.Panels[0].Display = &DisplayOptions{Reducer: reducer}
			if report := Validate(after); !report.Valid {
				t.Fatal(chart, reducer, report.Issues)
			}
		}
	}
}
