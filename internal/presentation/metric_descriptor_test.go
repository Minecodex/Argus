package presentation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func descriptorFixture() (map[string]any, map[string]any) {
	d := map[string]any{"argus_meta": map[string]any{"result_is_original_metric": true, "input_metrics_complete": true, "input_metrics_origin": "authorized_series_metadata", "input_metrics": []any{
		map[string]any{"name": "queue", "type": "gauge", "unit": "1", "source_key": "source:3"},
	}}}
	m := map[string]any{"__name__": "queue", "argus_source_id": "source"}
	return d, m
}

func TestMetricDisplayUnitRequiresVerifiedOriginalAndUniqueSource(t *testing.T) {
	data, metric := descriptorFixture()
	d, ok := metricDisplayDescriptor(data, metric)
	if !ok || d.Unit != "1" || d.SourceKey != "source:3" {
		t.Fatal("verified original metadata not presented", d, ok)
	}
	for _, change := range []func(map[string]any, map[string]any){
		func(d, m map[string]any) { d["argus_meta"].(map[string]any)["result_is_original_metric"] = false },
		func(d, m map[string]any) { d["argus_meta"].(map[string]any)["input_metrics_complete"] = false },
		func(d, m map[string]any) { d["argus_meta"].(map[string]any)["input_metrics_origin"] = "caller_hint" },
		func(d, m map[string]any) { m["argus_source_id"] = "other-source" },
		func(d, m map[string]any) { m["__name__"] = "queue_count" },
		func(d, m map[string]any) {
			meta := d["argus_meta"].(map[string]any)
			meta["input_metrics"] = append(meta["input_metrics"].([]any), map[string]any{"name": "queue", "unit": "s", "source_key": "source:4"})
		},
	} {
		data, metric = descriptorFixture()
		change(data, metric)
		if _, ok = metricDisplayDescriptor(data, metric); ok {
			t.Fatal("derived, incomplete, forged, foreign or ambiguous input metadata became output unit")
		}
	}
}

func TestRecordedSignalOriginalPresentsUnitAndSourceWithoutChangingValues(t *testing.T) {
	directory := os.Getenv("ARGUS_PLAN11_SIGNAL_REVIEW_DIR")
	if directory == "" {
		t.Skip("requires explicit archived authorized signal original")
	}
	files, err := filepath.Glob(filepath.Join(directory, "plan11-signal-untrusted-label-original-*.json"))
	if err != nil || len(files) != 1 {
		t.Fatal("actual label original unavailable", err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if json.Unmarshal(raw, &data) != nil {
		t.Fatal("invalid original")
	}
	row := data["data"].(map[string]any)["result"].([]any)[0].(map[string]any)
	before, _ := json.Marshal(row)
	data["items"] = []any{row}
	MetricContext(data)
	info := row["presentation_context"].(map[string]any)
	sources := data["query_context"].(map[string]any)["source_keys"].([]any)
	if len(sources) != 1 || info["unit"] != "1" || info["unit_origin"] != "authorized_series_metadata" || info["source"] != sources[0] {
		t.Fatal("actual original unit/source missing", info)
	}
	delete(row, "presentation_context")
	delete(row, "display_name")
	after, _ := json.Marshal(row)
	if string(before) != string(after) {
		t.Fatal("presentation changed original metric labels or values")
	}
	if output := os.Getenv("ARGUS_PLAN11_PRESENTATION_REPLAY_FILE"); output != "" {
		if !filepath.IsAbs(output) {
			t.Fatal("replay export must have an explicit absolute destination")
		}
		row["presentation_context"] = info
		indexRaw, err := os.ReadFile(filepath.Join(directory, "plan11-signal-untrusted-label-originals-index.json"))
		if err != nil {
			t.Fatal(err)
		}
		var index []struct {
			Hash      string `json:"sha256"`
			Bytes     int    `json:"bytes"`
			Arguments map[string]any
		}
		if json.Unmarshal(indexRaw, &index) != nil || len(index) != 1 || index[0].Hash != fmt.Sprintf("%x", sha256.Sum256(raw)) || index[0].Bytes != len(raw) {
			t.Fatal("recorded original no longer matches its actual authorized download index")
		}
		data["query_context"].(map[string]any)["query"] = index[0].Arguments["query"]
		resources := data["query_context"].(map[string]any)["resources"].([]any)
		ref := resources[0].(map[string]any)
		data["resources"] = []map[string]any{{"id": ref["id"], "name": ref["id"]}}
		MetricContext(data)
		payload := map[string]any{"schema_version": "argus.recorded_metric_presentation/v1", "tool": "telemetry.promql.query", "source_path": files[0],
			"source_sha256": fmt.Sprintf("%x", sha256.Sum256(raw)), "current_go_metric_context": true, "original_values_and_labels_unchanged": true,
			"scope": "controlled current presentation of archived authorized actual R79 query; not a fresh source-authorization or model run", "data": data}
		encoded, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = file.Write(encoded)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
	}
}
