package argusdev

import (
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	argusopenapi "github.com/kakj-go/Argus/api/openapi"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
)

func TestRealHostMetricProofRejectsCrossResourceAndNonfiniteValues(t *testing.T) {
	series := func(host, source, value string) map[string]any {
		return map[string]any{"metric": map[string]any{"argus_resource_id": host, "argus_source_id": source}, "value": []any{float64(1), value}}
	}
	for _, input := range []map[string]any{series("other", "source", "10"), series("host", "other", "10"), series("host", "source", "NaN"), series("host", "source", "+Inf")} {
		if _, err := plan6HostObservedValues(input, "host", "source"); err == nil {
			t.Fatal("untrusted metric accepted", input)
		}
	}
	values, err := plan6HostObservedValues(map[string]any{"data": map[string]any{"result": []any{series("host", "source", "12.5")}}}, "host", "source")
	if err != nil || len(values) != 1 || values[0] != 12.5 {
		t.Fatal(values, err)
	}
}

func TestRealHostDashboardChecksTheActualPrometheusMatrixWireShape(t *testing.T) {
	data := promql.Matrix{{Metric: labels.FromStrings("argus_resource_id", "host", "argus_source_id", "source"), Floats: []promql.FPoint{{T: 1000, F: 12.5}}}}
	execution := dashboard.Execution{Panels: []dashboard.PanelExecution{}}
	for _, id := range []string{"cpu", "memory"} {
		execution.Panels = append(execution.Panels, dashboard.PanelExecution{ID: id, Status: "success", Targets: []dashboard.TargetExecution{{ID: "main", Status: "success", ResultType: "matrix", Data: data}}})
	}
	raw, _ := json.Marshal(execution)
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if err := plan6HostVerifyDashboard(wire, "host", "source"); err != nil {
		t.Fatal("real Prometheus matrix shape rejected", err)
	}
	if err := plan6HostVerifyDashboard(wire, "other", "source"); err == nil {
		t.Fatal("foreign Host matrix accepted")
	}
	wire["partial"] = true
	if err := plan6HostVerifyDashboard(wire, "host", "source"); err == nil {
		t.Fatal("partial matrix accepted")
	}
	wire["partial"] = false
	panel := wire["panels"].([]any)[0].(map[string]any)
	panel["targets"].([]any)[0].(map[string]any)["data"] = []any{}
	if err := plan6HostVerifyDashboard(wire, "host", "source"); err == nil {
		t.Fatal("empty matrix accepted")
	}
}

func TestRealHostDashboardFixtureUsesPublicHTTPContract(t *testing.T) {
	document, err := openapi3.NewLoader().LoadFromData(argusopenapi.BundledJSON)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(planV2DraftRequest(plan6HostDashboardSpec("system_cpu_utilization", "system_memory_utilization")))
	if err != nil {
		t.Fatal(err)
	}
	var wire any
	if err = json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	schema := document.Paths.Value(planV2DraftPath).Post.RequestBody.Value.Content["application/json"].Schema.Value
	if err = schema.VisitJSON(wire); err != nil {
		t.Fatal("real Host dashboard fixture violates the public contract", err)
	}
	panel := wire.(map[string]any)["spec"].(map[string]any)["panels"].([]any)[0].(map[string]any)
	panel["detail_query_targets"] = nil
	if err = schema.VisitJSON(wire); err == nil {
		t.Fatal("null array regression passed the public contract")
	}
}
