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
