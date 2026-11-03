package argusdev

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMetricCohortCollectsIndependentSoftFailuresButStopsBoundaryOrAdmission(t *testing.T) {
	if plan11MetricCaseMustStop(map[string]any{"actual_model_evaluated": true}, errors.New("one numeric conclusion or read Oracle failed")) {
		t.Fatal("independent cohort stopped at an ordinary business failure")
	}
	if !plan11MetricCaseMustStop(map[string]any{"actual_model_evaluated": true}, errPlan11NavigationBoundary) {
		t.Fatal("hard boundary failed to stop further tasks")
	}
	if !plan11MetricCaseMustStop(map[string]any{"actual_model_evaluated": false}, errors.New("provider admission failed")) {
		t.Fatal("unavailable provider was retried as six business tasks")
	}
}

func TestOfflineMetricConsumptionNeedsSuccessfulActualChecksumAndAnalysisReceipt(t *testing.T) {
	file := "/workspace/queries/metrics/a1111111-1111-4111-8111-111111111111.json"
	body := []byte(`{"value":7}`)
	hash := sha256.Sum256(body)
	checksum := hex.EncodeToString(hash[:])
	for _, change := range []string{"none", "wrong_hash", "wrong_path", "failed_process", "partial", "echo_only", "metadata_only", "external_source", "checksum_only"} {
		t.Run(change, func(t *testing.T) {
			command := "for f in a1111111-1111-4111-8111-111111111111; do sha256sum /workspace/queries/metrics/$f.json; done; python3 -c \"\nimport json\nbase='/workspace/queries/metrics/'\nfor q in ['a1111111-1111-4111-8111-111111111111']:\n    d=json.load(open(base+q+'.json'))\n    print(d)\n\""
			stdout := checksum + "  " + file + "\na1111111-1111-4111-8111-111111111111 [200, '7']\n"
			summary := map[string]any{"exit_code": 0, "partial": false, "stdout": stdout}
			switch change {
			case "wrong_hash":
				summary["stdout"] = "bad  " + file
			case "wrong_path":
				summary["stdout"] = checksum + "  /workspace/queries/metrics/other.json"
			case "failed_process":
				summary["exit_code"] = 1
			case "partial":
				summary["partial"] = true
			case "echo_only":
				command = "echo '" + stdout + "'"
			case "metadata_only":
				command = "ls " + file
			case "checksum_only":
				summary["stdout"] = checksum + "  " + file + "\n"
			}
			source := "sandbox"
			if change == "external_source" {
				source = "external_mcp"
			}
			raw, _ := json.Marshal([]any{map[string]any{"tool": "bash", "source": source, "status": "succeeded", "input": map[string]any{"command": command}, "projection": map[string]any{"partial": false, "summary": summary}}})
			if plan11MetricOriginalConsumed(string(raw), file, checksum, len(body)) != (change == "none") {
				t.Fatal("offline proof confused command/checksum claim with successful original analysis", change)
			}
		})
	}
}

func TestRecordedR61OfflineLoopAndORSelectors(t *testing.T) {
	directory := os.Getenv("ARGUS_PLAN11_R61_REVIEW_DIR")
	if directory == "" {
		t.Skip("requires immutable R61 exported query originals and full process trace")
	}
	raw, err := os.ReadFile(filepath.Join(directory, "plan11-metric-semantics-zh-1-tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var calls []struct {
		ID, Tool, Status string
		Input            struct {
			Category, Name string
			Arguments      struct{ Query string }
		}
	}
	if json.Unmarshal(raw, &calls) != nil {
		t.Fatal("invalid recorded trace")
	}
	var round struct {
		Application string `json:"application_id"`
		From, To    time.Time
	}
	proof, err := os.ReadFile(filepath.Join(directory, "plan11-metric-semantics.json"))
	if err != nil || json.Unmarshal(proof, &round) != nil {
		t.Fatal(err)
	}
	values := map[string]float64{}
	for _, call := range calls {
		if call.Tool != "tool.invoke" || call.Status != "succeeded" || call.Input.Category != "metric" || call.Input.Name != "query" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(directory, "plan11-metric-semantics-zh-1-original-"+call.ID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		file := "/workspace/queries/metrics/" + call.ID + ".json"
		if !plan11MetricOriginalConsumed(string(raw), file, hex.EncodeToString(digest[:]), len(body)) {
			t.Fatal("actual loop/Python full-file receipt rejected", call.ID)
		}
		found, err := plan11MetricSemanticValues(body, call.Input.Arguments.Query, round.Application, "0e1c4276-13e8-5612-be78-81d913bee143:3", round.From, round.To)
		if err != nil {
			t.Fatal("actual legal OR/exact-selector route rejected", call.Input.Arguments.Query, err)
		}
		for kind, value := range found {
			values[kind] = value
		}
	}
	if values["counter"] != 5 || values["gauge"] != 7 || values["histogram_count"] != 3 || !plan11MetricClose(values["histogram_sum"]/values["histogram_count"], .2) || !plan11MetricClose(values["counter_rate"], .017777777777777778) {
		t.Fatal("recorded receipt/original numeric facts changed", values)
	}
	t.Log("Actual R61 five-file loop/SHA/Python analysis and safe OR originals replay passed; zero new model calls, no original round/prose rescore")
}
