package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
)

// This is the deployed foundation gate, not a substitute for the 76-case
// matrix. It uses the same owned-environment lifecycle as other full-stack
// gates, including snapshotting/restoring existing workloads on every exit.
func (a *App) runPlan8DeployedScenario(ctx context.Context, env *E2EEnvironment) (failure error) {
	if env.Options.RealModel == nil {
		return fmt.Errorf("Plan8 deployed acceptance requires a real model")
	}
	before, err := a.plan8Snapshot(ctx)
	if err != nil {
		return err
	}
	proof := map[string]any{
		"schema_version": "argus.plan8_deployed_foundation/v1", "run_id": env.Options.RunID,
		"scope":         "owned_linux_host_bastion_collectors_gateway_query_service_instances_migration_monitoring_api_real_model_workspace",
		"plan_complete": false, "release_eligible": false, "stages": []map[string]any{},
		"model":                       env.Options.RealModel.ModelID,
		"model_reasoning_effort":      env.Options.RealModel.ReasoningEffort,
		"source_before":               before,
		"selected_phase":              env.Options.Plan8Phase,
		"selected_integration_groups": env.Options.Plan8Integrations,
		"partial_scope":               env.Options.Plan8Phase != "" && env.Options.Plan8Phase != "all",
		"model_budget":                map[string]any{"scenarios": 8, "provider_call_acceptance_limit_per_scenario": 32, "maximum_duration_seconds_per_scenario": 600, "scenario_names": []string{"kubernetes_owned_workload_recovery", "independent_collection_change", "applications_apm_logs_shared_database", "secure_collection_form_and_followup", "sdk_private_key_delivery", "collection_alert_health", "same_conversation_application_revocation", "host_dashboard_workspace"}},
	}
	if env.Options.Plan8Phase != "" && env.Options.Plan8Phase != "all" {
		proof["scope"] = "partial_deployed_" + env.Options.Plan8Phase
		count := 0
		names := []string{}
		if env.Options.Plan8Phase == "kubernetes" {
			count = 1
			names = []string{"kubernetes_owned_workload_recovery"}
		}
		proof["model_budget"] = map[string]any{"scenarios": count, "scenario_names": names, "provider_call_acceptance_limit_per_scenario": 32, "maximum_duration_seconds_per_scenario": 600}
	}
	defer func() {
		after, err := a.plan8Snapshot(context.WithoutCancel(ctx))
		proof["source_after"] = after
		proof["source_unchanged"] = err == nil && after.Fingerprint == before.Fingerprint
		if err != nil || after.Fingerprint != before.Fingerprint {
			failure = errors.Join(failure, err, errors.New("deployed Plan8 source or Collector artifacts changed during acceptance"))
		}
		proof["passed"] = failure == nil
		raw, e := json.MarshalIndent(proof, "", "  ")
		if e == nil {
			e = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-deployed-foundation.json"), raw)
		}
		failure = errors.Join(failure, e)
	}()
	var h *plan6HostLoop
	stages := []struct {
		name string
		run  func() error
	}{
		{"workspace-device-prerequisites", func() error { return a.plan8WorkspaceDevicePrerequisites(ctx, env) }},
		{"workspace", func() error { return a.configureP5Workspace(ctx, env) }},
		{"control-prerequisite", func() error { return a.preparePlan7ControlModel(ctx, env) }},
		{"owned-linux-hosts", func() error {
			fixture, err := a.createPlan7DockerFixture(ctx, env)
			h = &plan6HostLoop{Docker: fixture, HostName: "Plan8 actual Linux Host", CPUExpression: `100 * (1 - avg by (argus_resource_id,argus_source_id,argus_collection_id) (system_cpu_utilization{state="idle"}))`, MemoryExpression: `100 * system_memory_utilization{state="used"}`}
			return err
		}},
		{"formal-onboarding-and-managed-collector", func() error { return a.plan6HostOnboarding(ctx, env, h) }},
		{"actual-query-and-dashboard", func() error { return a.plan6HostMetricsDashboard(ctx, env, h) }},
		{"normal-production-backend", func() error {
			env.State.Values["host_prerequisites_ready"] = "true"
			return a.usePlan6ProductionBackend(ctx, env)
		}},
		{"monitoring-api", func() error { return a.plan8DeployedMonitoringAPI(ctx, env) }},
		{"host-application-files-and-system-journal", func() error { return a.plan8DeployedHostLogs(ctx, env, h) }},
		{"kubernetes-namespace-workbench", func() error { return a.plan8DeployedKubernetesNamespace(ctx, env) }},
		{"service-instances-and-migration", func() error { return a.plan8DeployedServices(ctx, env, h) }},
		{"additional-services-and-managed-dashboards", func() error { return a.plan8DeployedExtraServices(ctx, env, h) }},
		{"sdk-application-protocols", func() error { return a.plan8DeployedSDK(ctx, env, h) }},
		{"real-model-applications-and-shared-database", func() error { return a.plan8ModelApplicationObservation(ctx, env) }},
		{"real-model-secure-form", func() error { return a.plan8ModelSecureCollectionForm(ctx, env, h) }},
		{"real-model-collection-alert-health", func() error { return a.plan8ModelCollectionAlertHealth(ctx, env, h) }},
		{"real-model-application-revocation", func() error { return a.plan8ModelApplicationRevocation(ctx, env) }},
		{"real-model-workspace", func() error { return a.plan8DeployedModelRead(ctx, env, h) }},
		{"existing-target-authorization", func() error { return a.plan8ExistingTargetAuthorization(ctx, env) }},
		{"additional-supported-integrations", func() error { return a.plan8DeployedSupportedTargets(ctx, env, h) }},
		{"real-portal-workspace", func() error { return a.plan8DeployedPortalWorkspace(ctx, env) }},
	}
	for _, stage := range stages {
		if !plan8StageSelected(env.Options.Plan8Phase, stage.name) {
			continue
		}
		if plan8IndependentModelStage(stage.name) {
			delete(env.State.Values, "plan8_last_model_run")
		}
		fmt.Fprintf(a.stdout, "Plan8 deployed %s started\n", stage.name)
		start := time.Now()
		err := stage.run()
		stageProof := map[string]any{"name": stage.name, "passed": err == nil, "duration_seconds": time.Since(start).Seconds()}
		proof["stages"] = append(proof["stages"].([]map[string]any), stageProof)
		if err != nil {
			recovered := false
			if (stage.name == "host-application-files-and-system-journal" || stage.name == "kubernetes-namespace-workbench" && errors.Is(err, errPlan8KubernetesQueueRecovery)) && ctx.Err() == nil {
				recovery := a.plan8RecoverHostLogStage(ctx, env, h)
				recovered = recovery == nil
				stageProof["recovered_for_independent_following_stages"] = recovered
				err = errors.Join(err, recovery)
			}
			if run := env.State.Values["plan8_last_model_run"]; plan8IndependentModelStage(stage.name) && run != "" {
				err = errors.Join(err, a.plan8StopFailedModelTask(ctx, env, run))
			}
			failure = errors.Join(failure, fmt.Errorf("Plan8 deployed %s: %w", stage.name, err))
			independent := plan8IndependentModelStage(stage.name) || stage.name == "existing-target-authorization"
			if !independent && !recovered || errors.Is(err, errPlan8ModelCleanup) || ctx.Err() != nil {
				return failure
			}
			fmt.Fprintf(a.stdout, "Plan8 deployed %s failed; remaining independent stages will still run\n", stage.name)
			continue
		}
		fmt.Fprintf(a.stdout, "Plan8 deployed %s passed\n", stage.name)
	}
	proof["host_id"], proof["collector_id"], proof["dashboard_id"] = h.HostID, h.CollectorID, h.DashboardID
	return failure
}

