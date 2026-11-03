package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboardcontext"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Run serially before other stages create resources or change grants. The
// Workspace deliberately rejects old sources after authorization changes.
func (a *App) plan11TaskScopeCases(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) error {
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return err
	}
	general, err := a.plan7ModelConversation(ctx, env, model, "plan11-scope-general")
	if err != nil {
		return err
	}
	generalPrompt := fmt.Sprintf("请先读取并分析仪表盘 ID %s 对主机 %s（ID %s）的 CPU 和内存，再从该主机实际指标目录选择一项不在仪表盘中的网络指标直接查询。读取实际 Workspace 查询文件后说明结果。只做这台主机的综合诊断，不修改配置，不执行远程命令。", host.DashboardID, host.HostName, host.HostID)
	cases := []map[string]any{}
	var failures []error
	run := func(conversation, name, prompt, scope string, needsDashboard, needsMetric bool) {
		proof, err := a.plan11ScopeCase(ctx, env, host, conversation, name, prompt, scope, needsDashboard, needsMetric)
		cases = append(cases, proof)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
		}
	}
	run(general, "plan11-scope-general", generalPrompt, dashboardcontext.GeneralDiagnosis, true, true)
	restricted, err := a.plan7ModelConversation(ctx, env, model, "plan11-scope-restricted")
	if err != nil {
		return errors.Join(errors.Join(failures...), err)
	}
	run(restricted, "plan11-scope-restricted", fmt.Sprintf("/分析仪表盘 请只分析仪表盘 ID %s 对主机 %s（ID %s）的已发布 CPU 和内存查询，读取本轮查询文件后总结。不用自由指标查询、其他数据源或远程命令补查，不修改配置。", host.DashboardID, host.HostName, host.HostID), dashboardcontext.DashboardAnalysis, true, false)
	if plan11BusinessWorkflowPhase(env.Options.Plan8Phase) {
		failures = append(failures, a.plan11ScopeBrowser(ctx, env, restricted, dashboardcontext.DashboardAnalysis))
	}
	run(restricted, "plan11-scope-reset", fmt.Sprintf("/综合诊断 现在退出仅仪表盘分析。只读查询主机 %s（ID %s）最近30分钟的一项真实网络指标，读取实际 Workspace 文件再说明结果，不修改配置或执行远程命令。", host.HostName, host.HostID), dashboardcontext.GeneralDiagnosis, false, true)
	if plan11BusinessWorkflowPhase(env.Options.Plan8Phase) {
		failures = append(failures, a.plan11ScopeBrowser(ctx, env, restricted, dashboardcontext.GeneralDiagnosis))
	}
	body, _ := json.MarshalIndent(map[string]any{"passed": len(failures) == 0, "scope": "actual_model_task_scope_and_tool_execution_not_all_conclusion_quality", "cases": cases}, "", "  ")
	return errors.Join(errors.Join(failures...), writePrivate(filepath.Join(env.Options.Artifacts, "plan11-task-scopes.json"), body))
}

func (a *App) plan11ScopeBrowser(ctx context.Context, env *E2EEnvironment, conversation, scope string) error {
	cookies, err := existingPortalBrowserCookies(env)
	if err != nil {
		return err
	}
	return a.runPlaywright(ctx, env, "plan11-task-scope-real.spec.ts", map[string]string{"ARGUS_PLAN11_SCOPE_REAL": "1", "ARGUS_PLAN11_SCOPE_COOKIES": cookies, "ARGUS_PLAN11_SCOPE_CONVERSATION": conversation, "ARGUS_PLAN11_EXPECTED_SCOPE": scope, "ARGUS_E2E_ARTIFACT_SUFFIX": plan11ScopeBrowserSuffix(scope)})
}

func plan11ScopeBrowserSuffix(scope string) string {
	return "plan11-scope-" + strings.ReplaceAll(scope, "_", "-")
}

