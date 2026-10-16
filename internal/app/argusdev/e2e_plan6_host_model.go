package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

func (a *App) plan6HostModelLoop(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) (failure error) {
	client, _ := scenarioHTTP(env)
	cfg := env.Options.RealModel
	request := cfg.modelRequest()
	request["name"] = "GLM-5.3 Docker Host loop"
	created, err := client.JSON(ctx, "host-glm-create", "enterprise", http.MethodPost, "/enterprise/ai-models/test-and-create", 201, request, enterpriseHeaders(env, "host-glm-create"))
	if err != nil {
		return err
	}
	if created["compatible"] != true {
		return fmt.Errorf("GLM provider incompatible")
	}
	h.ModelID, _ = stringField(created, "model", "id")
	if _, err = client.JSON(ctx, "host-glm-quota", "enterprise", http.MethodPost, "/enterprise/model-quotas", 200, map[string]any{"model_id": h.ModelID, "subject_type": "user", "subject_id": env.State.Values["admin_user_id"], "monthly_amount": cfg.MonthlyAmount}, enterpriseHeaders(env, "host-glm-quota")); err != nil {
		return err
	}
	if err = a.plan6HostModelRead(ctx, env, h, "active"); err != nil {
		return err
	}
	conversation, err := client.JSON(ctx, "host-dashboard-chat", "enterprise", http.MethodPost, "/conversations", 201, map[string]any{"title": "Docker Host dashboard analysis", "selected_model_id": h.ModelID}, enterpriseHeaders(env, "host-dashboard-chat"))
	if err != nil {
		return err
	}
	convo, _ := stringField(conversation, "id")
	before, err := a.plan6ConfigurationFacts(ctx, env)
	if err != nil {
		return err
	}
	prompt := fmt.Sprintf("请分析已选仪表盘中主机“%s”的 CPU 和内存图，只查询这台主机，不要查询堡垒机或其他集群。资源ID是%s。使用默认最近30分钟，说明这台主机的系统监控值、样本与范围限制。不要修改任何配置。%s", h.HostName, h.HostID, planV2ConclusionPrompt)
	sample, err := a.realModelTask(ctx, env, convo, "host-dashboard-analysis", prompt, nil, map[string]any{"mode": "analyze", "dashboard_ids": []string{h.DashboardID}})
	if uuid.Validate(sample.RunID) == nil {
		defer func() { failure = errors.Join(failure, a.planV2ModelEvidence(ctx, env, convo, sample, failure)) }()
	}
	if err != nil {
		return err
	}
	jobs, err := a.planV2ModelJobs(ctx, env, convo, sample.RunID)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return fmt.Errorf("GLM did not execute the selected Host dashboard")
	}
	for _, job := range jobs {
		if job.DashboardID.String() != h.DashboardID || job.Status != "complete" || job.Manifest == nil || len(job.Manifest.Execution.Resources) != 1 || job.Manifest.Execution.Resources[0].ID != uuid.MustParse(h.HostID) {
			return fmt.Errorf("GLM dashboard query escaped the selected real Host")
		}
		if len(job.Files) == 0 {
			return fmt.Errorf("GLM dashboard data was not delivered to Workspace")
		}
	}
	if err = a.planV2VerifyConclusion(ctx, env, convo, sample.RunID, jobs); err != nil {
		return err
	}
	after, err := a.plan6ConfigurationFacts(ctx, env)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("readonly model dashboard query changed alert configuration")
	}
	if sample.RunStatus != "succeeded" || !sample.UsageComplete || !sample.CachedUsageComplete {
		return fmt.Errorf("real GLM dashboard run or usage incomplete")
	}
	sample.Passed = true
	raw, _ := json.MarshalIndent(map[string]any{"model": cfg.ModelID, "replay": false, "passed": true, "sample": sample, "host_id": h.HostID, "dashboard_id": h.DashboardID, "query_jobs": jobs, "workspace_files_read": true, "exact_file_conclusion_verified": true}, "", "  ")
	env.State.Values["host_loop_dashboard_conversation"] = convo
	return writePrivate(filepath.Join(env.Options.Artifacts, "host-glm-dashboard.json"), raw)
}

