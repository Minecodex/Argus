package dashboard

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardparams"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"reflect"
	"testing"
	"time"
)

func conditionFixture() Spec {
	spec := EmptySpec()
	spec.Variables = []Variable{{ID: "env-id", Name: "env", Label: "Environment", Multiple: true, IncludeAll: true, Default: Selection{All: true, Values: []string{}}, Query: CandidateQuery{Signal: "logs", SourceBinding: SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, Field: "environment", Filters: []Filter{}}}}
	return spec
}

func TestConditionConsumerDetectsDSLFieldChangesButIgnoresFormatting(t *testing.T) {
	target := Target{ID: "q", Language: queryengine.LanguagePromQL, QueryMode: "instant", SourceDefinition: Definition{DSL: &DSL{Expression: `up{environment="$env"}`}}, ParameterBindings: []ParameterBinding{{Parameter: "env", Variable: "env"}}}
	original := analysisQueryContract(target)
	target.SourceDefinition = Definition{DSL: &DSL{Expression: `up { environment = "$env" }`}}
	if analysisQueryContract(target) != original {
		t.Fatal("formatting changed query parameter meaning")
	}
	target.SourceDefinition = Definition{DSL: &DSL{Expression: `up{service="$env"}`}}
	if analysisQueryContract(target) == original {
		t.Fatal("DSL reinterpreted a parameter without invalidating its contract")
	}
	spec := conditionFixture()
	spec.Panels = []Panel{{ID: "metrics", Signal: "metrics", SourceBinding: SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, Targets: []Target{{ID: "q", Language: queryengine.LanguagePromQL, QueryMode: "instant", SourceDefinition: Definition{Builder: &Builder{Operation: "current", Metric: "up", MetricType: "gauge", Filters: []Filter{{Field: "environment", Operator: "=", Variable: "env"}}}}}}}}
	before := conditionContracts(spec)["/variables/env"]
	spec.Panels[0].Targets[0].SourceDefinition.Builder.Filters[0].Field = "service"
	if conditionContracts(spec)["/variables/env"] == before {
		t.Fatal("implicit builder parameter use escaped compatibility checks")
	}
}
func cloneConditionSpec(spec Spec) Spec {
	raw, _ := json.Marshal(spec)
	var result Spec
	_ = json.Unmarshal(raw, &result)
	return result
}
func TestConditionsPersistOnlyExplicitOverridesAndCheckRevisionMeaning(t *testing.T) {
	spec := conditionFixture()
	base := dashboardparams.Empty()
	event := uuid.New()
	chosen := Selection{Values: []string{"prod"}}
	state, err := mergeConditions(base, dashboardparams.Patch{Time: &TimeRange{Kind: "relative", Seconds: 1800}, Variables: map[string]*Selection{"env": &chosen}}, []dashboardparams.Evidence{{Path: "/time", Quote: "最近半小时"}, {Path: "/variables/env", Quote: "生产环境"}}, event, "检查生产环境最近半小时", conditionContracts(spec))
	if err != nil {
		t.Fatal(err)
	}
	if len(base.Overrides.Variables) > 0 || base.Overrides.Time != nil {
		t.Fatal("mutated prior Run state")
	}
	first := time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC)
	input, err := conditionParameters(spec, state, first)
	if err != nil {
		t.Fatal(err)
	}
	if input.To.Sub(*input.From) != 30*time.Minute || len(input.PanelIDs) > 0 {
		t.Fatal("time or strategy inheritance incorrect")
	}
	next, err := conditionParameters(spec, state, first.Add(time.Hour))
	if err != nil || !next.To.Equal(first.Add(time.Hour)) {
		t.Fatal("relative duration did not resolve against new request")
	}
	inherited, err := mergeConditions(state, dashboardparams.Patch{}, nil, uuid.New(), "继续", conditionContracts(spec))
	if err != nil || !reflect.DeepEqual(state, inherited) {
		t.Fatal("follow-up mutated explicit conditions")
	}
	same := cloneConditionSpec(spec)
	same.Variables[0].Label = "New title"
	same.Variables[0].Default = Selection{Values: []string{"stage"}}
	if paths := inheritedConflicts(state, conditionContracts(same)); len(paths) > 0 {
		t.Fatal("presentation/default change invalidated explicit selection")
	}
	changed := cloneConditionSpec(spec)
	changed.Variables[0].Query.Field = "deployment_env"
	_, err = mergeConditions(state, dashboardparams.Patch{}, nil, uuid.New(), "继续", conditionContracts(changed))
	var code toolruntime.Error
	if !errors.As(err, &code) || code.Kind != "DASHBOARD_CONDITIONS_INCOMPATIBLE" {
		t.Fatalf("semantic change inherited silently: %v", err)
	}
	reset, err := mergeConditions(state, dashboardparams.Patch{Variables: map[string]*Selection{"env": nil}}, []dashboardparams.Evidence{{Path: "/variables/env", Quote: "环境改回默认"}}, event, "环境改回默认", conditionContracts(changed))
	if err != nil || len(reset.Overrides.Variables) != 0 || reset.Overrides.Time == nil {
		t.Fatal("reset should clear only the requested override")
	}
	_, err = mergeConditions(state, dashboardparams.Patch{ResetTime: true}, []dashboardparams.Evidence{{Path: "/time", Quote: "上条消息的要求"}}, event, "继续", conditionContracts(spec))
	if !errors.As(err, &code) || code.Kind != "DASHBOARD_CONDITION_EVIDENCE_REQUIRED" {
		t.Fatal("accepted evidence outside current message")
	}
	defaults, err := mergeConditions(dashboardparams.Empty(), dashboardparams.Patch{}, nil, event, "检查", conditionContracts(spec))
	if err != nil || len(defaults.Overrides.Variables) != 0 || defaults.Overrides.Time != nil {
		t.Fatal("published defaults became explicit user conditions")
	}
}
func TestEmptyResourceScopeCannotBecomeAllAndCandidateResetDoesNotCopyDefaults(t *testing.T) {
	state := dashboardparams.Empty()
	state.Overrides.Resources = &dashboardparams.Resources{IDs: []uuid.UUID{}}
	if _, err := conditionParameters(EmptySpec(), state, time.Now()); err == nil {
		t.Fatal("explicit empty resources became all")
	}
	state.Overrides.Resources = &dashboardparams.Resources{All: true, IDs: []uuid.UUID{}}
	state.Overrides.Variables["env"] = Selection{Values: []string{"gone"}}
	state.Contracts["/variables/env"] = "meaning"
	updated, changed := reconcileExplicit(state, Execution{Variables: map[string]Selection{"env": {All: true, Values: []string{}}, "implicit": {Values: []string{"value"}}}})
	if !changed || !updated.Overrides.Variables["env"].All || len(updated.Overrides.Variables) != 1 || !updated.Overrides.Resources.All {
		t.Fatal("candidate reset leaked defaults or altered resources")
	}
}
func TestLocalConditionFingerprintTracksDependenciesWithoutCouplingSiblings(t *testing.T) {
	spec := conditionFixture()
	spec.Panels = []Panel{{ID: "logs", Signal: "logs", SourceBinding: SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, LocalFilters: []LocalFilter{
		{ID: "service", Kind: "query", Query: &CandidateQuery{Field: "service", Filters: []Filter{{Variable: "env"}}}},
		{ID: "instance", Kind: "query", Query: &CandidateQuery{Field: "instance", Filters: []Filter{{LocalParameter: "service"}}}},
		{ID: "unrelated", Kind: "query", Query: &CandidateQuery{Field: "unrelated"}},
	}}}
	hashes := conditionContracts(spec)
	subset, err := candidateSpec(spec, "logs", "instance")
	if err != nil || len(subset.Variables) != 1 || len(subset.Panels) != 1 || len(subset.Panels[0].LocalFilters) != 2 {
		t.Fatalf("candidate dependency closure is incomplete or queries unrelated siblings: %v", err)
	}
	changed := cloneConditionSpec(spec)
	changed.Panels[0].LocalFilters[2].Query.Field = "other"
	next := conditionContracts(changed)
	if hashes["/local_values/logs/service"] != next["/local_values/logs/service"] || hashes["/local_values/logs/instance"] != next["/local_values/logs/instance"] {
		t.Fatal("unrelated local filter changed sibling meaning")
	}
	changed = cloneConditionSpec(spec)
	changed.Panels[0].LocalFilters[0].Query.Field = "new_service"
	next = conditionContracts(changed)
	if hashes["/local_values/logs/instance"] == next["/local_values/logs/instance"] {
		t.Fatal("changed local dependency not detected")
	}
}
