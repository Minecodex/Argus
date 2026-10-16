package presentation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/prometheus/prometheus/promql"
)

func metricWindow(t *testing.T, missing map[int]float64) map[string]any {
	t.Helper()
	points := make([]promql.FPoint, 600)
	for i := range points {
		points[i] = promql.FPoint{T: int64(i) * 1000, F: 1}
	}
	points[2].F, points[511].F = 10000, -50
	for i, value := range missing {
		points[i].F = value
	}
	raw, err := json.Marshal(promql.Matrix{{Floats: points}})
	if err != nil {
		t.Fatal(err)
	}
	var rows []any
	if err = json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	return map[string]any{"items": rows}
}

func assertMetricWindow(t *testing.T, full, view map[string]any) {
	t.Helper()
	original := full["items"].([]any)[0].(map[string]any)["values"].([]any)
	row := view["items"].([]any)[0].(map[string]any)
	points := row["values"].([]any)
	if len(points) > 200 || len(points) < 2 {
		t.Fatal("invalid point budget", len(points))
	}
	seen := map[float64]bool{}
	previous := -1.0
	for _, raw := range points {
		point := raw.([]any)
		stamp := point[0].(float64)
		if stamp <= previous || !reflect.DeepEqual(point, original[int(stamp)].([]any)) {
			t.Fatal("invented or unordered sample", point)
		}
		previous, seen[stamp] = stamp, true
	}
	if !seen[0] || !seen[599] {
		t.Fatal("lost window endpoints")
	}
	valid := func(point any) bool { v := sampleNumber(point); return !math.IsNaN(v) && !math.IsInf(v, 0) }
	for i := 1; i < len(original); i++ {
		if valid(original[i]) != valid(original[i-1]) && (!seen[float64(i)] || !seen[float64(i-1)]) {
			t.Fatal("lost missing boundary", i)
		}
	}
	stats := row["argus_statistics"].(map[string]any)
	actual := sampleStatistics(points)
	if stats["max"] != actual["max"] || stats["min"] != actual["min"] || stats["sample_count"] != float64(600) {
		t.Fatal("lost extrema or exact statistics", stats, actual)
	}
}

func TestMetricWindowPreservesFormalNonFiniteExtremaAndGaps(t *testing.T) {
	for name, missing := range map[string]map[int]float64{
		"flat": {}, "bucket_start": {1: math.NaN()}, "interior": {3: math.NaN()},
		"ends": {0: math.NaN(), 599: math.NaN()}, "infinities": {1: math.Inf(1), 3: math.Inf(-1)},
		"runs": {99: math.NaN(), 100: math.NaN(), 101: math.NaN(), 510: math.NaN()},
	} {
		t.Run(name, func(t *testing.T) {
			full := metricWindow(t, missing)
			before, _ := json.Marshal(full)
			view := ViewData(full)
			assertMetricWindow(t, full, view)
			page, _, err := PageData(pagination.Signer{Key: make([]byte, 32)}, pagination.Binding{SubjectID: "owner"}, db.GetToolPresentationRow{ID: uuid.New()}, full, "v1", map[string]any{})
			if err != nil {
				t.Fatal(err)
			}
			assertMetricWindow(t, full, page)
			assertMetricWindow(t, full, ViewData(view))
			after, _ := json.Marshal(full)
			if string(before) != string(after) {
				t.Fatal("sealed data mutated")
			}
		})
	}
}

func TestMetricWindowOmitsImpossibleGapsButKeepsExactPages(t *testing.T) {
	missing := map[int]float64{}
	for i := 1; i < 600; i += 2 {
		missing[i] = math.NaN()
	}
	full := metricWindow(t, missing)
	row := ViewData(full)["items"].([]any)[0].(map[string]any)
	if len(row["values"].([]any)) != 0 || row["argus_sample_preview"].(map[string]any)["state"] != "omitted_gap_budget" || row["argus_statistics"].(map[string]any)["max"] != float64(10000) {
		t.Fatal(row)
	}
	count, cursor := 0, ""
	p := db.GetToolPresentationRow{ID: uuid.New(), TemplateHash: "hash", CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}
	for {
		params := map[string]any{"index": float64(0), "field": "values", "limit": float64(100)}
		if cursor != "" {
			params["cursor"] = cursor
		}
		page, next, err := PageData(pagination.Signer{Key: make([]byte, 32)}, pagination.Binding{SubjectID: "owner"}, p, full, "v1", params)
		if err != nil {
			t.Fatal(err)
		}
		count += len(page["items"].([]any))
		cursor = next
		if cursor == "" {
			break
		}
	}
	if count != 600 {
		t.Fatal("exact values unavailable", count)
	}
}

func TestMetricWindowNullsAndRepeatedBudgetReduction(t *testing.T) {
	full := metricWindow(t, nil)
	points := full["items"].([]any)[0].(map[string]any)["values"].([]any)
	for _, i := range []int{1, 198, 598} {
		points[i].([]any)[1] = nil
	}
	for _, limit := range []int{200, 100, 50, 25, 10, 4} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			points = sampleWindow(points, limit)
			if len(points) > limit {
				t.Fatal("exceeded budget")
			}
			if limit >= 10 {
				assertMetricWindow(t, full, map[string]any{"items": []any{map[string]any{"values": points, "argus_statistics": map[string]any{"min": -50., "max": 10000., "sample_count": 600.}}}})
			} else if len(points) > 0 {
				t.Fatal("must omit rather than bridge gaps")
			}
		})
	}
}

func TestFrozenRawMetricMatrixKeepsExactPagesAndTargetBoundCursor(t *testing.T) {
	full := metricWindow(t, map[int]float64{1: math.NaN()})
	data := map[string]any{"panels": []any{map[string]any{"id": "panel", "targets": []any{
		map[string]any{"id": "one", "data": full["items"]},
		map[string]any{"id": "two", "data": full["items"]},
	}}}}
	signer := pagination.Signer{Key: make([]byte, 32)}
	owner := pagination.Binding{SubjectID: "owner"}
	p := db.GetToolPresentationRow{ID: uuid.New(), TemplateHash: "hash", CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}}
	original := full["items"].([]any)[0].(map[string]any)["values"].([]any)
	count, cursor := 0, ""
	for {
		params := map[string]any{"panel_id": "panel", "target_id": "one", "index": float64(0), "field": "values", "limit": float64(100)}
		if cursor != "" {
			params["cursor"] = cursor
		}
		page, next, err := PageData(signer, owner, p, data, "v1", params)
		if err != nil {
			t.Fatal(err)
		}
		rows := page["items"].([]any)
		if !reflect.DeepEqual(rows, original[count:count+len(rows)]) {
			t.Fatal("frozen samples changed")
		}
		count += len(rows)
		if next == "" {
			break
		}
		params["cursor"], params["target_id"] = next, "two"
		if _, _, err := PageData(signer, owner, p, data, "v1", params); err == nil {
			t.Fatal("cursor crossed frozen target")
		}
		cursor = next
	}
	if count != 600 {
		t.Fatal("frozen matrix samples lost", count)
	}
}
