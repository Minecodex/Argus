package telemetry

import (
	"encoding/json"
	"math"
	"time"
)

// Returned points are PromQL evaluation timestamps, not a statement that raw
// measurements continuously cover this interval. LatestSampleAt remains the
// separate engine observation fact. Count includes every returned series.
type returnedPointWindow struct {
	Count     int64      `json:"count"`
	From      *time.Time `json:"from,omitempty"`
	To        *time.Time `json:"to,omitempty"`
	TimeBasis string     `json:"time_basis"`
}

func promQLReturnedPoints(kind string, data any) *returnedPointWindow {
	if kind != "vector" && kind != "matrix" && kind != "scalar" && kind != "string" {
		return nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var series []struct {
		Value      []json.RawMessage   `json:"value"`
		Values     [][]json.RawMessage `json:"values"`
		Histogram  []json.RawMessage   `json:"histogram"`
		Histograms [][]json.RawMessage `json:"histograms"`
	}
	out := &returnedPointWindow{TimeBasis: "promql_evaluation_timestamps"}
	valid := true
	add := func(point []json.RawMessage) {
		var seconds float64
		if len(point) < 2 || json.Unmarshal(point[0], &seconds) != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < -62135596800 || seconds > 253402300799.999 {
			valid = false
			return
		}
		// Prometheus result timestamps have millisecond precision. Round before
		// constructing UTC time, avoiding float64 nanosecond rounding artifacts.
		at := time.UnixMilli(int64(math.Round(seconds * 1000))).UTC()
		out.Count++
		if out.From == nil || at.Before(*out.From) {
			copy := at
			out.From = &copy
		}
		if out.To == nil || at.After(*out.To) {
			copy := at
			out.To = &copy
		}
	}
	if kind == "scalar" || kind == "string" {
		var point []json.RawMessage
		if json.Unmarshal(raw, &point) != nil {
			return nil
		}
		add(point)
		if !valid {
			return nil
		}
		return out
	}
	if json.Unmarshal(raw, &series) != nil {
		return nil
	}
	for _, row := range series {
		if kind == "vector" {
			if len(row.Value) > 0 {
				add(row.Value)
			} else {
				add(row.Histogram)
			}
		} else {
			for _, point := range row.Values {
				add(point)
			}
			for _, point := range row.Histograms {
				add(point)
			}
		}
	}
	if !valid {
		return nil
	}
	return out
}
