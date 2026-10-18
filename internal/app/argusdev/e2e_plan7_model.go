package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/remoteexecution"
)

func (a *App) plan7Model(ctx context.Context, env *E2EEnvironment) (string, error) {
	if id := env.State.Values["p7_real_model_id"]; id != "" {
		return id, nil
	}
	if env.Options.RealModel == nil {
		return "", fmt.Errorf("Plan7 requires the selected real model")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", err
	}
	request := env.Options.RealModel.modelRequest()
	request["name"] = "Plan7 real model"
	created, err := client.JSON(ctx, "p7-model-create", "enterprise", http.MethodPost, "/enterprise/ai-models/test-and-create", 201, request, enterpriseHeaders(env, "p7-model-create"))
	if err != nil {
		return "", err
	}
	if created["compatible"] != true {
		return "", fmt.Errorf("Plan7 real provider compatibility failed")
	}
	id, err := stringField(created, "model", "id")
	if err != nil {
		return "", err
	}
	_, err = client.JSON(ctx, "p7-model-quota", "enterprise", http.MethodPost, "/enterprise/model-quotas", 200, map[string]any{"model_id": id, "subject_type": "user", "subject_id": env.State.Values["admin_user_id"], "monthly_amount": env.Options.RealModel.MonthlyAmount}, enterpriseHeaders(env, "p7-model-quota"))
	if err == nil {
		env.State.Values["p7_real_model_id"] = id
	}
	return id, err
}
func (a *App) plan7ModelConversation(ctx context.Context, env *E2EEnvironment, model, title string) (string, error) {
	if err := a.plan7WorkspaceCapacity(ctx, env, title); err != nil {
		return "", err
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", err
	}
	value, err := client.JSON(ctx, "p7-chat-"+title, "enterprise", http.MethodPost, "/conversations", 201, map[string]any{"title": "Plan7 " + title, "selected_model_id": model}, enterpriseHeaders(env, "p7-chat-"+title))
	if err != nil {
		return "", err
	}
	return stringField(value, "id")
}
func (a *App) plan7SetMode(ctx context.Context, env *E2EEnvironment, conversation, mode string) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	current, err := client.JSON(ctx, "p7-mode-current", "enterprise", http.MethodGet, "/conversations/"+conversation, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	_, err = client.JSON(ctx, "p7-mode-"+mode, "enterprise", http.MethodPut, "/conversations/"+conversation, 200, map[string]any{"mode": mode, "expected_version": current["version"]}, enterpriseHeaders(env, "p7-mode-"+uuid.NewString()))
	return err
}
func (a *App) plan7StartModelMessage(ctx context.Context, env *E2EEnvironment, conversation, label, prompt string) (string, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", err
	}
	result, err := client.JSON(ctx, "p7-message-"+label, "enterprise", http.MethodPost, "/conversations/"+conversation+"/messages", 202, map[string]any{"content": prompt, "file_ids": []string{}}, enterpriseHeaders(env, "p7-message-"+label))
	if err != nil {
		return "", err
	}
	return stringField(result, "run", "run_id")
}
func (a *App) plan7FinishModel(ctx context.Context, env *E2EEnvironment, conversation, run, label, host string, maximumApprovals int, reviews ...func(remoteexecution.View) error) ([]remoteexecution.View, error) {
	return a.plan7FinishScopedModel(ctx, env, conversation, run, label, maximumApprovals, func(step remoteexecution.Step) bool {
		return step.Target.ResourceID.String() == host && step.Target.ResourceType == "host" && step.Interpreter == "bash"
	}, reviews...)
}

