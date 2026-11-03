package argusdev

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
)

type plan11ValueEvidence struct {
	Min, Max float64
	AllIdle  bool
}

// Raw per-integration questions retain units and values. A dimensionless
// response code/count must not become a percentage merely because its unit is 1.
func plan11MatchRawTargetConclusion(c plan11Conclusion, unit string, raw plan11ValueEvidence) error {
	close := func(value *float64, expected float64) bool {
		return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) && math.Abs(*value-expected) <= math.Max(1e-12, math.Abs(expected)*1e-9)
	}
	if plan11DisplayUnit(c.Unit) != plan11DisplayUnit(unit) || plan11DisplayUnit(c.DisplayUnit) != plan11DisplayUnit(unit) || c.Transform != "identity" || !close(c.ValueMin, raw.Min) || !close(c.ValueMax, raw.Max) || !close(c.DisplayMin, raw.Min) || !close(c.DisplayMax, raw.Max) {
		return errors.New("raw integration evidence changed the original unit or values")
	}
	return nil
}

// The evaluation asks for a direct source metric. Do not assign its catalog
// unit to arbitrary transformed expressions or combine different metric units.
func plan11OriginalValueEvidence(body []byte, metric string) (plan11ValueEvidence, error) {
	var result struct {
		Data struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Values [][]any           `json:"values"`
				Value  []any             `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	out := plan11ValueEvidence{AllIdle: true}
	if json.Unmarshal(body, &result) != nil {
		return out, errors.New("invalid numeric result")
	}
	n := 0
	for _, row := range result.Data.Result {
		if row.Metric["__name__"] != metric {
			return out, errors.New("evaluation requires a direct original metric with its catalog unit")
		}
		out.AllIdle = out.AllIdle && row.Metric["state"] == "idle"
		points := row.Values
		if len(row.Value) != 0 {
			points = append(points, row.Value)
		}
		for _, point := range points {
			if len(point) != 2 {
				return out, errors.New("invalid numeric point")
			}
			text, ok := point[1].(string)
			value, err := strconv.ParseFloat(text, 64)
			if !ok || err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return out, errors.New("non-finite or non-numeric evidence needs separate analysis")
			}
			if n == 0 || value < out.Min {
				out.Min = value
			}
			if n == 0 || value > out.Max {
				out.Max = value
			}
			n++
		}
	}
	if n == 0 {
		return out, errors.New("numeric evidence is empty")
	}
	return out, nil
}

func plan11MatchNumericConclusion(c plan11Conclusion, unit string, raw plan11ValueEvidence) error {
	close := func(actual *float64, expected float64) bool {
		return actual != nil && !math.IsNaN(*actual) && !math.IsInf(*actual, 0) && math.Abs(*actual-expected) <= math.Max(1e-12, math.Abs(expected)*1e-9)
	}
	if plan11DisplayUnit(c.Unit) != plan11DisplayUnit(unit) || !close(c.ValueMin, raw.Min) || !close(c.ValueMax, raw.Max) {
		return errors.New("reported original unit or value extrema differ from sealed query values")
	}
	unit = plan11DisplayUnit(unit)
	min, max, display := raw.Min, raw.Max, unit
	// This is the Host fixture's declared display policy, not production
	// metric inference. Counts/load/time with unit 1 remain dimensionless.
	ratio := c.Metric == "system_cpu_utilization" || c.Metric == "system_memory_utilization"
	switch plan11DisplayTransform(c.Transform) {
	case "identity":
		if unit == "By" || unit == "1" && ratio {
			return errors.New("evaluation requires explicit byte or ratio display conversion")
		}
	case "bytes_to_gib":
		if unit != "By" {
			return errors.New("GiB conversion requires original byte units")
		}
		min, max, display = min/(1<<30), max/(1<<30), "GiB"
	case "percent":
		if unit != "1" || !ratio {
			return errors.New("percent conversion requires a ratio")
		}
		min, max, display = min*100, max*100, "%"
	case "invert_percent":
		if unit != "1" || c.Metric != "system_cpu_utilization" || !raw.AllIdle || min < 0 || max > 1 {
			return errors.New("CPU busy conversion requires exclusively idle ratio series")
		}
		min, max, display = (1-max)*100, (1-min)*100, "%"
	default:
		return errors.New("missing or unknown display conversion")
	}
	if plan11DisplayUnit(c.DisplayUnit) != display || !close(c.DisplayMin, min) || !close(c.DisplayMax, max) {
		return errors.New("reported display conversion differs from deterministic unit arithmetic")
	}
	return nil
}

// Only observed, unambiguous spellings of the same arithmetic are aliases.
// No expression evaluation or approximate extrema matching is introduced.
func plan11DisplayTransform(value string) string {
	switch value {
	case "percent (value × 100)", "percent (x100)", "identity_x100_percent":
		return "percent"
	case "value / 2**30", "bytes_to_gib (raw / 2**30)":
		return "bytes_to_gib"
	default:
		return value
	}
}

func plan11DisplayUnit(unit string) string {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "by", "byte", "bytes", "{byte}", "{bytes}":
		// Explicit observed legacy/OTel spellings only; no generic brace
		// stripping, metric-name guessing or binary/decimal scale coercion.
		return "By"
	case "%", "percent", "percentage":
		return "%"
	case "gib", "gibibyte", "gibibytes":
		return "GiB"
	default:
		return unit
	}
}

func plan11ComputedStatistics(raw, path string) bool {
	var calls []struct {
		plan11ProcessOutcome
		Tool, Status string
		Input        struct{ Command string }
	}
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return false
	}
	for _, call := range calls {
		command := call.Input.Command
		operations := strings.ToLower(command)
		computed := strings.Contains(operations, "argus-metric-evidence") || strings.Contains(operations, "argus-evidence-report") || strings.Contains(operations, "min") && strings.Contains(operations, "max")
		if call.Tool == "bash" && call.Status == "succeeded" && call.success() && plan11CommandReferencesFile(command, path) && computed {
			return true
		}
	}
	return false
}
