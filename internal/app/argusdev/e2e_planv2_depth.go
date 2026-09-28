package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
	"net/http"
	"path/filepath"
	"time"
)

const planV2LinkedTrace = "aabbccddeeff00112233445566778899"

func planV2DepthSpec() (dashboard.Spec, error) {
	spec := dashboard.EmptySpec()
	spec.DefaultTimeRange.Seconds = 7200
	source := dashboard.SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}
	query := dashboard.CandidateQuery{Signal: "traces", SourceBinding: source, Field: "source_id", Filters: []dashboard.Filter{{Field: "service_name", Operator: "=", Value: "p2-backend"}}}
	panel := dashboard.Panel{ID: "linked", Title: "Cross-resource trace", Type: "trace_list", Signal: "traces", AuthoringMode: "builder", ApplicableResourceTypes: []string{"host", "kubernetes_cluster"}, SourceBinding: source,
		LocalFilters:       []dashboard.LocalFilter{{ID: "origin", Label: "Collection source", Kind: "query", Default: dashboard.Selection{All: true, Values: []string{}}, Query: &query}},
		Targets:            []dashboard.Target{{ID: "main", Language: queryengine.LanguageTrace, SourceDefinition: dashboard.Definition{Builder: &dashboard.Builder{Operation: "list", Limit: 100, GroupBy: []string{}, Filters: []dashboard.Filter{{Field: "traceId", Operator: "=", Value: planV2LinkedTrace}, {Field: "sourceId", Operator: "=", LocalParameter: "origin"}, {Field: "resource_attributes.deployment.environment.name", Operator: "=", Value: "p2-depth"}}}}, ParameterBindings: []dashboard.ParameterBinding{{Parameter: "origin", LocalParameter: "origin"}}, RangeStepPolicy: dashboard.StepPolicy{Kind: "auto", TargetPoints: 300, MinStepSeconds: 1}}},
		DetailQueryTargets: []dashboard.Target{}, Drilldowns: []dashboard.Drilldown{}, Thresholds: []dashboard.Threshold{}, Layout: dashboard.Rectangle{W: 12, H: 56, MinW: 4, MinH: 16}}
	spec.Panels = []dashboard.Panel{panel}
	generated, err := dashboard.GenerateStandardDrilldowns(spec, panel.ID, nil)
	return generated.Spec, err
}