func (a *App) plan6HostModelRead(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop, state string) error {
	client, _ := scenarioHTTP(env)
	conversation, err := client.JSON(ctx, "host-alert-chat-"+state, "enterprise", http.MethodPost, "/conversations", 201, map[string]any{"title": "Docker Host alerts " + state, "selected_model_id": h.ModelID}, enterpriseHeaders(env, "host-alert-chat-"+state))
	if err != nil {
		return err
	}
	convo, _ := stringField(conversation, "id")
	before, err := a.plan6ConfigurationFacts(ctx, env)
	if err != nil {
		return err
	}
	prompt := fmt.Sprintf("请查询主机“%s”（资源ID %s）的当前告警和最近恢复历史，核实 CPU 指标、告警评估健康、确认/认领/评论与屏蔽抑制状态。只检查这台主机，不修改配置，不把无数据或未查询当作健康。回答末尾用JSON代码块汇总本主机CPU告警策略的实际实例，结构为{\"host_id\":\"资源ID\",\"instances\":[{\"id\":\"实例ID\",\"lifecycle\":\"查询结果中的实际状态\",\"value\":告警实例state.value的实际数值或null}]}。当前和历史实例不要遗漏，不把另一次指标查询的数值替代实例记录值。", h.HostName, h.HostID)
	sample, err := a.realModelTask(ctx, env, convo, "host-alert-query-"+state, prompt, nil, nil)
	sampleRaw, _ := json.MarshalIndent(sample, "", "  ")
	err = errors.Join(err, writePrivate(filepath.Join(env.Options.Artifacts, "host-glm-alert-"+state+"-sample.json"), sampleRaw))
	if err != nil {
		return err
	}
	raw, err := a.postgresQuery(ctx, env, "SELECT coalesce(json_agg(json_build_object('tool',tool_id,'input',input,'output',(SELECT projection->'summary' FROM tool_results r WHERE r.tool_call_id=tool_calls.id ORDER BY created_at DESC LIMIT 1),'status',status,'source',source) ORDER BY created_at,call_sequence),'[]') FROM tool_calls WHERE run_id='"+sample.RunID+"';")
	if err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "host-glm-alert-"+state+"-tools.json"), []byte(raw)); err != nil {
		return err
	}
	if err = verifyPlan6DiscoveryOrder(raw); err != nil {
		return err
	}
	if err = verifyPlan6StateProjection(raw, h.PolicyID, "normal", false, state == "recovered", h.HostID); err != nil {
		return err
	}
	if !strings.Contains(raw, h.HostID) || !strings.Contains(raw, `"alert"`) {
		return fmt.Errorf("GLM did not read the selected Host alerts")
	}
	reply, err := a.planV2ModelReply(ctx, env, sample.RunID)
	if err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "host-glm-alert-"+state+"-reply.txt"), []byte(reply)); err != nil {
		return err
	}
	if err = plan6HostVerifyAlertConclusion(reply, raw, h.HostID, h.PolicyID, state); err != nil {
		return err
	}
	after, err := a.plan6ConfigurationFacts(ctx, env)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("readonly Host alert query changed configuration")
	}
	if sample.RunStatus != "succeeded" || !sample.UsageComplete || !sample.CachedUsageComplete {
		return fmt.Errorf("GLM Host alert query did not complete with real usage")
	}
	sample.Passed = true
	data, _ := json.MarshalIndent(map[string]any{"model": env.Options.RealModel.ModelID, "replay": false, "state": state, "host_id": h.HostID, "sample": sample, "passed": true, "no_business_mutation": true}, "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "host-glm-alert-"+state+".json"), data); err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "host-glm-alert-"+state+"-tools.json"), []byte(raw)); err != nil {
		return err
	}
	env.State.Values["host_loop_alert_conversation_"+state] = convo
	return nil
}

func (a *App) plan6HostBrowser(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) error {
	return a.runPlaywright(ctx, env, "e2e/plan6-host-real.spec.ts", map[string]string{"ARGUS_PLAN6_HOST_E2E": "1", "ARGUS_PLAN6_USERNAME": env.State.Values["enterprise_username"], "ARGUS_PLAN6_PASSWORD": env.State.Values["enterprise_password"], "ARGUS_PLAN6_HOST_ID": h.HostID, "ARGUS_PLAN6_HOST_NAME": h.HostName, "ARGUS_PLAN6_HOST_DASHBOARD_ID": h.DashboardID, "ARGUS_PLAN6_HOST_POLICY_ID": h.PolicyID, "ARGUS_PLAN6_HOST_ALERT_CONVERSATION": env.State.Values["host_loop_alert_conversation_recovered"], "ARGUS_PLAN6_HOST_DASHBOARD_CONVERSATION": env.State.Values["host_loop_dashboard_conversation"]})
}