func plan8IndependentModelStage(name string) bool {
	switch name {
	case "real-model-applications-and-shared-database", "real-model-secure-form", "real-model-collection-alert-health", "real-model-application-revocation", "real-model-workspace":
		return true
	default:
		return false
	}
}

func (a *App) plan8DeployedMonitoringAPI(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	for _, endpoint := range []string{"/applications", "/monitoring/collections"} {
		if _, err = client.JSON(ctx, "plan8-api-"+filepath.Base(endpoint), "enterprise", http.MethodGet, endpoint, 200, nil, enterpriseHeaders(env, "")); err != nil {
			return err
		}
	}
	_, err = client.JSONArray(ctx, "plan8-api-integrations", "enterprise", http.MethodGet, "/monitoring/integrations", 200, nil, enterpriseHeaders(env, ""))
	return err
}

func (a *App) plan8DeployedModelRead(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) (failure error) {
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return err
	}
	conversation, err := a.plan7ModelConversation(ctx, env, model, "Plan8 actual dashboard")
	if err != nil {
		return err
	}
	prompt := fmt.Sprintf("请分析当前选中的仪表盘，只读取主机%s（ID %s）的最近30分钟 CPU 和内存。先调用工具查询仪表盘，再读取 Workspace 中生成的实际结果文件，说明观测值、采集来源和限制，不得修改配置或用示例值。%s", h.HostName, h.HostID, planV2ConclusionPrompt)
	bounded, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	sample, err := a.realModelTask(bounded, env, conversation, "plan8-dashboard-workspace", prompt, nil, map[string]any{"mode": "analyze", "dashboard_ids": []string{h.DashboardID}})
	defer func() { failure = errors.Join(failure, a.planV2ModelEvidence(ctx, env, conversation, sample, failure)) }()
	if err != nil {
		return err
	}
	jobs, err := a.planV2ModelJobs(ctx, env, conversation, sample.RunID)
	if err != nil || len(jobs) == 0 {
		return errors.Join(err, errors.New("Plan8 real model did not execute its selected dashboard"))
	}
	for _, job := range jobs {
		if job.DashboardID.String() != h.DashboardID || job.Status != "complete" || job.Manifest == nil || len(job.Files) == 0 || len(job.Manifest.Execution.Resources) != 1 || job.Manifest.Execution.Resources[0].ID.String() != h.HostID {
			return fmt.Errorf("Plan8 real model result escaped the selected Host or lacks actual Workspace files")
		}
	}
	if err = a.planV2VerifyConclusion(ctx, env, conversation, sample.RunID, jobs); err != nil {
		return err
	}
	if sample.RunStatus != "succeeded" || !sample.UsageComplete || !sample.CachedUsageComplete || sample.ModelCalls > 32 {
		return fmt.Errorf("Plan8 real model run or token accounting incomplete")
	}
	sample.Passed = true
	raw, err := json.MarshalIndent(map[string]any{"replay": false, "model": env.Options.RealModel.ModelID, "sample": sample, "jobs": jobs, "host_id": h.HostID, "dashboard_id": h.DashboardID, "workspace_conclusion_verified": true}, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-model-dashboard-workspace.json"), raw)
}
