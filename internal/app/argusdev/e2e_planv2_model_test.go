package argusdev

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func TestPlanV2ModelOracleRejectsSemanticallyDifferentMetric(t *testing.T) {
	spec, err := planV2ThreeSignals()
	if err != nil {
		t.Fatal(err)
	}
	spec.Panels = spec.Panels[:1]
	spec.Panels[0].ApplicableResourceTypes = []string{"host", "kubernetes_cluster"}
	if !planV2ModelSpecMatches(spec, "dsl") {
		t.Fatal("valid fixture rejected")
	}
	spec.Panels[0].ApplicableResourceTypes = []string{"host"}
	if planV2ModelSpecMatches(spec, "dsl") {
		t.Fatal("unrequested narrowing hid the actual cluster metric")
	}
	spec.Panels[0].ApplicableResourceTypes = []string{"host", "kubernetes_cluster"}
	for _, expr := range []string{"argus_m7_e2e_gauge_planv2 * 0", "sum(argus_m7_e2e_gauge_planv2)", "argus_m7_e2e_gauge_planv2{host=\"hidden\"}"} {
		spec.Panels[0].Targets[0].SourceDefinition.DSL.Expression = expr
		if planV2ModelSpecMatches(spec, "dsl") {
			t.Fatalf("changed query passed: %s", expr)
		}
	}
}

func TestPlanV2VariableOracleRejectsGlobalFilterAndFakeBindings(t *testing.T) {
	spec, err := planV2ModelConditionSpec("blue", 3600)
	if err != nil {
		t.Fatal(err)
	}
	for i := range spec.Panels {
		p := &spec.Panels[i]
		p.Signal, p.Type, p.AuthoringMode = "metrics", "timeseries", "builder"
		p.ApplicableResourceTypes = []string{"host", "kubernetes_cluster"}
		p.Targets[0].QueryMode = "range"
		p.Targets[0].Language = queryengine.LanguagePromQL
		p.Targets[0].ParameterBindings = []dashboard.ParameterBinding{}
		p.Targets[0].SourceDefinition = dashboard.Definition{Builder: &dashboard.Builder{Operation: "value", Metric: "argus_planv2_selection", Filters: []dashboard.Filter{}, GroupBy: []string{}}}
	}
	spec.Panels[0].Targets[0].SourceDefinition.Builder.Filters = []dashboard.Filter{{Field: "pool", Operator: "=", Variable: "pool"}}
	if !planV2VariableSpecMatches(spec) {
		t.Fatal("valid scoped variable rejected")
	}
	spec.Panels[0].Targets[0].SourceDefinition.Builder.GroupBy = []string{"pool"}
	if !planV2VariableSpecMatches(spec) {
		t.Fatal("inert grouping field changed value query semantics")
	}
	spec.Panels[0].Targets[0].SourceDefinition.Builder.Operation = "sum"
	if planV2VariableSpecMatches(spec) {
		t.Fatal("aggregation accepted as raw values")
	}
	spec.Panels[0].Targets[0].SourceDefinition.Builder.Operation = "value"
	spec.Panels[1].Targets[0].SourceDefinition.Builder.Filters = []dashboard.Filter{{Field: "pool", Operator: "=", Variable: "pool"}}
	if planV2VariableSpecMatches(spec) {
		t.Fatal("variable incorrectly applied to both panels")
	}
	a := dashboard.Binding{ResourceType: "host", ResourceID: uuid.New()}
	b := dashboard.Binding{ResourceType: "kubernetes_cluster", ResourceID: uuid.New()}
	if !planV2BindingsEqual([]dashboard.Binding{b, a}, []dashboard.Binding{a, b}) {
		t.Fatal("binding order is not semantic")
	}
	if planV2BindingsEqual([]dashboard.Binding{a, a}, []dashboard.Binding{a, b}) {
		t.Fatal("duplicate binding hid missing cluster")
	}
	b.ResourceType = "pod"
	if planV2BindingsEqual([]dashboard.Binding{b}, []dashboard.Binding{a}) {
		t.Fatal("unsupported resource accepted")
	}
}