func (a *App) plan7FinishScopedModel(ctx context.Context, env *E2EEnvironment, conversation, run, label string, maximumApprovals int, allowedTarget func(remoteexecution.Step) bool, reviews ...func(remoteexecution.View) error) ([]remoteexecution.View, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return nil, err
	}
	approved := []remoteexecution.View{}
	seen := map[uuid.UUID]bool{}
	deadline := time.NewTimer(10 * time.Minute)
	defer deadline.Stop()
	for {
		requests, e := a.plan7Requests(ctx, env, conversation)
		if e != nil {
			return approved, e
		}
		for _, request := range requests {
			if request.Spec.RunID.String() != run || request.Status != "awaiting_confirmation" || seen[request.ID] {
				continue
			}
			if len(approved) >= maximumApprovals {
				return approved, fmt.Errorf("Plan7 model exceeded its explicit approval budget")
			}
			for _, step := range request.Spec.Steps {
				if allowedTarget == nil || !allowedTarget(step) {
					return approved, fmt.Errorf("Plan7 model requested a resource outside its explicit test scope")
				}
			}
			if e = a.plan7NoDispatchBeforeApproval(ctx, env, request); e != nil {
				return approved, e
			}
			for _, review := range reviews {
				if e = review(request); e != nil {
					if saveErr := savePlan7RejectedPreview(env, label, request); saveErr != nil {
						return approved, saveErr
					}
					return approved, e
				}
			}
			if e = a.plan7Approve(ctx, env, request); e != nil {
				return approved, e
			}
			seen[request.ID] = true
			approved = append(approved, request)
		}
		state, e := client.PollJSON(ctx, "p7-model-wait-"+label, "enterprise", "/runs/"+run, enterpriseHeaders(env, ""))
		if e != nil {
			return approved, e
		}
		switch state["status"] {
		case "succeeded":
			sample, e := a.p5BenchmarkSample(ctx, env, label, run)
			if e != nil {
				return approved, e
			}
			sample.Passed = sample.UsageComplete && sample.TotalTokens > 0 && sample.ModelCalls >= 1
			raw, _ := json.MarshalIndent(map[string]any{"scope": "provider completion, usage and approval-scope checks; domain outcomes are verified by the enclosing phase", "model": env.Options.RealModel.ModelID, "replay": false, "sample": sample, "approved_previews": approved}, "", "  ")
			e = writePrivate(filepath.Join(env.Options.Artifacts, "plan7-model-"+label+".json"), raw)
			if e != nil {
				return approved, e
			}
			if !sample.Passed {
				return approved, fmt.Errorf("Plan7 lacks real provider usage evidence")
			}
			return approved, nil
		case "failed", "cancelled", "timed_out":
			return approved, fmt.Errorf("Plan7 real model %s failed: %v/%v", label, state["error_code"], state["stop_reason"])
		}
		select {
		case <-ctx.Done():
			return approved, ctx.Err()
		case <-deadline.C:
			return approved, fmt.Errorf("Plan7 real model %s timed out", label)
		case <-time.After(time.Second):
		}
	}
}

// The target and absence of dispatch have already been checked. Keep the
// rejected owned-fixture preview private so a failure can be diagnosed without
// approving changed commands or weakening the exact-script check.
func savePlan7RejectedPreview(env *E2EEnvironment, label string, view remoteexecution.View) error {
	raw, err := json.MarshalIndent(map[string]any{"scope": "owned acceptance fixture; rejected before approval", "run_id": env.Options.RunID, "view": view}, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-model-"+label+"-rejected-preview.json"), raw)
}
func (a *App) plan7TelemetryModelFlow(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) error {
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return err
	}
	telemetry, err := a.plan7ModelConversation(ctx, env, model, "telemetry-analysis")
	if err != nil {
		return err
	}
	prompt := fmt.Sprintf("请分析已选仪表盘中主机 %s（资源 ID %s）的 CPU 和内存，使用已有监控数据并在 Workspace 复核，说明样本、时间范围和局限；此次不连接主机、不修复。%s", h.HostName, h.HostID, planV2ConclusionPrompt)
	sample, err := a.realModelTask(ctx, env, telemetry, "plan7-telemetry", prompt, nil, map[string]any{"mode": "analyze", "dashboard_ids": []string{h.DashboardID}})
	if err != nil {
		return err
	}
	if sample.RunStatus != "succeeded" || !sample.UsageComplete {
		return fmt.Errorf("Plan7 telemetry analysis did not complete with real usage")
	}
	jobs, err := a.planV2ModelJobs(ctx, env, telemetry, sample.RunID)
	if err != nil || len(jobs) == 0 {
		return fmt.Errorf("Plan7 model did not query telemetry: %w", err)
	}
	if err = a.planV2VerifyConclusion(ctx, env, telemetry, sample.RunID, jobs); err != nil {
		return err
	}
	requests, err := a.plan7Requests(ctx, env, telemetry)
	if err != nil || len(requests) != 0 {
		return fmt.Errorf("Plan7 telemetry-only analysis unnecessarily requested remote execution")
	}
	sample.Passed = true
	if err = a.planV2ModelEvidence(ctx, env, telemetry, sample, nil); err != nil {
		return err
	}
	env.State.Values["p7_telemetry_conversation"] = telemetry
	return nil
}

