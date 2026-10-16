package skywalking

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func previewRows(count int, resource string) []any {
	rows := []any{}
	start := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	for i := 0; i < count; i++ {
		rows = append(rows, map[string]any{"sourceId": "source", "resourceId": resource, "serviceName": "orders", "timestamp": start.Add(time.Duration(i) * time.Second).Format(time.RFC3339), "samplesPerSecond": float64(i % 19), "errorRate": float64(i%5) / 100, "durationP95Ms": float64(i % 101)})
	}
	return rows
}

func TestREDPreviewKeepsEachIdentityWindowExtremaAndNullBoundaries(t *testing.T) {
	rows := []any{}
	for _, resource := range []string{"host-one", "host-two"} {
		group := previewRows(600, resource)
		group[317].(map[string]any)["samplesPerSecond"] = float64(999)
		group[419].(map[string]any)["errorRate"] = 0.99
		group[511].(map[string]any)["durationP95Ms"] = float64(9999)
		group[200].(map[string]any)["samplesPerSecond"] = nil
		rows = append(rows, group...)
	}
	original, _ := json.Marshal(rows)
	preview := redChartPreview(rows)
	if preview["shown_rows"].(int) > presentationPointBudget || preview["shown_series"] != 2 || preview["omitted_series"] != 0 {
		t.Fatal(preview)
	}
	for _, raw := range preview["series"].([]any) {
		points := raw.(map[string]any)["points"].([]any)
		first, last := points[0].(map[string]any), points[len(points)-1].(map[string]any)
		if first["timestamp"] != "2026-10-14T10:00:00Z" || last["timestamp"] != "2026-10-14T10:09:59Z" {
			t.Fatal("time window clipped", first, last)
		}
		seen := map[string]bool{}
		for _, point := range points {
			seen[point.(map[string]any)["timestamp"].(string)] = true
		}
		for _, second := range []int{199, 200, 201, 317, 419, 511} {
			at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC).Add(time.Duration(second) * time.Second).Format(time.RFC3339)
			if !seen[at] {
				t.Fatalf("lost extremum/gap boundary at %s", at)
			}
		}
	}
	after, _ := json.Marshal(rows)
	if string(original) != string(after) {
		t.Fatal("sealed observations mutated")
	}
}

func TestREDPreviewExplicitlyOmitsUnrepresentableSeriesInsteadOfBridgingGaps(t *testing.T) {
	rows := previewRows(600, "host")
	for i, row := range rows {
		if i%2 == 0 {
			row.(map[string]any)["samplesPerSecond"] = nil
		}
	}
	preview := redChartPreview(rows)
	if preview["omitted_series"] != 1 || preview["shown_rows"] != 0 || preview["sampled"] != true {
		t.Fatal(preview)
	}
	delete(rows[0].(map[string]any), "resourceId")
	preview = redChartPreview(rows[:1])
	if preview["unrenderable_rows"] != 1 || preview["shown_rows"] != 0 {
		t.Fatal(preview)
	}
}

func TestREDPreviewBoundsManySeriesAndPreservesUnknownValues(t *testing.T) {
	rows := []any{}
	for i := 0; i < 150; i++ {
		rows = append(rows, previewRows(3, fmt.Sprint(i))...)
	}
	preview := redChartPreview(rows)
	if preview["shown_rows"].(int) > presentationPointBudget || preview["omitted_series"].(int) < 1 || preview["returned_rows"] != 450 {
		t.Fatal(preview)
	}
	row := previewRows(1, "host")[0].(map[string]any)
	row["samplesPerSecond"], row["errorRate"] = nil, nil
	snapshot := map[string]any{"rows": []any{row}}
	AttachREDPreviews(snapshot)
	first, _ := json.Marshal(snapshot)
	AttachREDPreviews(snapshot)
	second, _ := json.Marshal(snapshot)
	if string(first) != string(second) {
		t.Fatal("preview is not idempotent")
	}
	point := snapshot["chart_preview"].(map[string]any)["series"].([]any)[0].(map[string]any)["points"].([]any)[0]
	if !reflect.DeepEqual(point, row) {
		t.Fatal("preview fabricated observations", point)
	}
	if redChartPreview(nil)["shown_rows"] != 0 {
		t.Fatal("empty data should stay empty")
	}
}
