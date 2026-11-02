package telemetry

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"testing"
	"time"
)

func TestReturnedPointWindowUsesActualEvaluationTimesNotRequestedStart(t *testing.T) {
	var data any
	if err := json.Unmarshal([]byte(`[{"values":[[1793320216,"0.793"],[1793320276,"0.788"],[1793320336,"0.790"]]}]`), &data); err != nil {
		t.Fatal(err)
	}
	window := promQLReturnedPoints("matrix", data)
	if window == nil || window.Count != 3 || window.From.Format(time.RFC3339) != "2026-10-30T00:30:16Z" || window.To.Format(time.RFC3339) != "2026-10-30T00:32:16Z" || window.TimeBasis != "promql_evaluation_timestamps" {
		t.Fatalf("wrong returned-point facts: %+v", window)
	}
}
func TestReturnedPointWindowHandlesMultipleSeriesHistogramsAndEmptyData(t *testing.T) {
	for _, tc := range []struct {
		kind, raw string
		count     int64
	}{
		{"vector", `[{"value":[1.125,"2"]},{"histogram":[2.375,{"count":"3"}]}]`, 2},
		{"matrix", `[{"values":[[1.125,"2"]],"histograms":[[2.375,{"count":"3"}]]},{"values":[[2.375,"4"]]}]`, 3},
		{"matrix", `[]`, 0},
	} {
		var data any
		_ = json.Unmarshal([]byte(tc.raw), &data)
		w := promQLReturnedPoints(tc.kind, data)
		if w == nil || w.Count != tc.count {
			t.Fatal(tc, w)
		}
		if tc.count == 0 {
			if w.From != nil || w.To != nil {
				t.Fatal("empty data invented timestamps")
			}
		} else if w.From.UnixMilli() != 1125 || w.To.UnixMilli() != 2375 {
			t.Fatal("lost millisecond precision", w)
		}
	}
	if promQLReturnedPoints("vector", []any{"invalid"}) != nil {
		t.Fatal("invented metadata for an invalid result")
	}
}
func TestProtocolOutputPreservesEngineObservationFactsSeparately(t *testing.T) {
	observed := time.Date(2026, 10, 30, 0, 30, 10, 0, time.UTC)
	completed := observed.Add(time.Minute)
	out := protocolQueryOutput(queryengine.Result{Language: queryengine.LanguagePromQL, ResultType: "vector", Data: []any{map[string]any{"metric": map[string]string{}, "value": []any{float64(observed.Add(time.Second).Unix()), "1"}}}, Meta: queryengine.QueryMeta{LatestSampleAt: &observed, QueryCompletedAt: &completed, SampleTimeBasis: "observed_query_samples", IngestionStatus: "unknown"}})
	meta := out["argus_meta"].(map[string]any)
	if !meta["latest_sample_at"].(*time.Time).Equal(observed) || !meta["query_completed_at"].(*time.Time).Equal(completed) || meta["sample_time_basis"] != "observed_query_samples" || meta["returned_points"].(*returnedPointWindow).From.Equal(observed) {
		t.Fatal("observation and evaluation timestamps were conflated", meta)
	}
}
