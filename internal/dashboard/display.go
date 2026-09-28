package dashboard

import (
	"fmt"
	"math"
	"slices"
)

func validateDisplay(panel Panel, path string, issue func(string, string)) {
	if panel.Decimals < 0 || panel.Decimals > 8 {
		issue(path+".decimals", "decimal places must be between 0 and 8")
	}
	if len(panel.Unit) > 32 {
		issue(path+".unit", "unit exceeds 32 bytes")
	}
	if len(panel.Thresholds) > 16 {
		issue(path+".thresholds", "at most 16 display thresholds")
	}
	if len(panel.Thresholds) > 0 && !slices.Contains([]string{"stat", "gauge", "bar_gauge", "bar", "timeseries", "scatter", "state_timeline"}, panel.Type) {
		issue(path+".thresholds", "thresholds are not supported by this chart")
	}
	for i, threshold := range panel.Thresholds {
		p := fmt.Sprintf("%s.thresholds[%d]", path, i)
		if math.IsNaN(threshold.Value) || math.IsInf(threshold.Value, 0) || i > 0 && threshold.Value <= panel.Thresholds[i-1].Value {
			issue(p+".value", "thresholds must be finite and strictly increasing")
		}
		if !slices.Contains([]string{"info", "success", "warning", "danger"}, threshold.Tone) {
			issue(p+".tone", "unsupported threshold tone")
		}
	}
	d := panel.Display
	if d == nil {
		return
	}
	if panel.Signal == "traces" || panel.Type == "logs" || panel.Signal == "logs" && panel.Type == "table" {
		issue(path+".display", "display options require a numeric chart")
	}
	if d.Reducer != "" {
		if !slices.Contains([]string{"last", "min", "max", "mean", "sum"}, d.Reducer) {
			issue(path+".display.reducer", "unsupported sample reduction")
		}
		if !slices.Contains([]string{"stat", "gauge", "bar_gauge", "bar", "pie", "table"}, panel.Type) {
			issue(path+".display.reducer", "this chart preserves individual samples")
		}
	}
	if d.Min != nil || d.Max != nil {
		if !slices.Contains([]string{"timeseries", "gauge", "bar_gauge", "bar", "scatter", "heatmap"}, panel.Type) {
			issue(path+".display", "numeric bounds are not supported by this chart")
		}
		for name, value := range map[string]*float64{"min": d.Min, "max": d.Max} {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
				issue(path+".display."+name, "bound must be finite")
			}
		}
		if d.Min != nil && d.Max != nil && *d.Min >= *d.Max {
			issue(path+".display", "minimum must be less than maximum")
		}
	}
	if d.DrawStyle != "" && !slices.Contains([]string{"line", "area", "bar"}, d.DrawStyle) {
		issue(path+".display.draw_style", "unsupported draw style")
	}
	if (d.DrawStyle != "" || d.Stack || d.Smooth) && panel.Type != "timeseries" {
		issue(path+".display", "draw style, stacking and smoothing require a time series")
	}
	if d.DrawStyle == "bar" && d.Smooth {
		issue(path+".display.smooth", "bar time series cannot be smoothed")
	}
}
