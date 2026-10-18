package argusdev

import (
	"encoding/json"
	"testing"
)

func TestMatrixOracleUsesValuesAndLatestTimestampWithoutSummingTimestamps(t *testing.T) {
	var data any
	if err := json.Unmarshal([]byte(`[{"metric":{"resource":"host"},"values":[[1800000001,"12.5"],[1800000002,"2.5"],[1800000000,"5"]]}]`), &data); err != nil {
		t.Fatal(err)
	}
	got, err := planV2MatrixStats(data)
	if err != nil || got.Samples != 3 || got.Min != 2.5 || got.Max != 12.5 || got.Last != 2.5 {
		t.Fatal(got, err)
	}
	for _, invalid := range []string{`[]`, `[{"metric":{},"values":[]}]`, `[{"metric":{},"values":[[1,"NaN"]]}]`, `[{"metric":{},"values":[[1,"+Inf"]]}]`} {
		_ = json.Unmarshal([]byte(invalid), &data)
		if _, err = planV2MatrixStats(data); err == nil {
			t.Fatal("invalid matrix accepted", invalid)
		}
	}
}

func TestMatrixOracleAcceptsEmptyOnlyWithNoDataAndNoInventedNumbers(t *testing.T) {
	for _, tc := range []struct {
		status, raw string
		valid       bool
	}{
		{"no_data", `[]`, true},
		{"success", `[]`, false},
		{"no_data", `null`, false},
		{"no_data", `[{"metric":{},"values":[[1,"2"]]}]`, false},
	} {
		var data any
		if err := json.Unmarshal([]byte(tc.raw), &data); err != nil {
			t.Fatal(err)
		}
		panel := planV2ConclusionPanel{Status: tc.status}
		err := planV2SetMatrixStats(&panel, data)
		if (err == nil) != tc.valid {
			t.Fatalf("%s %s: %v", tc.status, tc.raw, err)
		}
		if tc.valid && (panel.Samples == nil || *panel.Samples != 0 || panel.Min != nil || panel.Max != nil || panel.Last != nil) {
			t.Fatalf("empty observations acquired invented values: %+v", panel)
		}
	}
}

func TestMatrixOracleTiedLatestUsesLastPointInOriginalFileOrder(t *testing.T) {
	var data any
	if err := json.Unmarshal([]byte(`[{"metric":{"host":"a"},"values":[[2,"8"],[1,"99"]]},{"metric":{"host":"b"},"values":[[2,"3"],[0,"1"]]}]`), &data); err != nil {
		t.Fatal(err)
	}
	got, err := planV2MatrixStats(data)
	if err != nil || got.Samples != 4 || got.Min != 1 || got.Max != 99 || got.Last != 3 {
		t.Fatalf("latest sample tie changed original file order: %+v %v", got, err)
	}
}

func TestConclusionRejectsNonScalarLastValue(t *testing.T) {
	for _, value := range []string{`{"timestamp":1,"value":2}`, `[1,2]`, `"2"`} {
		reply := "```json\n{\"overall\":\"unknown\",\"panels\":[{\"last\":" + value + "}]}\n```"
		if _, err := planV2ParseConclusion(reply); err == nil {
			t.Fatalf("non-numeric last accepted: %s", value)
		}
	}
}

func TestMatrixRecordCountRemainsSeriesCount(t *testing.T) {
	for _, seriesCount := range []int{1, 2} {
		rows := make([]any, seriesCount)
		for n := range rows {
			points := make([]any, 13)
			for index := range points {
				points[index] = []any{index + 1, "5"}
			}
			rows[n] = map[string]any{"metric": map[string]any{"series": n}, "values": points}
		}
		stats, err := planV2MatrixStats(rows)
		if err != nil || planV2RecordCount(rows) != seriesCount || stats.Samples != 13*seriesCount {
			t.Fatalf("series and observation counts were conflated: records=%d stats=%+v err=%v", planV2RecordCount(rows), stats, err)
		}
	}
}
