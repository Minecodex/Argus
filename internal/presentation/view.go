package presentation

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Bounded previews retain endpoints and bucket extrema across the full window.
// Exact reductions remain attached to each series. v1 Details is unchanged.
func ViewData(value map[string]any) map[string]any {
	copy := PublicSnapshot(value)
	maxPoints := 200
	var visit func(any)
	visit = func(value any) {
		switch item := value.(type) {
		case map[string]any:
			if points, ok := item["values"].([]any); ok && len(points) > maxPoints && samplePair(points[0]) {
				if item["argus_statistics"] == nil {
					item["argus_statistics"] = sampleStatistics(points)
				}
				sampled := sampleWindow(points, maxPoints)
				item["values"] = sampled
				if item["_sample_preview_total"] == nil {
					item["_sample_preview_total"] = len(points)
				}
				state := "sampled"
				if len(sampled) == 0 {
					state = "omitted_gap_budget"
				}
				item["argus_sample_preview"] = map[string]any{"state": state, "shown": len(sampled), "total": item["_sample_preview_total"]}
				copy["_presentation_truncated"] = true
			}
			for _, child := range item {
				visit(child)
			}
		case []any:
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(copy)
	// v1's eight-level detail clamp predates nested dashboard envelopes. A
	// sealed panel -> target -> data -> result -> series is already eight
	// levels deep, before its labels and samples. Bound v2 independently.
	trimmed := false
	var bound func(any, int) any
	bound = func(value any, depth int) any {
		if depth > 32 {
			trimmed = true
			return "[detail omitted]"
		}
		switch item := value.(type) {
		case string:
			if len(item) > 8192 {
				trimmed = true
				return strings.ToValidUTF8(item[:8192], "")
			}
		case []any:
			if len(item) > 200 {
				trimmed = true
				item = item[:200]
			}
			for index, child := range item {
				item[index] = bound(child, depth+1)
			}
			return item
		case map[string]any:
			for key, child := range item {
				item[key] = bound(child, depth+1)
			}
		}
		return value
	}
	copy = bound(copy, 0).(map[string]any)
	if trimmed {
		copy["_presentation_truncated"] = true
	}
	encoded, _ := json.Marshal(copy)
	for _, limit := range []int{100, 50, 25, 10, 5, 1} {
		if len(encoded) <= 512<<10 {
			break
		}
		// Keep the complete frozen panel/target structure and parameters. Shrink
		// only data collections, with whole-window samples and exact reductions.
		maxPoints = max(4, min(100, limit*4))
		visit(copy)
		boundCollections(copy, limit)
		copy["_presentation_truncated"] = true
		encoded, _ = json.Marshal(copy)
	}
	if len(encoded) > 512<<10 {
		// A detail view is only a preview. The exact sealed rows remain in
		// result.page; do not turn the preview into a huge bridge message.
		result := map[string]any{"_presentation_truncated": true, "preview_omitted": true}
		for _, key := range []string{"view", "tool", "name", "job_id", "dashboard_id", "attempt_id", "status", "error_code", "from", "to", "timezone", "revision_number", "query_context", "resources", "observed_at"} {
			if value, ok := copy[key]; ok {
				result[key] = value
			}
		}
		return result
	}
	canonical := map[string]any{}
	_ = json.Unmarshal(encoded, &canonical)
	return canonical
}

func boundCollections(value any, limit int) {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			switch key {
			case "items", "result", "rows", "traces", "spans", "lines", "metrics":
				if rows, ok := child.([]any); ok && len(rows) > limit {
					if item["_"+key+"_preview_total"] == nil {
						item["_"+key+"_preview_total"] = len(rows)
					}
					child = rows[:limit]
					item[key] = child
				}
			}
			boundCollections(child, limit)
		}
	case []any:
		for _, child := range item {
			boundCollections(child, limit)
		}
	}
}
func samplePair(value any) bool { pair, ok := value.([]any); return ok && len(pair) == 2 }
func sampleNumber(value any) float64 {
	pair, ok := value.([]any)
	if !ok || len(pair) != 2 || pair[1] == nil {
		return math.NaN()
	}
	switch v := pair[1].(type) {
	case float64:
		return v
	case string:
		n, e := strconv.ParseFloat(v, 64)
		if e != nil {
			return math.NaN()
		}
		return n
	}
	return math.NaN()
}
func sampleStatistics(points []any) map[string]any {
	count := 0
	sum, low, high := 0., math.Inf(1), math.Inf(-1)
	var last any
	for i, point := range points {
		value := sampleNumber(point)
		if i == len(points)-1 && !math.IsNaN(value) && !math.IsInf(value, 0) {
			last = value
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		count++
		sum += value
		low = math.Min(low, value)
		high = math.Max(high, value)
	}
	result := map[string]any{"last": last, "count": count, "sample_count": len(points)}
	if count > 0 {
		result["min"], result["max"] = low, high
		if !math.IsInf(sum, 0) && !math.IsNaN(sum) {
			result["sum"], result["mean"] = sum, sum/float64(count)
		}
	}
	return result
}