func (a *App) preparePlanV2Depth(ctx context.Context, env *E2EEnvironment) error {
	at := env.State.Values["p2_trace_time"]
	if _, err := time.Parse(time.RFC3339Nano, at); err != nil {
		return fmt.Errorf("missing shared trace event time: %w", err)
	}
	if _, err := env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-server", "argus-server", "/usr/local/bin/argus-telemetry-e2e", "--endpoint=127.0.0.1:4317", "--planv2-trace-role=frontend", "--planv2-trace-time="+at); err != nil {
		return err
	}
	spec, err := planV2DepthSpec()
	if err != nil {
		return err
	}
	id, err := a.publishPlanV2Fixture(ctx, env, "PlanV2 trace and source boundaries", spec)
	if err != nil {
		return err
	}
	env.State.Values["p2_depth_id"] = id
	host := env.State.Values["m7_host_id"]
	before, err := a.waitPlanV2DepthExecution(ctx, env, id, host, 1)
	if err != nil {
		return err
	}
	old := before.Panels[0].Sources[0]
	env.State.Values["p2_old_source_id"] = old.ID.String()
	// M7 already uninstalled the actual Host Collector. Its published queries
	// must still see history; a reinstall goes through public Preview/Confirm.
	client, _ := scenarioHTTP(env)
	state, err := client.JSON(ctx, "p2-source-stopped", "enterprise", http.MethodGet, "/enterprise/hosts/"+host+"/collector", 200, nil, enterpriseHeaders(env, ""))
	if err != nil || state["status"] != "uninstalled" {
		return fmt.Errorf("host collection was not stopped: %v", err)
	}
	distribution, profiles, _, err := a.verifyM7Catalog(ctx, env)
	if err != nil {
		return err
	}
	key := "p2-reinstall-" + uuid.NewString()
	preview, err := client.JSON(ctx, key, "enterprise", http.MethodPost, "/enterprise/hosts/"+host+"/collector/actions/preview-install", 201, m7CollectorPreviewBody(distribution, profiles, ""), enterpriseHeaders(env, key))
	if err != nil {
		return err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	if _, err := a.confirmPendingAction(ctx, env, key+"-confirm", ref); err != nil {
		return err
	}
	if err := a.waitM7HostCollector(ctx, env, "converged"); err != nil {
		return err
	}
	if _, err := a.execM7Host(ctx, env, "argus-direct-executor", "/usr/local/bin/argus-telemetry-e2e", "--endpoint=127.0.0.1:4317", "--planv2-trace-role=backend", "--planv2-trace-generation=new", "--planv2-trace-time="+at); err != nil {
		return err
	}
	after, err := a.waitPlanV2DepthExecution(ctx, env, id, host, 2)
	if err != nil {
		return err
	}
	newSource := dashboard.ResolvedSource{}
	for _, source := range after.Panels[0].Sources {
		if source.ID != old.ID {
			newSource = source
			break
		}
	}
	if newSource.ID == uuid.Nil || newSource.Generation == old.Generation {
		return fmt.Errorf("reinstallation reused source identity")
	}
	env.State.Values["p2_new_source_id"] = newSource.ID.String()
	var detail dashboard.Drilldown
	for _, drill := range spec.Panels[0].Drilldowns {
		if drill.Kind == "trace_details" && drill.OriginQueryRef == "main" {
			detail = drill
		}
	}
	frozen, err := a.planV2DepthDrilldown(ctx, env, id, dashboard.DrilldownInput{ContextToken: before.ContextToken, PanelID: "linked", DrilldownID: detail.ID, Values: map[string]string{"trace_id": planV2LinkedTrace, "source_id": old.ID.String(), "resource_id": host}})
	if err != nil {
		return err
	}
	for _, source := range frozen.Sources {
		if source.ID == newSource.ID {
			return fmt.Errorf("old execution imported a new source")
		}
	}
	graph, _ := frozen.Result.Data.(map[string]any)["queryTraceGraph"].(map[string]any)
	spans, _ := graph["spans"].([]any)
	if len(spans) != 1 {
		return fmt.Errorf("frozen history lost span deduplication: %d", len(spans))
	}
	span, _ := spans[0].(map[string]any)
	if span["status"] != "error" {
		return fmt.Errorf("late duplicate span update was lost")
	}
	encoded, _ := json.MarshalIndent(map[string]any{"dashboard_id": id, "host_id": host, "old_source": old, "new_source": newSource, "history_after_uninstall": true, "frozen_sources": frozen.Sources, "deduplicated_spans": len(spans), "separate_installations": true}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "planv2-source-history.json"), encoded)
}

func (a *App) waitPlanV2DepthExecution(ctx context.Context, env *E2EEnvironment, id, host string, count int) (dashboard.Execution, error) {
	client, _ := scenarioHTTP(env)
	deadline := time.Now().Add(90 * time.Second)
	for {
		value, err := client.JSON(ctx, "p2-depth-execute", "enterprise", http.MethodPost, "/dashboards/"+id+"/execute", 200, map[string]any{"resource_ids": []string{host}}, enterpriseHeaders(env, ""))
		if err != nil {
			return dashboard.Execution{}, err
		}
		raw, _ := json.Marshal(value)
		var result dashboard.Execution
		if err := json.Unmarshal(raw, &result); err != nil {
			return result, err
		}
		if len(result.Panels) == 1 && result.Panels[0].Status == "success" && len(result.LocalCandidates["linked"]["origin"].Values) == count {
			return result, nil
		}
		if time.Now().After(deadline) {
			return result, fmt.Errorf("trace source history did not converge (%d sources)", count)
		}
		if err := waitContext(ctx, time.Second); err != nil {
			return result, err
		}
	}
}

func (a *App) planV2DepthDrilldown(ctx context.Context, env *E2EEnvironment, id string, input dashboard.DrilldownInput) (dashboard.DrilldownExecution, error) {
	client, _ := scenarioHTTP(env)
	value, err := client.JSON(ctx, "p2-depth-drilldown", "enterprise", http.MethodPost, "/dashboards/"+id+"/drilldown", 200, input, enterpriseHeaders(env, ""))
	if err != nil {
		return dashboard.DrilldownExecution{}, err
	}
	raw, _ := json.Marshal(value)
	var result dashboard.DrilldownExecution
	err = json.Unmarshal(raw, &result)
	return result, err
}
