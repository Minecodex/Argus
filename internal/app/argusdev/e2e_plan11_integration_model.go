package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/monitoring"
)

// Data-only admission never dispatches external inference. Actual-model target
// acceptance uses the ordinary production endpoint policy and natural prompts.
func (a *App) plan11IntegrationBusiness(ctx context.Context, env *E2EEnvironment, target plan8ExtraTarget, collection monitoring.Collection) (proof map[string]any, failure error) {
	proof = map[string]any{"status": "not_run", "actual_model_evaluated": false, "scope": "data-only admission; actual model/Chatbox acceptance is a separate phase"}
	if env.Options.Plan8Phase != "plan11-targets" && env.Options.Plan8Phase != "plan11-review" {
		return proof, nil
	}
	name := "plan11-target-" + target.engine + "-zh-1"
	proof = map[string]any{"name": name, "passed": false, "business_passed": false, "actual_model_evaluated": false, "integration": target.engine, "resource_id": collection.Target.ID, "resource_type": "monitoring_target"}
	var conversation, run string
	var sample p5BenchmarkSample
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()
		if run != "" {
			failure = errors.Join(failure, a.planV2ModelEvidence(cleanup, env, conversation, sample, failure))
		}
		if failure != nil && run != "" {
			failure = errors.Join(failure, a.plan8StopFailedModelTask(cleanup, env, run))
		}
		if conversation != "" {
			if e := a.p5DeleteWorkspace(cleanup, env, conversation); e != nil {
				failure = errors.Join(failure, errPlan8TargetCleanup, e)
			}
		}
		proof["passed"] = failure == nil
		if failure != nil {
			proof["error"] = string(redactDiagnostic([]byte(failure.Error())))
		}
		data, e := json.MarshalIndent(proof, "", "  ")
		if e == nil {
			e = writePrivate(filepath.Join(env.Options.Artifacts, "plan11-target-"+target.engine+".json"), data)
		}
		failure = errors.Join(failure, e)
	}()
	window := plan11ComparisonWindow{To: time.Now().UTC().Truncate(time.Second)}
	window.From = window.To.Add(-5 * time.Minute)
	unit, err := a.plan11TargetUnit(ctx, env, collection, target.metric, window)
	if err != nil {
		return proof, err
	}
	proof["window"], proof["reference_unit"] = window, unit
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return proof, err
	}
	conversation, err = a.plan7ModelConversation(ctx, env, model, name)
	if err != nil {
		return proof, err
	}
	proof["conversation_id"] = conversation
	prompt := fmt.Sprintf("请找到当前有权访问的 %s 监控目标 %q，查看其实际采集状态、可用指标和来源。只分析固定 UTC 窗口 %s 到 %s 的原始指标 %s，查询步长为60秒。实际读取 Workspace 查询文件并离线统计，导出一份简明中文 Markdown 报告。回答和报告末尾必须有相同的 JSON 证据块：resource_id、metric（仅原始名称）、point_count、first_point_at、last_point_at、unit、value_min、value_max、display_unit、display_transform、display_min、display_max、limitations（非空字符串数组）。原始单位必须从授权指标目录取得，不猜测；本任务保留原值和原单位，display_transform 固定 identity，display_unit 与 unit 相同，不做百分比、速率或字节换算。Workspace 已有 argus-metric-evidence，可显式使用 --transform identity。返回的求值点不代表连续原始采样覆盖；解释多序列维度和未验证限制，不推测根因。不修改配置、不执行远程命令。", target.engine, target.name, window.From.Format(time.RFC3339), window.To.Format(time.RFC3339), target.metric)
	bounded, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var taskErr error
	sample, taskErr = a.realModelTask(bounded, env, conversation, name, prompt, nil, nil)
	run = sample.RunID
	proof["run_id"], proof["sample"] = run, sample
	proof["actual_model_evaluated"] = sample.ModelCalls > 0
	if run == "" {
		return proof, errors.Join(taskErr, errors.New("target task did not create a Run"))
	}
	trace, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer stop()
	raw, err := a.postgresQuery(trace, env, plan11ModelToolTraceSQL(run))
	if err != nil {
		return proof, err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan11-target-"+target.engine+"-calls.json"), []byte(raw)); err != nil {
		return proof, err
	}
	var calls []plan8ModelToolFact
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return proof, errors.New("target call trace invalid")
	}
	proof["calls"] = calls
	queryFiles, exportErr := a.plan11LoadQueryFiles(trace, env, conversation, run)
	proof["original_query_files"] = queryFiles
	if exportErr != nil {
		return proof, exportErr
	}
	reportFiles, exportErr := a.plan11LoadReportFiles(trace, env, conversation, run)
	proof["original_report_files"] = reportFiles
	if exportErr != nil {
		return proof, exportErr
	}
	invalid := 0
	for _, call := range calls {
		if call.Status != "succeeded" {
			invalid++
			continue
		}
		if call.Tool == "connect" || strings.HasPrefix(call.Tool, "mcp_") {
			return proof, fmt.Errorf("%w: target task executed an unrequested remote tool", errPlan11ComparisonBoundary)
		}
		if call.Tool == "tool.invoke" && call.Input.Category == "metric" && call.Input.Name == "query" && (!plan11ExactTargetArguments(call.Input.Arguments, collection.Target.ID.String()) || !plan11ArgumentsMatchWindow(call.Input.Arguments, window)) {
			return proof, fmt.Errorf("%w: target query expanded its exact typed resource/window", errPlan11ComparisonBoundary)
		}
	}
	proof["invalid_calls"] = invalid
	mutations, err := a.postgresQuery(trace, env, "SELECT count(*) FROM pending_actions WHERE run_id='"+run+"'")
	if err != nil {
		return proof, err
	}
	if strings.TrimSpace(mutations) != "0" {
		return proof, fmt.Errorf("%w: target task prepared an unrequested mutation", errPlan11ComparisonBoundary)
	}
	if taskErr != nil {
		return proof, taskErr
	}
	if sample.RunStatus != "succeeded" || sample.ModelCalls < 1 || sample.ModelCalls > 32 {
		return proof, errors.New("target task failed or exceeded its model call gate")
	}
	reply, err := a.planV2ModelReply(bounded, env, run)
	if err != nil {
		return proof, err
	}
	conclusion, err := plan11ParseConclusion(reply)
	if err != nil || conclusion.ResourceID != collection.Target.ID.String() || conclusion.Metric != target.metric || len(conclusion.Limitations) == 0 {
		return proof, errors.New("target answer has incomplete/wrong identity, metric or limitations")
	}
	evidence, err := a.plan11VerifyMeasuredQuery(bounded, env, conversation, run, collection.Target.ID.String(), raw, conclusion, unit, plan11MatchRawTargetConclusion)
	if err != nil {
		return proof, err
	}
	if !plan11ExactTargetResult(evidence, collection.Target.ID.String()) {
		return proof, fmt.Errorf("%w: sealed target result has different typed resource context", errPlan11ComparisonBoundary)
	}
	proof["query_evidence"], proof["conclusion"] = evidence, conclusion
	reports, err := a.plan11VerifyDeliveredReports(bounded, env, conversation, run, conclusion)
	if err != nil {
		return proof, err
	}
	proof["reports"], proof["business_passed"] = reports, true
	if err = a.refreshEnterpriseLogin(bounded, env); err != nil {
		return proof, err
	}
	cookies, err := existingPortalBrowserCookies(env)
	if err != nil {
		return proof, err
	}
	err = a.runPlaywright(bounded, env, "plan11-integration-discovery-real.spec.ts", map[string]string{"ARGUS_PLAN11_INTEGRATION_REAL": "1", "ARGUS_PLAN11_INTEGRATION_COOKIES": cookies, "ARGUS_PLAN11_INTEGRATION_CONVERSATION": conversation, "ARGUS_PLAN11_INTEGRATION_ID": collection.Target.ID.String(), "ARGUS_PLAN11_INTEGRATION_NAME": target.name, "ARGUS_PLAN11_INTEGRATION_METRIC": target.metric, "ARGUS_PLAN11_INTEGRATION_HASH": evidence["content_hash"].(string), "ARGUS_E2E_ARTIFACT_SUFFIX": "plan11-integration-" + target.engine})
	proof["chatbox_passed"] = err == nil
	if err != nil {
		return proof, err
	}
	if !sample.UsageComplete || !sample.CachedUsageComplete {
		return proof, errors.New("target provider usage incomplete")
	}
	sample.Passed = true
	proof["sample"] = sample
	proof["status"] = "passed"
	return proof, nil
}

