package argusdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOnlyDashboardEvidenceUsesActualSeriesArrayAndRejectsUnscopedData(t *testing.T) {
	decode := func(body string) map[string]any {
		t.Helper()
		var value map[string]any
		if err := json.Unmarshal([]byte(body), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	valid := `{"resources":[{"type":"host","id":"host"}],"panels":[{"targets":[{"status":"success","result_type":"matrix","data":[{"metric":{"argus_resource_id":"host"},"values":[[1,"2"],[2,"3"]]}]}]}]}`
	if count, err := plan11ReadOnlyDashboardSeries(decode(valid), "host"); err != nil || count != 1 {
		t.Fatal("actual target series array was mistaken for a PromQL envelope", count, err)
	}
	for _, body := range []string{
		`{"resources":[],"panels":[]}`,
		`{"resources":[{"type":"host","id":"other"}],"panels":[]}`,
		`{"resources":[{"type":"host","id":"host"}],"panels":[{"targets":[{"status":"success","result_type":"matrix","data":{"result":[]}}]}]}`,
		`{"resources":[{"type":"host","id":"host"}],"panels":[{"targets":[{"status":"success","result_type":"matrix","data":[{"metric":{"argus_resource_id":"other"},"values":[[1,"2"]]}]}]}]}`,
		`{"resources":[{"type":"host","id":"host"}],"panels":[{"targets":[{"status":"success","result_type":"matrix","data":[{"metric":{},"values":[[1,"NaN"]]}]}]}]}`,
	} {
		if _, err := plan11ReadOnlyDashboardSeries(decode(body), "host"); err == nil {
			t.Fatal("invalid or unscoped numeric evidence passed", body)
		}
	}
}

func TestRecordedReadOnlyDashboardArrayContract(t *testing.T) {
	directory := os.Getenv("ARGUS_PLAN11_READER_RESPONSE_REPLAY_DIR")
	if directory == "" {
		t.Skip("requires an explicitly selected owned real dashboard execution receipt")
	}
	body, err := os.ReadFile(filepath.Join(directory, "host-dashboard-execution.json"))
	if err != nil {
		t.Fatal(err)
	}
	var execution map[string]any
	if json.Unmarshal(body, &execution) != nil {
		t.Fatal("original dashboard response unavailable")
	}
	resources := execution["resources"].([]any)
	host := resources[0].(map[string]any)["id"].(string)
	series, err := plan11ReadOnlyDashboardSeries(execution, host)
	if err != nil || series < 2 {
		t.Fatal("real dashboard target contract rejected", series, err)
	}
	t.Logf("Actual stored API response: %d observed series; prior R41 reader outcome remains failed/unverified", series)
}