func (a *App) plan11ScopeCase(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop, conversation, name, prompt, scope string, needsDashboard, needsMetric bool) (proof map[string]any, failure error) {
	proof = map[string]any{"name": name, "conversation_id": conversation, "expected_scope": scope, "passed": false}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	sample, err := a.realModelTask(bounded, env, conversation, name, prompt, nil, nil)
	proof["sample"] = sample
	defer func() {
		if sample.RunID != "" {
			traceContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
			defer cancel()
			full, e := a.postgresQuery(traceContext, env, plan11InteractionTraceSQL(sample.RunID))
			if e == nil {
				e = writePlan11ToolEvidence(filepath.Join(env.Options.Artifacts, name+"-tools.json"), full)
			}
			failure = errors.Join(failure, e, a.plan11MetadataModelLedger(traceContext, env, name, sample.RunID, sample.ModelCalls))
			originals, exportErr := a.plan11LoadQueryFiles(traceContext, env, conversation, sample.RunID)
			proof["original_query_files"] = originals
			jobs, jobsErr := a.planV2ModelJobs(traceContext, env, conversation, sample.RunID)
			panels, panelErr := a.plan11LoadPanelFiles(traceContext, env, conversation, jobs)
			proof["dashboard_jobs"], proof["original_panel_files"] = jobs, panels
			failure = errors.Join(failure, exportErr, jobsErr, panelErr)
		}
		failure = errors.Join(failure, a.planV2ModelEvidence(ctx, env, conversation, sample, failure))
		if failure != nil && sample.RunID != "" {
			failure = errors.Join(failure, a.plan8StopFailedModelTask(ctx, env, sample.RunID))
		}
	}()
	if err != nil {
		return proof, err
	}
	if sample.RunStatus != "succeeded" || sample.ModelCalls < 1 || sample.ModelCalls > 32 {
		return proof, errors.New("scope model did not complete within its call gate")
	}
	actual, err := a.postgresQuery(bounded, env, "SELECT selection->>'task_scope' FROM dashboard_run_contexts WHERE run_id='"+sample.RunID+"'")
	if err != nil || strings.TrimSpace(actual) != scope {
		return proof, errors.New("persisted task scope differs from user admission")
	}
	raw, err := a.postgresQuery(bounded, env, plan11ModelToolTraceSQL(sample.RunID))
	if err != nil {
		return proof, err
	}
	var calls []plan8ModelToolFact
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return proof, errors.New("scope call evidence invalid")
	}
	acquired, queried, metric, read := false, false, false, false
	for _, call := range calls {
		if call.Status != "succeeded" {
			continue
		}
		read = read || call.Tool == "read" || call.Tool == "bash"
		if call.Tool == "connect" || strings.HasPrefix(call.Tool, "mcp_") {
			return proof, errors.New("read-only scope used a remote tool")
		}
		if call.Tool != "tool.invoke" {
			continue
		}
		category, name := call.Input.Category, call.Input.Name
		if category == "dashboard" && name == "get" {
			acquired = true
		}
		if category == "dashboard" && name == "query" {
			queried = true
		}
		if category == "metric" && name == "query" {
			if !plan11ExactHostArguments(call.Input.Arguments, host.HostID) {
				return proof, errors.New("scope query expanded the requested host")
			}
			if needsDashboard && !acquired {
				return proof, errors.New("general metric query preceded dashboard acquisition")
			}
			query, _ := call.Input.Arguments["query"].(string)
			id, parseErr := uuid.Parse(call.ID)
			if !strings.Contains(query, "system_network_") || parseErr != nil || !plan11ReadActualQuery(raw, "/workspace/"+toolruntime.NativeQueryFile("telemetry.promql.query", id)) {
				return proof, errors.New("scope task omitted its actual network query or corresponding file analysis")
			}
			metric = true
		}
		if scope == dashboardcontext.DashboardAnalysis && !plan11RestrictedBusiness(category, name) {
			return proof, errors.New("restricted analysis dispatched an unrelated business tool")
		}
	}
	if !read || needsDashboard && (!acquired || !queried) || needsMetric && !metric {
		return proof, errors.New("scope task lacks required query and actual file analysis")
	}
	if needsDashboard {
		if err = verifyTypedToolDiscoveryOrder(raw, "dashboard"); err != nil {
			return proof, err
		}
		jobs, e := a.planV2ModelJobs(bounded, env, conversation, sample.RunID)
		if e != nil || len(jobs) == 0 {
			return proof, errors.New("scope task has no actual dashboard job")
		}
		for _, job := range jobs {
			if job.DashboardID.String() != host.DashboardID || job.Manifest == nil || job.Status != "complete" || len(job.Files) == 0 || len(job.Manifest.Execution.Resources) != 1 || job.Manifest.Execution.Resources[0].Type != "host" || job.Manifest.Execution.Resources[0].ID.String() != host.HostID {
				return proof, errors.New("dashboard job escaped selection or did not seal files")
			}
		}
	}
	if needsMetric {
		if err = verifyTypedToolDiscoveryOrder(raw, "metric"); err != nil {
			return proof, err
		}
	}
	mutations, err := a.postgresQuery(bounded, env, "SELECT count(*) FROM pending_actions WHERE run_id='"+sample.RunID+"'")
	if err != nil || strings.TrimSpace(mutations) != "0" {
		return proof, errors.New("read-only scope prepared a mutation")
	}
	proof["scope_execution_passed"] = true
	if !sample.UsageComplete || !sample.CachedUsageComplete {
		return proof, errors.New("scope model usage incomplete")
	}
	sample.Passed = true
	proof["sample"] = sample
	proof["passed"] = true
	return proof, nil
}

func plan11RestrictedBusiness(category, name string) bool {
	if category == "observe" {
		return name == "resources.list"
	}
	if category == "workflow" {
		return name == "publish_file"
	}
	if category != "dashboard" {
		return false
	}
	switch name {
	case "list", "get", "catalog.resources", "context.resolve", "context.candidates", "query", "query.get", "query.cancel", "query.resume":
		return true
	}
	return false
}
