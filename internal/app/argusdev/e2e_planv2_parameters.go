package argusdev

import (
	"context"
	"fmt"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func planV2ParameterDashboard() (dashboard.Spec, error) {
	spec := dashboard.EmptySpec()
	source := dashboard.SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}
	query := func(field string, filters []dashboard.Filter) dashboard.CandidateQuery {
		return dashboard.CandidateQuery{Signal: "metrics", SourceBinding: source, Metric: "argus_planv2_selection", Field: field, Filters: filters}
	}
	poolFilter := []dashboard.Filter{{Field: "pool", Operator: "=", Variable: "pool"}}
	spec.Variables = []dashboard.Variable{
		{ID: "pool", Name: "pool", Label: "Pool", IncludeAll: true, Default: dashboard.Selection{Values: []string{"blue"}}, Query: query("pool", []dashboard.Filter{})},
		{ID: "member", Name: "member", Label: "Member", IncludeAll: true, Multiple: true, Default: dashboard.Selection{Values: []string{"blue-220"}}, Query: query("member", poolFilter)},
	}
	for i, id := range []string{"dependent", "local", "independent", "mapped_logs"} {
		expression := "sum(last_over_time(argus_planv2_selection[1h]))"
		bindings := []dashboard.ParameterBinding{}
		panel := dashboard.Panel{ID: id, Title: id, Signal: "metrics", Type: "stat", AuthoringMode: "dsl", ApplicableResourceTypes: []string{"kubernetes_cluster"}, SourceBinding: source, LocalFilters: []dashboard.LocalFilter{}, DetailQueryTargets: []dashboard.Target{}, Drilldowns: []dashboard.Drilldown{}, Thresholds: []dashboard.Threshold{}, Layout: dashboard.Rectangle{X: (i % 2) * 6, Y: (i / 2) * 44, W: 6, H: 44, MinW: 3, MinH: 12}}
		if id == "dependent" {
			expression = `sum(last_over_time(argus_planv2_selection{pool="$pool",member="$member"}[1h]))`
			bindings = []dashboard.ParameterBinding{{Parameter: "pool", Variable: "pool"}, {Parameter: "member", Variable: "member"}}
		}
		if id == "local" {
			expression = `sum(last_over_time(argus_planv2_selection{pool="$pool",member="$instance"}[1h]))`
			bindings = []dashboard.ParameterBinding{{Parameter: "pool", Variable: "pool"}, {Parameter: "instance", LocalParameter: "instance"}}
			candidate := query("member", poolFilter)
			panel.LocalFilters = []dashboard.LocalFilter{{ID: "instance", Label: "Instance", Kind: "query", Default: dashboard.Selection{All: true, Values: []string{}}, Query: &candidate}}
		}
		target := dashboard.Target{ID: "main", Language: queryengine.LanguagePromQL, QueryMode: "instant", SourceDefinition: dashboard.Definition{DSL: &dashboard.DSL{Expression: expression}}, ParameterBindings: bindings, RangeStepPolicy: dashboard.StepPolicy{Kind: "fixed", Seconds: 10}}
		if id == "mapped_logs" {
			panel.Signal, panel.Type = "logs", "logs"
			panel.LocalFilters = []dashboard.LocalFilter{{ID: "term", Label: "Keyword", Kind: "text", Default: dashboard.Selection{All: true, Values: []string{}}}}
			target.Language, target.QueryMode = queryengine.LanguageKQL, ""
			target.SourceDefinition.DSL = &dashboard.DSL{Expression: `body : "planv2 selection" AND severity_text = "$pool" AND body = "$term"`, Pipeline: "limit 10"}
			target.ParameterBindings = []dashboard.ParameterBinding{{Parameter: "pool", Variable: "pool", ValueMap: map[string]string{"blue": "INFO", "green": "ERROR"}}, {Parameter: "term", LocalParameter: "term"}}
		}
		panel.Targets = []dashboard.Target{target}
		spec.Panels = append(spec.Panels, panel)
	}
	if report := dashboard.Validate(spec); !report.Valid {
		return spec, fmt.Errorf("parameters fixture invalid: %+v", report.Issues)
	}
	return spec, nil
}

func (a *App) preparePlanV2Parameters(ctx context.Context, env *E2EEnvironment) error {
	spec, err := planV2ParameterDashboard()
	if err != nil {
		return err
	}
	id, err := a.publishPlanV2Fixture(ctx, env, "PlanV2 variable boundaries", spec)
	if err == nil {
		env.State.Values["p2_parameters_id"] = id
	}
	return err
}