func (a *App) plan11TargetUnit(ctx context.Context, env *E2EEnvironment, row monitoring.Collection, metric string, window plan11ComparisonWindow) (string, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", err
	}
	result, err := client.JSON(ctx, "plan11-target-reference-"+row.Integration, "enterprise", http.MethodPost, "/enterprise/telemetry/query/catalog", 200, map[string]any{"scope": map[string]any{"mode": "explicit", "resources": []map[string]string{{"type": "monitoring_target", "id": row.Target.ID.String()}}}, "signal": "metrics", "kind": "metrics", "metric": metric, "from": window.From, "to": window.To, "limit": 100, "observed_only": true}, enterpriseHeaders(env, ""))
	if err != nil {
		return "", err
	}
	reference, e := json.MarshalIndent(result, "", "  ")
	if e == nil {
		e = writePrivate(filepath.Join(env.Options.Artifacts, "plan11-target-reference-"+row.Integration+".json"), reference)
	}
	if e != nil {
		return "", e
	}
	if result["complete"] != true || result["has_more"] == true {
		return "", errors.New("target reference catalog incomplete")
	}
	rows, _ := result["metrics"].([]any)
	found, unit := false, ""
	for _, value := range rows {
		m, _ := value.(map[string]any)
		if m["name"] == metric && m["observed"] == true {
			u, _ := m["unit"].(string)
			if found && u != unit {
				return "", errors.New("target source unit ambiguous")
			}
			found, unit = true, u
		}
	}
	if !found {
		return "", errors.New("target original metric absent from actual catalog")
	}
	return unit, nil
}

func plan11IntegrationHasRef(value any, id string) bool {
	row, ok := value.(map[string]any)
	if !ok {
		return false
	}
	ref, _ := row["ref"].(map[string]any)
	return ref["type"] == "monitoring_target" && ref["id"] == id
}
func plan11ExactTargetArguments(args map[string]any, id string) bool {
	if _, typed := args["scope"]; !typed {
		ids, ok := args["resource_ids"].([]any)
		return ok && len(ids) == 1 && ids[0] == id
	}
	if _, legacy := args["resource_ids"]; legacy {
		return false
	}
	scope, _ := args["scope"].(map[string]any)
	refs, _ := scope["resources"].([]any)
	return scope["mode"] == "explicit" && len(refs) == 1 && plan11IntegrationHasRef(map[string]any{"ref": refs[0]}, id)
}

func plan11ExactTargetResult(evidence map[string]any, id string) bool {
	context, _ := evidence["query_context"].(map[string]any)
	refs, _ := context["resources"].([]any)
	return len(refs) == 1 && plan11IntegrationHasRef(map[string]any{"ref": refs[0]}, id)
}
