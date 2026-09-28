package argusdev

import (
	"context"
	"fmt"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

func planV2NativeMappingSpec() (dashboard.Spec, error) {
	spec, err := planV2ParameterDashboard()
	if err != nil {
		return spec, err
	}
	spec.Variables = spec.Variables[:1]
	independent := spec.Panels[2]
	independent.ID, independent.Title = "independent", "Independent metric"
	independent.Layout = dashboard.Rectangle{X: 0, Y: 48, W: 12, H: 30, MinW: 3, MinH: 12}
	spec.Panels = []dashboard.Panel{}
	for index, source := range []string{"skywalking", "jaeger"} {
		panel := dashboard.Panel{ID: source, Title: source + " shared service", Type: "apm_services", Signal: "traces", AuthoringMode: "builder", ApplicableResourceTypes: []string{"kubernetes_cluster"}, SourceBinding: dashboard.SourceBinding{SourceType: source, CapabilityVersion: "v1"}, LocalFilters: []dashboard.LocalFilter{}, DetailQueryTargets: []dashboard.Target{}, Drilldowns: []dashboard.Drilldown{}, Thresholds: []dashboard.Threshold{}, Layout: dashboard.Rectangle{X: index * 6, W: 6, H: 44, MinW: 3, MinH: 12}, Targets: []dashboard.Target{{ID: "main", Language: queryengine.LanguageTrace, SourceDefinition: dashboard.Definition{Builder: &dashboard.Builder{Operation: "apm_services", Filters: []dashboard.Filter{{Field: "serviceName", Operator: "=", Variable: "pool"}}, GroupBy: []string{}, Limit: 10}}, ParameterBindings: []dashboard.ParameterBinding{{Parameter: "pool", Variable: "pool", ValueMap: map[string]string{"blue": "p2-shared-service", "green": "p2-not-received"}}}, RangeStepPolicy: dashboard.StepPolicy{Kind: "auto", TargetPoints: 300, MinStepSeconds: 1}}}}
		spec.Panels = append(spec.Panels, panel)
	}
	spec.Panels = append(spec.Panels, independent)
	if report := dashboard.Validate(spec); !report.Valid {
		return spec, fmt.Errorf("native mapping invalid: %+v", report.Issues)
	}
	return spec, nil
}

func (a *App) preparePlanV2NativeMapping(ctx context.Context, env *E2EEnvironment) error {
	if _, err := env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-server", "argus-server", "/usr/local/bin/argus-telemetry-e2e", "--native-selfcheck-url=http://127.0.0.1:8080", "--native-service-name=p2-shared-service"); err != nil {
		return err
	}
	spec, err := planV2NativeMappingSpec()
	if err != nil {
		return err
	}
	id, err := a.publishPlanV2Fixture(ctx, env, "PlanV2 native source mapping", spec)
	if err == nil {
		env.State.Values["p2_native_mapping_id"] = id
	}
	return err
}
