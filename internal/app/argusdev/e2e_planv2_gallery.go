package argusdev

import (
	"context"
	"fmt"
	"slices"

	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func planV2MetricGallery() (dashboard.Spec, error) {
	spec := dashboard.EmptySpec()
	for i, kind := range []string{"timeseries", "stat", "gauge", "bar_gauge", "bar", "pie", "histogram", "heatmap", "state_timeline", "scatter", "table"} {
		query := "last_over_time(argus_m7_e2e_gauge_planv2[1h])"
		mode := "instant"
		if slices.Contains([]string{"timeseries", "heatmap", "state_timeline", "scatter"}, kind) {
			mode = "range"
		}
		unit := "ms"
		if kind == "histogram" {
			query = "last_over_time(argus_planv2_latency_bucket[1h])"
			unit = ""
		}
		if kind == "stat" {
			query += " / 10"
			unit = "percent_ratio"
		}
		panel := dashboard.Panel{ID: kind, Title: "Real " + kind, Signal: "metrics", Type: kind, AuthoringMode: "dsl", ApplicableResourceTypes: []string{"host", "kubernetes_cluster"}, SourceBinding: dashboard.SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, LocalFilters: []dashboard.LocalFilter{}, DetailQueryTargets: []dashboard.Target{}, Drilldowns: []dashboard.Drilldown{}, Thresholds: []dashboard.Threshold{}, Layout: dashboard.Rectangle{X: (i % 2) * 6, Y: (i / 2) * 52, W: 6, H: 48, MinW: 3, MinH: 24}, Unit: unit, Decimals: 1, Legend: true, Targets: []dashboard.Target{{ID: "main", Language: queryengine.LanguagePromQL, QueryMode: mode, SourceDefinition: dashboard.Definition{DSL: &dashboard.DSL{Expression: query}}, RangeStepPolicy: dashboard.StepPolicy{Kind: "fixed", Seconds: 10}, ParameterBindings: []dashboard.ParameterBinding{}}}}
		if slices.Contains([]string{"gauge", "bar_gauge", "bar"}, kind) {
			min, max := 0.0, 10.0
			panel.Display = &dashboard.DisplayOptions{Min: &min, Max: &max}
			panel.Thresholds = []dashboard.Threshold{{Value: 5, Tone: "warning"}}
		}
		spec.Panels = append(spec.Panels, panel)
	}
	if report := dashboard.Validate(spec); !report.Valid {
		return spec, fmt.Errorf("metric gallery invalid: %+v", report.Issues)
	}
	return spec, nil
}

func (a *App) preparePlanV2Gallery(ctx context.Context, env *E2EEnvironment) error {
	spec, err := planV2MetricGallery()
	if err != nil {
		return err
	}
	id, err := a.publishPlanV2Fixture(ctx, env, "PlanV2 real Metrics gallery", spec)
	if err != nil {
		return err
	}
	env.State.Values["p2_gallery_id"] = id
	return nil
}
