package argusdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/metricmeta"
)

func TestMetricSemanticBatchMatrixUsesActualTypedSeriesAtWindowEnd(t *testing.T) {
	application, source := uuid.NewString(), uuid.NewString()+":3"
	from, to := time.Unix(100, 0).UTC(), time.Unix(200, 0).UTC()
	makeResult := func() map[string]any {
		rows := []any{}
		for _, item := range []struct{ name, value string }{{"completed", "5"}, {"queue", "7"}, {"latency_count", "3"}, {"latency_sum", "0.6"}, {"latency_bucket", "3"}} {
			rows = append(rows, map[string]any{"metric": map[string]string{"__name__": "argus_plan11_sdk_" + item.name}, "values": []any{[]any{150, "1"}, []any{200, item.value}}})
		}
		return map[string]any{"status": "success", "query_context": map[string]any{"resources": []any{map[string]any{"type": "application", "id": application}}, "source_keys": []string{source}, "from": from, "to": to},
			"argus_meta": map[string]any{"partial": false, "input_metrics_origin": metricmeta.Origin, "input_metrics_complete": true, "input_metrics": []metricmeta.Descriptor{
				{Name: "argus_plan11_sdk_completed", Type: "counter", Unit: "{request}", SourceKey: source},
				{Name: "argus_plan11_sdk_queue", Type: "gauge", Unit: "{request}", SourceKey: source},
				{Name: "argus_plan11_sdk_latency", Type: "histogram", Unit: "s", SourceKey: source},
			}}, "data": map[string]any{"resultType": "matrix", "result": rows}}
	}
	for _, change := range []string{"none", "or_selectors", "source", "unit", "missing_end", "duplicate", "constant", "offset", "or_constant"} {
		t.Run(change, func(t *testing.T) {
			out, expression := makeResult(), `{__name__=~"argus_plan11_sdk_(completed|queue|latency)"}`
			switch change {
			case "or_selectors":
				expression = `argus_plan11_sdk_completed or argus_plan11_sdk_queue or argus_plan11_sdk_latency_count or argus_plan11_sdk_latency_sum`
			case "source":
				out["query_context"].(map[string]any)["source_keys"] = []string{uuid.NewString() + ":3"}
			case "unit":
				out["argus_meta"].(map[string]any)["input_metrics"].([]metricmeta.Descriptor)[0].Unit = "s"
			case "missing_end":
				out["data"].(map[string]any)["result"].([]any)[0].(map[string]any)["values"] = []any{[]any{150, "5"}}
			case "duplicate":
				data := out["data"].(map[string]any)
				rows := data["result"].([]any)
				data["result"] = append(rows, rows[0])
			case "constant":
				expression = "vector(5)"
			case "offset":
				expression += " offset 1m"
			case "or_constant":
				expression += " or vector(3)"
			}
			raw, _ := json.Marshal(out)
			values, err := plan11MetricSemanticValues(raw, expression, application, source, from, to)
			exploration := change == "constant" || change == "offset" || change == "or_constant"
			if exploration && err == nil && len(values) == 0 {
				return
			}
			if (err == nil) != (change == "none" || change == "or_selectors") {
				t.Fatal("batch evidence authority changed", change, values, err)
			}
			if err == nil && (len(values) != 4 || values["counter"] != 5 || values["gauge"] != 7 || values["histogram_count"] != 3 || !plan11MetricClose(values["histogram_sum"]/values["histogram_count"], .2)) {
				t.Fatal("batch family rows were mistaken for separate tool calls or raw histogram points", values)
			}
		})
	}
}

func TestMetricExplorationCannotContributeManufacturedOrUnrecognizedValues(t *testing.T) {
	application, source := uuid.NewString(), uuid.NewString()+":3"
	from, to := time.Unix(100, 0).UTC(), time.Unix(200, 0).UTC()
	out := map[string]any{"status": "success", "query_context": map[string]any{"resources": []any{map[string]any{"type": "application", "id": application}}, "source_keys": []string{source}, "from": from, "to": to},
		"argus_meta": map[string]any{"partial": false, "input_metrics_origin": metricmeta.Origin, "input_metrics_complete": true, "input_metrics": []metricmeta.Descriptor{{Name: "argus_plan11_sdk_latency", Type: "histogram", Unit: "s", SourceKey: source}}},
		"data":       map[string]any{"resultType": "vector", "result": []any{map[string]any{"metric": map[string]string{"__name__": "argus_plan11_sdk_latency_count"}, "value": []any{200, "3"}}}}}
	raw, _ := json.Marshal(out)
	for _, expression := range []string{`vector(3)`, `argus_plan11_sdk_latency_count*0+3`, `argus_plan11_sdk_latency_count and on() vector(1)`, `argus_plan11_sdk_latency_count/(argus_plan11_sdk_latency_sum/argus_plan11_sdk_latency_count)`} {
		values, err := plan11MetricSemanticValues(raw, expression, application, source, from, to)
		if err != nil || len(values) != 0 {
			t.Fatal("exploration certified a numeric fact", expression, values, err)
		}
	}
}

func TestRecordedR56BatchMetricSemanticsRoute(t *testing.T) {
	directory := os.Getenv("ARGUS_PLAN11_R56_REVIEW_DIR")
	if directory == "" {
		t.Skip("requires immutable R56 complete-read originals")
	}
	var proof struct {
		Application string `json:"application_id"`
		From, To    time.Time
	}
	raw, err := os.ReadFile(filepath.Join(directory, "plan11-metric-semantics.json"))
	if err != nil || json.Unmarshal(raw, &proof) != nil {
		t.Fatal("missing original round identity/window", err)
	}
	values := map[string]float64{}
	for _, item := range []struct{ id, expression string }{
		{"19333f97-c69c-5a4d-9a9a-50634158e4f4", `{__name__=~"argus_plan11_sdk_(completed|queue|latency)"}`},
		{"81b6d3c0-5d24-5be1-b00b-73b63a78269c", "rate(argus_plan11_sdk_completed[600s])"},
	} {
		body, err := os.ReadFile(filepath.Join(directory, "verified-read-original-"+item.id+".json"))
		if err != nil {
			t.Fatal(err)
		}
		found, err := plan11MetricSemanticValues(body, item.expression, proof.Application, "46e1499f-6ad5-5ba3-866f-47665ce9b737:3", proof.From, proof.To)
		if err != nil {
			t.Fatal("actual valid batch/range route rejected", err)
		}
		for key, value := range found {
			values[key] = value
		}
	}
	if values["counter"] != 5 || values["gauge"] != 7 || values["histogram_count"] != 3 || !plan11MetricClose(values["histogram_sum"]/values["histogram_count"], .2) || !plan11MetricClose(values["counter_rate"], .017777777777777778) {
		t.Fatal("R56 exact original values changed", values)
	}
	t.Log("R56 two actual batch/matrix originals accepted as five numeric facts; no new provider calls and no rescore of original failed round or prose")
}
