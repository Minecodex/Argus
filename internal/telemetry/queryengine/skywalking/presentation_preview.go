package skywalking

import (
	"encoding/json"
	"math"
	"sort"
	"time"
)

const presentationPointBudget = 200

var redMeasures = []string{"samplesPerSecond", "errorRate", "durationP95Ms"}

type redPreviewGroup struct {
	rows  []map[string]any
	times []time.Time
	keep  map[int]bool
	quota int
}

// AttachREDPreviews operates only on a private presentation copy, never the
// provider result or sealed rows. Callers identify RED envelopes using their
// frozen result type. Points are actual observations, not averaged percentiles.
func AttachREDPreviews(value any) {
	switch item := value.(type) {
	case map[string]any:
		if rows, ok := item["rows"].([]any); ok {
			item["chart_preview"] = redChartPreview(rows)
			return
		}
		for key, child := range item {
			if key != "chart_preview" {
				AttachREDPreviews(child)
			}
		}
	case []any:
		for _, child := range item {
			AttachREDPreviews(child)
		}
	}
}

func redChartPreview(rows []any) map[string]any {
	groups, positions := []*redPreviewGroup{}, map[string]int{}
	unrenderable := 0
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			unrenderable++
			continue
		}
		stamp, _ := row["timestamp"].(string)
		at, err := time.Parse(time.RFC3339Nano, stamp)
		identity := []any{row["sourceId"], row["resourceId"], row["serviceName"], row["instanceId"], row["operationName"]}
		identified := true
		for _, name := range []string{"sourceId", "resourceId", "serviceName"} {
			text, present := row[name].(string)
			identified = identified && present && text != ""
		}
		if err != nil || !identified {
			unrenderable++
			continue
		}
		key, _ := json.Marshal(identity)
		index, exists := positions[string(key)]
		if !exists {
			index = len(groups)
			positions[string(key)] = index
			groups = append(groups, &redPreviewGroup{keep: map[int]bool{}})
		}
		group := groups[index]
		group.rows, group.times = append(group.rows, row), append(group.times, at)
	}
	remaining := presentationPointBudget
	selected := []*redPreviewGroup{}
	for _, group := range groups {
		order := make([]int, len(group.rows))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(i, j int) bool { return group.times[order[i]].Before(group.times[order[j]]) })
		ordered := make([]map[string]any, len(order))
		for i, index := range order {
			ordered[i] = group.rows[index]
		}
		group.rows = ordered
		group.keep[0], group.keep[len(ordered)-1] = true, true
		for _, measure := range redMeasures {
			low, high := -1, -1
			previousValid := false
			for i, row := range ordered {
				number, valid := redNumber(row[measure])
				if i > 0 && valid != previousValid {
					group.keep[i-1], group.keep[i] = true, true
				}
				previousValid = valid
				if !valid {
					continue
				}
				if low < 0 {
					low, high = i, i
				}
				lowest, _ := redNumber(ordered[low][measure])
				highest, _ := redNumber(ordered[high][measure])
				if number < lowest {
					low = i
				}
				if number > highest {
					high = i
				}
			}
			if low >= 0 {
				group.keep[low], group.keep[high] = true, true
			}
		}
		// If too many series/gap boundaries fit no honest bounded curve, omit
		// that series explicitly rather than joining across missing observations.
		if len(group.keep) > remaining {
			continue
		}
		group.quota = len(group.keep)
		remaining -= group.quota
		selected = append(selected, group)
	}
	for remaining > 0 {
		allocated := false
		for _, group := range selected {
			if remaining > 0 && group.quota < len(group.rows) {
				group.quota++
				remaining--
				allocated = true
			}
		}
		if !allocated {
			break
		}
	}
	series, shown := []any{}, 0
	for _, group := range selected {
		for len(group.keep) < group.quota {
			indices := redIndices(group.keep)
			left, gap := 0, 0
			for i := 1; i < len(indices); i++ {
				if distance := indices[i] - indices[i-1]; distance > gap {
					left, gap = indices[i-1], distance
				}
			}
			if gap <= 1 {
				break
			}
			group.keep[left+gap/2] = true
		}
		points := []any{}
		for _, index := range redIndices(group.keep) {
			points = append(points, group.rows[index])
		}
		shown += len(points)
		series = append(series, map[string]any{"returned_points": len(group.rows), "shown_points": len(points), "points": points})
	}
	return map[string]any{"algorithm": "window-extrema/v1", "point_budget": presentationPointBudget, "returned_rows": len(rows), "shown_rows": shown, "returned_series": len(groups), "shown_series": len(selected), "omitted_series": len(groups) - len(selected), "unrenderable_rows": unrenderable, "sampled": shown < len(rows), "series": series}
}

func redIndices(keep map[int]bool) []int {
	indices := make([]int, 0, len(keep))
	for index := range keep {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	return indices
}

func redNumber(value any) (float64, bool) {
	var number float64
	switch n := value.(type) {
	case float64:
		number = n
	case int:
		number = float64(n)
	case json.Number:
		var err error
		number, err = n.Float64()
		if err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}
