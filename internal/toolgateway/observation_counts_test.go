package toolgateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogPageCountsSeparateVariantsNamesAndSourceVersions(t *testing.T) {
	var data map[string]any
	if err := json.Unmarshal([]byte(`{"complete":false,"has_more":true,"metrics":[
 {"name":"same","type":"gauge","source_keys":["11111111-1111-4111-8111-111111111111:1"]},
 {"name":"same","type":"counter","source_keys":["11111111-1111-4111-8111-111111111111:2"]},
 {"name":"other","source_keys":["22222222-2222-4222-8222-222222222222:1","not-a-source"]}],
 "sources":[{"source_key":"11111111-1111-4111-8111-111111111111:1"}],"fields":[{}],"values":[{},{}]}`), &data); err != nil {
		t.Fatal(err)
	}
	counts := observationPageCounts(data)
	for key, expected := range map[string]int{"metric_entries": 3, "distinct_metric_names": 2, "source_entries": 1, "distinct_source_ids": 2, "distinct_source_versions": 3, "field_entries": 1, "value_entries": 2} {
		if counts[key] != expected {
			t.Fatalf("%s=%v, want %d", key, counts[key], expected)
		}
	}
	if counts["scope"] != "returned_page" || data["complete"] != false || data["has_more"] != true {
		t.Fatal("page count widened completeness or changed original pagination")
	}
}

func TestRecordedSignalCatalogCountsUseActualReturnedArrays(t *testing.T) {
	directory := os.Getenv("ARGUS_PLAN11_SIGNAL_REVIEW_DIR")
	if directory == "" {
		t.Skip("requires explicit archived signal evidence")
	}
	for _, kind := range []string{"availability-zh", "semantics-zh"} {
		raw, err := os.ReadFile(filepath.Join(directory, "plan11-signal-"+kind+"-tools.json"))
		if err != nil {
			t.Fatal(err)
		}
		var calls []struct {
			Tool, Status string
			Input        struct{ Category, Name string }
			Projection   struct{ Summary map[string]any }
		}
		if json.Unmarshal(raw, &calls) != nil {
			t.Fatal("invalid archived trace")
		}
		found := false
		for _, call := range calls {
			if call.Tool != "tool.invoke" || call.Status != "succeeded" || call.Input.Category != "observe" || call.Input.Name != "catalog.query" {
				continue
			}
			counts := observationPageCounts(call.Projection.Summary)
			if kind == "availability-zh" && counts["distinct_metric_names"] == 22 {
				found = counts["metric_entries"] == 65 && counts["distinct_source_ids"] == 1 && counts["distinct_source_versions"] == 3
			}
			if kind == "semantics-zh" {
				found = counts["metric_entries"] == 8 && counts["distinct_metric_names"] == 6 && counts["distinct_source_ids"] == 1
			}
		}
		if !found {
			t.Fatal("actual returned catalog count not independently reproduced", kind)
		}
	}
}
