package argusdev

import (
	"fmt"
	"math"
	"strconv"
)

type planV2MatrixSummary struct {
	Samples int     `json:"samples"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Last    float64 `json:"last"`
}

// A range matrix is not an instant-vector sum. Validate and summarize actual
// sample values, keeping timestamps out of the numeric calculation.
func planV2SetMatrixStats(panel *planV2ConclusionPanel, data any) error {
	if panel.Status == "no_data" {
		rows, ok := data.([]any)
		if !ok || len(rows) != 0 {
			return fmt.Errorf("no_data matrix contains unexpected data")
		}
		zero := 0
		panel.Samples = &zero
		return nil
	}
	stats, err := planV2MatrixStats(data)
	if err != nil {
		return err
	}
	panel.Samples, panel.Min, panel.Max, panel.Last = &stats.Samples, &stats.Min, &stats.Max, &stats.Last
	return nil
}

func planV2MatrixStats(data any) (planV2MatrixSummary, error) {
	result := planV2MatrixSummary{}
	rows, ok := data.([]any)
	if !ok || len(rows) == 0 {
		return result, fmt.Errorf("matrix file has no series")
	}
	lastTime := math.Inf(-1)
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		points, _ := row["values"].([]any)
		if _, ok := row["metric"].(map[string]any); !ok || len(points) == 0 {
			return result, fmt.Errorf("invalid matrix series shape")
		}
		for _, rawPoint := range points {
			pair, _ := rawPoint.([]any)
			if len(pair) != 2 {
				return result, fmt.Errorf("invalid matrix sample shape")
			}
			timestamp, te := strconv.ParseFloat(fmt.Sprint(pair[0]), 64)
			value, ve := strconv.ParseFloat(fmt.Sprint(pair[1]), 64)
			if te != nil || ve != nil || math.IsNaN(timestamp) || math.IsInf(timestamp, 0) || math.IsNaN(value) || math.IsInf(value, 0) {
				return result, fmt.Errorf("nonfinite matrix file sample")
			}
			if result.Samples == 0 {
				result.Min, result.Max = value, value
			}
			result.Min, result.Max = math.Min(result.Min, value), math.Max(result.Max, value)
			if timestamp >= lastTime {
				result.Last, lastTime = value, timestamp
			}
			result.Samples++
		}
	}
	return result, nil
}