func TestPlanV2ConclusionOracleReadsValuesNotTimestamps(t *testing.T) {
	var data any
	if err := json.Unmarshal([]byte(`[{"metric":{"pool":"blue"},"value":[1800000000,"12.5"]},{"metric":{},"value":[1800000001,"2.5"]}]`), &data); err != nil {
		t.Fatal(err)
	}
	if sum, ok := planV2MetricSum(data); !ok || sum != 15 {
		t.Fatalf("sum=%v found=%v", sum, ok)
	}
	if err := json.Unmarshal([]byte(`{"rows":[{"severity_text":"ERROR","body":"INFO"},{"severity_text":"INFO","body":"ERROR"},{"severity_text":"ERROR"}]}`), &data); err != nil {
		t.Fatal(err)
	}
	if n := planV2ErrorLogCount(data); n != 2 {
		t.Fatalf("error log count=%d", n)
	}
	for _, reply := range []string{"整体正常", "```json\n{\"overall\":\"healthy\"}\n```", "```json\nnot json\n```"} {
		if _, err := planV2ParseConclusion(reply); err == nil {
			t.Fatalf("unsupported conclusion accepted: %s", reply)
		}
	}
	if _, err := planV2ParseConclusion("说明\n```json\n{\"overall\":\"unknown\",\"panels\":[],\"limitations\":[\"预算不足\"]}\n```"); err != nil {
		t.Fatal(err)
	}
}

func TestPlanV2ModelGroupSelectionIsExplicitAndClosed(t *testing.T) {
	for _, value := range []string{"authoring,unknown", "core,core", ",failures", "conditions,"} {
		if _, err := planV2ParseModelGroups(value); err == nil {
			t.Fatalf("invalid group selection accepted: %q", value)
		}
	}
	all, err := planV2ParseModelGroups("")
	if err != nil || len(all) != 4 {
		t.Fatalf("default must retain all groups: %v %v", all, err)
	}
	selected, err := planV2ParseModelGroups("authoring,conditions")
	if err != nil || len(selected) != 2 || selected[0] != "authoring" || selected[1] != "conditions" {
		t.Fatalf("selection changed scope: %v %v", selected, err)
	}
}

func TestPlanV2UnknownOracleSeparatesExecutionDeliveryAndAnalysis(t *testing.T) {
	jobs := []dashboard.QueryJobView{{Status: "failed", ErrorCode: "WORKSPACE_QUOTA_EXCEEDED", Manifest: &dashboard.QueryManifest{Execution: dashboard.Execution{Panels: []dashboard.PanelExecution{{ID: "logs", Status: "success"}}}}}}
	p := planV2ConclusionPanel{PanelID: "logs", QueryStatus: "success", DeliveryStatus: "failed", AnalysisStatus: "not_analyzed"}
	if !planV2UnknownStatusesMatch(p, jobs) {
		t.Fatal("successful query with failed delivery was conflated")
	}
	p.QueryStatus = "failed"
	if planV2UnknownStatusesMatch(p, jobs) {
		t.Fatal("delivery status accepted as query status")
	}
	p.QueryStatus = "not_checked"
	p.DeliveryStatus = "not_started"
	if !planV2UnknownStatusesMatch(p, nil) {
		t.Fatal("query not admitted was presented as executed")
	}
}

func TestPlanV2BudgetOracleAcceptsExactProseButRejectsSwappedOrStaleClaims(t *testing.T) {
	want := planV2BudgetFact{Scan: 235236784, Bytes: 8385978, Rows: 49997, Samples: 5000000, Calls: 254}
	text := "扫描字节剩 235,236,784 / 268,435,456，结果字节剩 8,385,978 / 8,388,608，行数剩 49,997 / 50,000，样本剩 5,000,000 / 5,000,000，调用剩 254 / 256。"
	if !planV2BudgetMatches(nil, text, want) {
		t.Fatal("correct natural-language budget rejected")
	}
	wrong := want
	wrong.Scan++
	if planV2BudgetMatches(&wrong, text, want) {
		t.Fatal("wrong structured budget hid behind correct prose")
	}
	if planV2BudgetMatches(nil, strings.Replace(text, "扫描字节剩 235,236,784", "扫描字节剩 8,385,978", 1), want) {
		t.Fatal("swapped budget dimension accepted")
	}
	if planV2BudgetMatches(nil, "预算充足", want) {
		t.Fatal("unsupported blanket budget claim accepted")
	}
}

func TestPlanV2ModelFileOracleCountsEachSignalShape(t *testing.T) {
	for _, entry := range []struct {
		raw  string
		want int
	}{
		{`[{"metric":{"name":"a"},"values":[[1,2],[3,4]]}]`, 1},
		{`{"result":[{},{}]}`, 2},
		{`{"rows":[{},{},{}]}`, 3},
		{`{"queryTraces":{"total":200,"traces":[{},{}]}}`, 2},
		{`{"queryTraces":{"total":200,"traces":[]}}`, 0},
	} {
		var value any
		if err := json.Unmarshal([]byte(entry.raw), &value); err != nil {
			t.Fatal(err)
		}
		if got := planV2RecordCount(value); got != entry.want {
			t.Fatalf("%s got %d want %d", entry.raw, got, entry.want)
		}
	}
}
