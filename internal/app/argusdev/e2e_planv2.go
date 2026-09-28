package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

const planV2DraftPath = "/dashboard-drafts"
const planV2DashboardPath = "/dashboards"

func (a *App) runPlanV2Scenario(ctx context.Context, env *E2EEnvironment) error {
	if err := a.configurePlanV2SelfMonitoring(ctx, env); err != nil {
		return err
	}
	if _, err := env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-server", "argus-server", "/usr/local/bin/argus-telemetry-e2e", "--native-selfcheck-url=http://127.0.0.1:8080"); err != nil {
		return fmt.Errorf("native SDK self-check: %w", err)
	}
	// M7's transport probe injects directly into the forwarding Gateway and has
	// no registered receiver origin. Dashboard source bindings intentionally
	// exclude it. Seed the three signals through the actual local OTLP receiver.
	if _, err := env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-server", "argus-server", "/usr/local/bin/argus-telemetry-e2e", "--endpoint=127.0.0.1:4317", "--marker=planv2", "--resource-id="+env.State.Values["m3_cluster_id"]); err != nil {
		return err
	}
	if err := a.waitPlanV2QueryReady(ctx, env); err != nil {
		return err
	}
	if err := a.verifyM7Signals(ctx, env, env.State.Values["m3_cluster_id"], "planv2"); err != nil {
		return err
	}
	// These real query requests create HTTP -> client RPC -> server RPC traces.
	if err := a.runM10QueryScenario(ctx, env); err != nil {
		return err
	}
	if err := a.planV2SelfDependencyFailure(ctx, env); err != nil {
		return err
	}
	client, _ := scenarioHTTP(env)
	if _, err := client.JSON(ctx, "p2-controlled-query-error", "enterprise", http.MethodPost, "/enterprise/metrics/query", 400, map[string]any{"query": "(", "resource_ids": []string{env.State.Values["m3_cluster_id"]}, "time_range": telemetryTimeRange(15 * time.Minute), "budget": telemetryBudget(100)}, enterpriseHeaders(env, "")); err != nil {
		return err
	}
	spec, err := planV2SelfDashboard()
	if err != nil {
		return err
	}
	// The suite itself continuously emits application spans. Freeze the seeded
	// acceptance window so later UI cases inspect the same received samples,
	// instead of eventually exhausting the topology fact budget with test traffic.
	to := time.Now().UTC()
	from := to.Add(-time.Hour)
	spec.DefaultTimeRange = dashboard.TimeRange{Kind: "absolute", From: &from, To: &to}
	draft, err := client.JSON(ctx, "p2-create-draft", "enterprise", http.MethodPost, planV2DraftPath, 201, planV2DraftRequest(spec), enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	id, err := stringField(draft, "id")
	if err != nil {
		return err
	}
	version, err := numberField(draft, "draft_version")
	if err != nil {
		return err
	}
	if _, err = client.JSON(ctx, "p2-sample-draft", "enterprise", http.MethodPost, "/dashboard-drafts/"+id+"/sample", 200, map[string]any{"expected_version": version}, enterpriseHeaders(env, "")); err != nil {
		return err
	}
	preview, err := client.JSON(ctx, "p2-publish-preview", "enterprise", http.MethodPost, "/dashboard-drafts/"+id+"/preview", 201, map[string]any{"expected_version": version}, enterpriseHeaders(env, "p2-preview"))
	if err != nil {
		return err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	if _, err = a.confirmPendingAction(ctx, env, "p2-publish", ref); err != nil {
		return err
	}
	items, err := client.JSONArray(ctx, "p2-list-published", "enterprise", http.MethodGet, "/dashboards", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	dashboardID := ""
	for _, item := range items {
		if item["name"] == "Argus self monitoring" {
			dashboardID, _ = item["id"].(string)
		}
	}
	if dashboardID == "" {
		return fmt.Errorf("published self-monitoring dashboard missing")
	}
	env.State.Values["p2_dashboard_id"] = dashboardID
	var result map[string]any
	deadline := time.Now().Add(90 * time.Second)
	for attempt := 0; time.Now().Before(deadline); attempt++ {
		result, err = client.JSON(ctx, fmt.Sprintf("p2-execute-%d", attempt), "enterprise", http.MethodPost, "/dashboards/"+dashboardID+"/execute", 200, map[string]any{"resource_ids": []string{env.State.Values["m3_cluster_id"]}}, enterpriseHeaders(env, ""))
		if err == nil {
			err = validatePlanV2SelfResults(result)
		}
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if err != nil {
		if raw, e := json.MarshalIndent(result, "", "  "); e == nil {
			_ = os.WriteFile(filepath.Join(env.Options.Artifacts, "planv2-selfmonitor-last-execution.json"), raw, 0600)
		}
		return fmt.Errorf("self-monitoring data did not converge: %w", err)
	}
	encoded, _ := json.MarshalIndent(map[string]any{"dashboard_id": dashboardID, "signals": "real SDK -> managed Collector -> Kafka -> ClickHouse -> published queries", "native_sdks": []string{"go2sky v1.5.0", "Jaeger Go v2.30.0"}, "application_path": "Argus HTTP -> Telemetry Query gRPC via OTel/OTLP", "basis": "received samples", "execution": result}, "", "  ")
	if err = os.WriteFile(filepath.Join(env.Options.Artifacts, "planv2-selfmonitor.json"), encoded, 0600); err != nil {
		return err
	}
	if env.Options.PlanV2RealModelOnly {
		if err := writePrivate(filepath.Join(env.Options.Artifacts, "planv2-browser-status.json"), []byte("{\"executed\":false,\"reason\":\"real_model_with_telemetry_and_files_scope\"}\n")); err != nil {
			return err
		}
		if err := a.checkPlanV2ServerHealth(ctx, env, true); err != nil {
			return err
		}
		if err := a.runPlanV2Files(ctx, env); err != nil {
			return err
		}
		modelErr := a.runPlanV2RealModel(ctx, env)
		return errors.Join(modelErr, a.checkPlanV2ServerHealth(ctx, env, false))
	}
	if err := a.preparePlanV2Lifecycle(ctx, env); err != nil {
		return err
	}
	if err := a.preparePlanV2Editor(ctx, env); err != nil {
		return err
	}
	if err := a.preparePlanV2Gallery(ctx, env); err != nil {
		return err
	}
	if err := a.preparePlanV2Parameters(ctx, env); err != nil {
		return err
	}
	if err := a.preparePlanV2Depth(ctx, env); err != nil {
		return err
	}
	if err := a.preparePlanV2NativeMapping(ctx, env); err != nil {
		return err
	}
	if err := a.preparePlanV2Faults(ctx, env); err != nil {
		return err
	}
	if err := a.runPlanV2Capacity(ctx, env); err != nil {
		return err
	}
	if err := a.checkPlanV2ServerHealth(ctx, env, true); err != nil {
		return err
	}
	var browserErr error
	if env.Options.PlanV2RuntimeOnly {
		browserErr = writePrivate(filepath.Join(env.Options.Artifacts, "planv2-browser-status.json"), []byte("{\"executed\":false,\"reason\":\"runtime_protocol_faults_scope\"}\n"))
	} else {
		browserErr = a.runPlaywright(ctx, env, `e2e/planv2-.*\.spec\.ts`, map[string]string{
			"ARGUS_PLANV2_E2E": "1", "ARGUS_PLANV2_USERNAME": env.State.Values["enterprise_username"], "ARGUS_PLANV2_PASSWORD": env.State.Values["enterprise_password"],
			"ARGUS_PLANV2_DASHBOARD_ID": dashboardID, "ARGUS_PLANV2_CLUSTER_ID": env.State.Values["m3_cluster_id"], "ARGUS_PLANV2_GALLERY_ID": env.State.Values["p2_gallery_id"],
			"ARGUS_PLANV2_DISPOSABLE_CLUSTER_ID": env.State.Values["p2_disposable_cluster_id"],
			"ARGUS_PLANV2_PARAMETERS_ID":         env.State.Values["p2_parameters_id"],
			"ARGUS_PLANV2_DEPTH_ID":              env.State.Values["p2_depth_id"], "ARGUS_PLANV2_HOST_ID": env.State.Values["m7_host_id"],
			"ARGUS_PLANV2_OLD_SOURCE_ID": env.State.Values["p2_old_source_id"], "ARGUS_PLANV2_NEW_SOURCE_ID": env.State.Values["p2_new_source_id"],
			"ARGUS_PLANV2_NATIVE_MAPPING_ID": env.State.Values["p2_native_mapping_id"],
			"ARGUS_PLANV2_CAPACITY_ID":       env.State.Values["p2_capacity_id"],
			"ARGUS_PLANV2_MODEL_ID":          env.State.Values["m4_model_id"],
			"ARGUS_PLANV2_EDITOR_ID":         env.State.Values["p2_editor_id"], "ARGUS_PLANV2_EDITOR_USERNAME": env.State.Values["p2_editor_username"], "ARGUS_PLANV2_EDITOR_PASSWORD": env.State.Values["p2_editor_password"],
			"ARGUS_PLANV2_COLLABORATION_ID": env.State.Values["p2_collaboration_dashboard_id"], "ARGUS_E2E_ENTERPRISE_EDITOR_TOTP_SECRET": env.State.Values["p2_editor_mfa_secret"], "ARGUS_E2E_ENTERPRISE_EDITOR_TOTP_LAST_CODE": env.State.Values["p2_editor_mfa_last"],
		})
	}
	// These checks use independent dashboards/conversations. Preserve both
	// outcomes so one UI assertion does not hide file-delivery regressions.
	if err := a.refreshEnterpriseLogin(ctx, env); err != nil {
		return errors.Join(browserErr, err)
	}
	filesErr := a.runPlanV2Files(ctx, env)
	var faultsErr error
	if filesErr == nil {
		faultsErr = a.runPlanV2Faults(ctx, env)
	}
	var toolsErr error
	if filesErr == nil {
		toolsErr = a.runPlanV2Tools(ctx, env)
	}
	healthErr := a.checkPlanV2ServerHealth(ctx, env, false)
	if err := errors.Join(browserErr, filesErr, faultsErr, toolsErr, healthErr); err != nil {
		return err
	}
	if env.Options.RealModel != nil {
		return a.runPlanV2RealModel(ctx, env)
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "planv2-real-model-status.json"), []byte("{\"status\":\"not_configured\",\"executed\":false}\n"))
}

func (a *App) waitPlanV2QueryReady(ctx context.Context, env *E2EEnvironment) error {
	client, _ := scenarioHTTP(env)
	deadline := time.Now().Add(45 * time.Second)
	for attempt := 0; ; attempt++ {
		result, err := client.JSON(ctx, fmt.Sprintf("p2-query-ready-%d", attempt), "enterprise", http.MethodPost, "/enterprise/metrics/query", 200, map[string]any{"query": "argus_m7_e2e_gauge_base", "resource_ids": []string{env.State.Values["m3_cluster_id"]}, "time_range": telemetryTimeRange(15 * time.Minute), "budget": telemetryBudget(100)}, enterpriseHeaders(env, ""))
		if err == nil {
			return nil
		}
		if result["code"] != "TELEMETRY_DEPENDENCY_UNAVAILABLE" || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func planV2DraftRequest(spec dashboard.Spec) map[string]any {
	return map[string]any{"name": "Argus self monitoring", "description": "OTel application traces and native SkyWalking/Jaeger SDK self-checks. Received samples only.", "spec": spec, "proposed_bindings": []any{}}
}

func validatePlanV2SelfResults(result map[string]any) error {
	panels, ok := result["panels"].([]any)
	if !ok || len(panels) != 18 {
		return fmt.Errorf("expected all 18 source-specific panels")
	}
	for _, raw := range panels {
		panel := raw.(map[string]any)
		id, _ := panel["id"].(string)
		if panel["status"] != "success" {
			return fmt.Errorf("%s is %v", id, panel["status"])
		}
		parts := strings.SplitN(id, "_", 2)
		sources, _ := panel["sources"].([]any)
		allowedSources := map[string]bool{}
		if len(sources) == 0 {
			return fmt.Errorf("%s source not frozen", id)
		}
		for _, rawSource := range sources {
			if rawSource.(map[string]any)["type"] != parts[0] {
				return fmt.Errorf("source mixing in %s", id)
			}
			key, _ := rawSource.(map[string]any)["id"].(string)
			allowedSources[key] = true
		}
		if !planV2SourcesMatch(panel["targets"], allowedSources) {
			return fmt.Errorf("%s returned data from a foreign source", id)
		}
		if strings.HasSuffix(id, "_services") {
			payload, _ := json.Marshal(panel["targets"])
			text := string(payload)
			if !strings.Contains(text, "received_entry_spans") {
				return fmt.Errorf("%s lost sample basis", id)
			}
			for _, service := range map[string][]string{"otlp": {"argus-server", "argus-telemetry-query"}, "skywalking": {"argus-selfcheck-skywalking"}, "jaeger": {"argus-selfcheck-jaeger"}}[parts[0]] {
				if !strings.Contains(text, service) {
					return fmt.Errorf("%s missing actual service %s", id, service)
				}
			}
			if !planV2HasReceivedError(panel["targets"]) {
				return fmt.Errorf("%s has no observed error samples", id)
			}
		}
	}
	return nil
}

func planV2SourcesMatch(value any, allowed map[string]bool) bool {
	switch item := value.(type) {
	case []any:
		for _, child := range item {
			if !planV2SourcesMatch(child, allowed) {
				return false
			}
		}
	case map[string]any:
		for key, child := range item {
			if key == "sourceId" {
				id, ok := child.(string)
				if !ok || !allowed[id] {
					return false
				}
			} else if !planV2SourcesMatch(child, allowed) {
				return false
			}
		}
	}
	return true
}

func planV2HasReceivedError(value any) bool {
	targets, _ := value.([]any)
	for _, raw := range targets {
		target, _ := raw.(map[string]any)
		data, _ := target["data"].(map[string]any)
		for _, root := range data {
			object, _ := root.(map[string]any)
			rows, _ := object["rows"].([]any)
			for _, rawRow := range rows {
				row, _ := rawRow.(map[string]any)
				count, _ := row["errorCount"].(float64)
				if count > 0 {
					return true
				}
			}
		}
	}
	return false
}

func planV2SelfDashboard() (dashboard.Spec, error) {
	spec := dashboard.EmptySpec()
	spec.DefaultTimeRange.Seconds = 3600
	spec.Variables = []dashboard.Variable{{ID: "environment", Name: "environment", Label: "Environment", IncludeAll: true, Default: dashboard.Selection{All: false, Values: []string{"planv2-e2e"}}, Query: dashboard.CandidateQuery{Signal: "traces", SourceBinding: dashboard.SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, Field: "resource_attributes.deployment.environment.name", Filters: []dashboard.Filter{}}}}
	sourceNames := []string{"OTel / OTLP", "SkyWalking", "Jaeger"}
	panelNames := []string{"Services", "Instances", "Endpoints", "Received samples: rate, errors and duration", "Service topology", "Traces"}
	// Tables need room for service identities, sample counts and durations. Keep
	// each source in its own block, with full-width service/trace overviews.
	positions := []dashboard.Rectangle{
		{X: 0, Y: 0, W: 12, H: 48}, {X: 0, Y: 52, W: 6, H: 48},
		{X: 6, Y: 52, W: 6, H: 48}, {X: 0, Y: 104, W: 6, H: 48},
		{X: 6, Y: 104, W: 6, H: 48}, {X: 0, Y: 156, W: 12, H: 56},
	}
	for sourceIndex, source := range []string{"otlp", "skywalking", "jaeger"} {
		for row, kind := range []string{"apm_services", "apm_instances", "apm_endpoints", "apm_red", "apm_topology", "trace_list"} {
			operation := kind
			if kind == "trace_list" {
				operation = "list"
			}
			builder := &dashboard.Builder{Operation: operation, Filters: []dashboard.Filter{}, GroupBy: []string{}, Limit: 100}
			if source == "otlp" {
				builder.Filters = append(builder.Filters, dashboard.Filter{Field: "resource_attributes.deployment.environment.name", Operator: "=", Variable: "environment"})
			}
			if kind == "apm_red" {
				builder.BucketSeconds = 60
			}
			position := positions[row]
			position.Y += sourceIndex * 216
			position.MinW, position.MinH = 3, 8
			spec.Panels = append(spec.Panels, dashboard.Panel{ID: source + "_" + strings.TrimPrefix(kind, "apm_"), Title: sourceNames[sourceIndex] + " · " + panelNames[row], Type: kind, Signal: "traces", AuthoringMode: "builder", ApplicableResourceTypes: []string{"kubernetes_cluster"}, SourceBinding: dashboard.SourceBinding{SourceType: source, CapabilityVersion: "v1"}, LocalFilters: []dashboard.LocalFilter{}, DetailQueryTargets: []dashboard.Target{}, Drilldowns: []dashboard.Drilldown{}, Thresholds: []dashboard.Threshold{}, Layout: position, Legend: true, Targets: []dashboard.Target{{ID: "main", Language: queryengine.LanguageTrace, SourceDefinition: dashboard.Definition{Builder: builder}, RangeStepPolicy: dashboard.StepPolicy{Kind: "auto", TargetPoints: 300, MinStepSeconds: 1}, ParameterBindings: []dashboard.ParameterBinding{}}}})
		}
	}
	for _, panel := range append([]dashboard.Panel{}, spec.Panels...) {
		generated, err := dashboard.GenerateStandardDrilldowns(spec, panel.ID, nil)
		if err != nil {
			return spec, err
		}
		spec = generated.Spec
	}
	if report := dashboard.Validate(spec); !report.Valid {
		return spec, fmt.Errorf("self dashboard invalid: %+v", report.Issues)
	}
	return spec, nil
}
