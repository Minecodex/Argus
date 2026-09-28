package dashboard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func validPanel() Panel {
	return Panel{ID: "cpu", Title: "CPU", Type: "timeseries", Signal: "metrics", AuthoringMode: "builder", ApplicableResourceTypes: []string{"host"}, SourceBinding: SourceBinding{SourceType: "hostmetrics", CapabilityVersion: "v1"}, Layout: Rectangle{W: 6, H: 8, MinW: 2, MinH: 2}, Targets: []Target{{ID: "a", Language: queryengine.LanguagePromQL, SourceDefinition: Definition{Builder: &Builder{Operation: "value", Metric: "system_cpu_utilization"}}, QueryMode: "range", RangeStepPolicy: StepPolicy{Kind: "auto", TargetPoints: 300, MinStepSeconds: 1}}}}
}

func TestSpecValidationRejectsAmbiguousQueriesAndLayouts(t *testing.T) {
	spec := EmptySpec()
	spec.Panels = []Panel{validPanel()}
	if report := Validate(spec); !report.Valid {
		t.Fatal(report.Issues)
	}
	spec.Panels[0].Targets[0].SourceDefinition.DSL = &DSL{Expression: "up"}
	if Validate(spec).Valid {
		t.Fatal("two editable query sources were accepted")
	}
	spec.Panels = []Panel{validPanel(), validPanel()}
	spec.Panels[1].ID = "memory"
	if Validate(spec).Valid {
		t.Fatal("overlapping panels accepted")
	}
	spec.Panels = []Panel{validPanel()}
	spec.Panels[0].Signal = "logs"
	if Validate(spec).Valid {
		t.Fatal("language/signal mismatch accepted")
	}
	raw, _ := json.Marshal(EmptySpec())
	var object map[string]any
	_ = json.Unmarshal(raw, &object)
	object["compiled_query"] = "user controlled"
	raw, _ = json.Marshal(object)
	if _, err := DecodeSpec(raw); err == nil {
		t.Fatal("derived query accepted as editable source")
	}
}

func TestVariableDependenciesAndProvenCandidateAbsence(t *testing.T) {
	spec := EmptySpec()
	spec.Variables = []Variable{
		{ID: "a", Name: "a", IncludeAll: true, Default: Selection{All: true}, Query: CandidateQuery{Signal: "logs", SourceBinding: SourceBinding{SourceType: "filelog"}, Field: "service_name", Filters: []Filter{{Variable: "b"}}}},
		{ID: "b", Name: "b", IncludeAll: true, Default: Selection{All: true}, Query: CandidateQuery{Signal: "logs", SourceBinding: SourceBinding{SourceType: "filelog"}, Field: "service_name", Filters: []Filter{{Variable: "a"}}}},
	}
	if Validate(spec).Valid {
		t.Fatal("cyclic dependency accepted")
	}
	current := Selection{Values: []string{"production"}}
	for _, test := range []struct {
		complete   bool
		membership map[string]bool
		changed    bool
	}{
		{false, nil, false}, {false, map[string]bool{"production": true}, false}, {false, map[string]bool{"production": false}, true}, {true, nil, true},
	} {
		value, changed := ReconcileSelection(current, []string{"test"}, test.complete, test.membership)
		if changed != test.changed || changed && !value.All {
			t.Fatalf("candidate fallback confused absence and uncertainty: %+v %v", value, changed)
		}
	}
}

func TestMetricsStepAndTypeConstraints(t *testing.T) {
	from, to := time.Unix(0, 0), time.Unix(3600, 0)
	if step, err := (StepPolicy{Kind: "auto", TargetPoints: 300, MinStepSeconds: 1}).Resolve(from, to); err != nil || step != 12*time.Second {
		t.Fatalf("wrong evaluation grid: %v %v", step, err)
	}
	if step, err := (StepPolicy{Kind: "fixed", Seconds: 1}).Resolve(from, to); err != nil || step != time.Second {
		t.Fatal("fixed step changed")
	}
	target := validPanel().Targets[0]
	target.SourceDefinition.Builder.Operation = "p95"
	target.SourceDefinition.Builder.MetricType = "gauge"
	target.SourceDefinition.Builder.WindowSeconds = 300
	if _, err := CompileTarget(target); err == nil {
		t.Fatal("P95 fabricated from a gauge")
	}
	target.SourceDefinition.Builder.MetricType = "histogram"
	if _, err := CompileTarget(target); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationDiffShowsSameCountQueryChanges(t *testing.T) {
	before := EmptySpec()
	before.Panels = []Panel{validPanel()}
	after := EmptySpec()
	after.Panels = []Panel{validPanel()}
	after.Panels[0].Targets[0].SourceDefinition.Builder.Metric = "system_memory_usage"
	changes := publicationDiff(before, after)
	if len(changes) != 1 || changes[0].Kind != "change" || !strings.Contains(changes[0].Text, "system_cpu_utilization") || !strings.Contains(changes[0].Text, "system_memory_usage") {
		t.Fatalf("same-count edit missing from diff: %+v", changes)
	}
}

func TestNativeGraphQLInputsAndDashboardReferencesAreSeparate(t *testing.T) {
	target := Target{ID: "trace", Language: queryengine.LanguageTrace, SourceDefinition: Definition{DSL: &DSL{Expression: `query Inspect($id: String!) { queryTrace(traceId: $id) { traceId } }`, Variables: map[string]any{"id": "trace-one"}}}}
	if _, err := CompileTarget(target); err != nil {
		t.Fatal(err)
	}
	target.SourceDefinition.DSL.Variables = nil
	if _, err := CompileTarget(target); err == nil {
		t.Fatal("missing required GraphQL variable was accepted")
	}
	prom := validPanel().Targets[0]
	prom.SourceDefinition = Definition{DSL: &DSL{Expression: `up{service="$unknown"}`}}
	if _, err := CompileTarget(prom); err == nil {
		t.Fatal("undefined dashboard reference was treated as a literal")
	}
	prom.SourceDefinition.DSL.Expression = `label_replace(up, "copy", "$name", "source", "(?P<name>.*)")`
	if _, err := CompileTarget(prom); err != nil {
		t.Fatalf("native regex replacement was mistaken for a dashboard variable: %v", err)
	}
}