func (a *App) plan7RealModelFlow(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) error {
	if err := a.plan7TelemetryModelFlow(ctx, env, h); err != nil {
		return err
	}
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return err
	}
	fault, err := a.preparePlan7ServiceFault(ctx, env, h)
	if err != nil {
		return err
	}
	root := fault.Directory
	conversation, err := a.plan7ModelConversation(ctx, env, model, "diagnosis-and-repair")
	if err != nil {
		return err
	}
	diagnosis := fmt.Sprintf("只排查主机资源 %s 上本轮专用 systemd 服务 %s 和健康检查 %s/health。服务当前失败，配置在 %s/workers，正确值应为 2。请取得现场状态、相关日志和配置证据，在 Workspace 复核并解释原因。此阶段不要修改配置或重启服务；每次远程取证等待确认。取证时保留预期的失败退出码及输出，不要把失败状态当作取证命令没有执行。不要操作其他服务、其他资源或读取平台身份文件。", h.HostID, fault.Unit, root, root)
	run, err := a.plan7StartModelMessage(ctx, env, conversation, "diagnosis", diagnosis)
	if err != nil {
		return err
	}
	approvals, err := a.plan7FinishModel(ctx, env, conversation, run, "diagnosis", h.HostID, 3)
	if err != nil {
		return err
	}
	if len(approvals) == 0 {
		return fmt.Errorf("Plan7 model guessed the remote fault without evidence")
	}
	value, err := a.plan6DockerScript(ctx, h.Docker, h.Docker.Host, "cat "+root+"/workers\n")
	if err != nil || strings.TrimSpace(value) != "0" {
		return fmt.Errorf("diagnosis changed the test configuration")
	}
	if err = a.plan7SetMode(ctx, env, conversation, "plan"); err != nil {
		return err
	}
	run, err = a.plan7StartModelMessage(ctx, env, conversation, "repair-plan", "根据已经确认的现场证据生成修复计划，将 workers 改为 2，重启本轮专用服务 "+fault.Unit+"，验证进程实际运行及 health。先展示完整计划，等待确认后执行；文件操作只限该测试目录，不得操作其他服务。")
	if err != nil {
		return err
	}
	original, err := a.plan7WaitRequest(ctx, env, conversation, 1, func(v remoteexecution.View) bool {
		return v.Spec.RunID.String() == run && v.Status == "awaiting_confirmation"
	}, run)
	if err != nil {
		return err
	}
	if original.Spec.Kind != "plan" {
		if err = plan7ModelPreviewEvidence(env, "repair-plan-original", original); err != nil {
			return err
		}
		return fmt.Errorf("plan mode did not produce a fixed plan")
	}
	if err = plan7ModelPreviewEvidence(env, "repair-plan-original", original); err != nil {
		return err
	}
	client, _ := scenarioHTTP(env)
	if _, err = client.JSON(ctx, "p7-model-feedback", "enterprise", http.MethodPost, "/conversations/"+conversation+"/remote-executions/"+original.ID.String()+"/feedback", 202, map[string]any{"feedback": "请先把 workers 备份为同目录的 workers.bak，再修改配置并验证 health。其他目标和限制保持不变。"}, enterpriseHeaders(env, "p7-model-feedback")); err != nil {
		return err
	}
	revised, err := a.plan7WaitRequest(ctx, env, conversation, 2, func(v remoteexecution.View) bool {
		return v.Spec.RunID.String() == run && v.Status == "awaiting_confirmation" && v.ID != original.ID
	}, run)
	if err != nil {
		return err
	}
	if err = plan7ModelPreviewEvidence(env, "repair-plan-revised", revised); err != nil {
		return err
	}
	if revised.Spec.SupersedesID == nil || *revised.Spec.SupersedesID != original.ID || revised.Revision != original.Revision+1 {
		return fmt.Errorf("model feedback lost its revision relationship")
	}
	approvals, err = a.plan7FinishModel(ctx, env, conversation, run, "repair", h.HostID, 2)
	if err != nil {
		return err
	}
	if len(approvals) == 0 {
		return fmt.Errorf("Plan7 model repair had no approval")
	}
	if err = a.verifyPlan7ServiceRecovery(ctx, env, h, fault); err != nil {
		return err
	}
	env.State.Values["p7_model_conversation"] = conversation
	return nil
}

func plan7ModelPreviewEvidence(env *E2EEnvironment, label string, view remoteexecution.View) error {
	raw, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-model-"+label+"-preview.json"), raw)
}
