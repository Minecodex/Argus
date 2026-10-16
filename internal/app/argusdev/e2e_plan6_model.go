package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"path/filepath"
	"strings"
)

func (a *App) runPlan6RealModel(ctx context.Context, env *E2EEnvironment) (failure error) {
	cfg := env.Options.RealModel
	if cfg == nil {
		return fmt.Errorf("selected real model missing")
	}
	client, _ := scenarioHTTP(env)
	request := cfg.modelRequest()
	request["name"] = "Plan6 GLM-5.3"
	created, err := client.JSON(ctx, "p6-real-model", "enterprise", http.MethodPost, "/enterprise/ai-models/test-and-create", 201, request, enterpriseHeaders(env, "p6-real-model"))
	if err != nil {
		return err
	}
	if created["compatible"] != true {
		return fmt.Errorf("Plan6 model compatibility failed")
	}
	modelID, err := stringField(created, "model", "id")
	if err != nil {
		return err
	}
	env.State.Values["p6_model_id"] = modelID
	if _, err = client.JSON(ctx, "p6-model-quota", "enterprise", http.MethodPost, "/enterprise/model-quotas", 200, map[string]any{"model_id": modelID, "subject_type": "user", "subject_id": env.State.Values["admin_user_id"], "monthly_amount": cfg.MonthlyAmount}, enterpriseHeaders(env, "p6-model-quota")); err != nil {
		return err
	}
	report := map[string]any{"model": cfg.ModelID, "protocol": cfg.Protocol, "replay": false, "samples": []p5BenchmarkSample{}, "passed": false}
	defer func() {
		report["passed"] = failure == nil
		if failure != nil {
			report["failure_reason"] = failure.Error()
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "plan6-real-model.json"), data))
	}()
	cluster := env.State.Values["m3_cluster_id"]
	resource, err := client.JSON(ctx, "p6-model-cluster", "enterprise", http.MethodGet, "/enterprise/kubernetes-clusters/"+cluster, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	name, _ := stringField(resource, "name")
	tasks := []struct {
		id, prompt string
		confirm    bool
	}{
		{"query", fmt.Sprintf("请查询集群%s 的 CPU 指标和相关告警。告诉我当前有没有异常，并区分无数据、评估失败与正常。", name), false},
		{"exists", "请检查是否已经有名为“Plan6 real pressure”的告警策略，并说明它的评估健康。", false},
		{"missing", "请检查名为“Plan6 不存在的策略”的规则是否存在，不存在时明确告诉我。", false},
		{"create", fmt.Sprintf("请为集群%s 创建名为“Plan6 模型双指标策略”的告警。CPU 指标 argus_plan6_cpu_percent 超过85%%时严重告警，75%%以下恢复；内存指标 argus_plan6_memory_percent 超过80%%时警告，70%%以下恢复。两条规则单位均为百分比。两个指标各自独立触发，每10秒评估，触发与恢复持续时间均为0秒；无数据创建健康告警，查询失败保留活跃告警。通知发送到已有的 Plan6 feishu 渠道。生成完整预览，等我确认后提交。", name), true},
		{"modify", "请把已有“Plan6 模型双指标策略”的 CPU 阈值改为90，恢复阈值仍为75。只改 CPU 阈值，其他所有配置字段（包括描述、规则名称、范围、其他规则和通知）保持原样。先给我预览。", true},
		{"silence", fmt.Sprintf("请创建名为“Plan6 模型屏蔽”的临时屏蔽，只匹配集群%s 的 argus_plan6_cpu_percent 指标。即刻生效，一小时后结束，原因是发布验证。先给我预览。", name), true},
		{"unsilence", "请解除已有名为“Plan6 模型屏蔽”的临时屏蔽，先给我预览。", true},
		{"disable", "请停用已有“Plan6 模型双指标策略”，其他配置保持原样。先给我预览。", true},
		{"enable", "请重新启用已有“Plan6 模型双指标策略”，其他配置保持原样。先给我预览。", true},
		{"cancel", fmt.Sprintf("请为集群%s 的内存指标 argus_plan6_memory_percent 创建一个名为“Plan6 取消方案”的一小时临时屏蔽，原因是取消流程测试。先预览，等我确认。", name), false},
	}
	samples := []p5BenchmarkSample{}
	for _, task := range tasks {
		report["failed_task"] = task.id
		before, err := a.plan6ConfigurationFacts(ctx, env)
		if err != nil {
			return err
		}
		conversation, err := client.JSON(ctx, "p6-conversation-"+task.id, "enterprise", http.MethodPost, "/conversations", 201, map[string]any{"title": "Plan6 " + task.id, "selected_model_id": modelID}, enterpriseHeaders(env, "p6-conversation-"+task.id))
		if err != nil {
			return err
		}
		conversationID, _ := stringField(conversation, "id")
		env.State.Values["p6_conversation_"+task.id] = conversationID
		sample, err := a.realModelTask(ctx, env, conversationID, "p6-"+task.id, task.prompt, nil, nil)
		if err != nil {
			return err
		}
		if uuid.Validate(sample.RunID) != nil {
			return fmt.Errorf("invalid model run identity")
		}
		sample.Passed = false
		samples = append(samples, sample)
		report["samples"] = samples
		evidence, err := a.postgresQuery(ctx, env, "SELECT coalesce(json_agg(json_build_object('id',id,'tool',tool_id,'input',input,'sequence',call_sequence,'status',status,'source',source,'error_code',error_code) ORDER BY created_at,call_sequence), '[]') FROM tool_calls WHERE run_id='"+sample.RunID+"';")
		if err != nil {
			return err
		}
		if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan6-tools-"+task.id+".json"), []byte(strings.TrimSpace(evidence))); err != nil {
			return err
		}
		if err = verifyPlan6DiscoveryOrder(evidence); err != nil {
			return fmt.Errorf("model task %s: %w", task.id, err)
		}
		if err = a.verifyPlan6ModelReads(ctx, env, task.id, sample.RunID); err != nil {
			return err
		}
		afterPreview, err := a.plan6ConfigurationFacts(ctx, env)
		if err != nil {
			return err
		}
		if before != afterPreview {
			return fmt.Errorf("model task %s changed configuration before host confirmation", task.id)
		}
		if task.id == "query" || task.id == "exists" || task.id == "missing" {
			sample.Passed = sample.RunStatus == "succeeded"
		} else {
			action, err := a.postgresQuery(ctx, env, "SELECT action_ref FROM pending_actions WHERE run_id='"+sample.RunID+"' AND action_type LIKE 'alert.%' AND status='awaiting_confirmation' ORDER BY created_at DESC LIMIT 1;")
			if err != nil {
				return err
			}
			action = strings.TrimSpace(action)
			if action == "" {
				return fmt.Errorf("model task %s did not produce a host confirmation", task.id)
			}
			if err = a.verifyPlan6ModelPreview(ctx, env, task.id, action, before); err != nil {
				return err
			}
			if task.confirm {
				if _, err = a.confirmPendingAction(ctx, env, "p6-model-confirm-"+task.id, action); err != nil {
					return err
				}
			} else {
				if _, err = client.JSON(ctx, "p6-model-cancel", "enterprise", http.MethodPost, "/enterprise/pending-actions/"+action+"/cancel", 200, nil, enterpriseHeaders(env, "p6-model-cancel")); err != nil {
					return err
				}
			}
			sample.Passed = sample.RunStatus == "waiting_input"
			if err = a.verifyPlan6ModelBusiness(ctx, env, task.id, action, before); err != nil {
				return err
			}
		}
		samples[len(samples)-1] = sample
		report["samples"] = samples
		if !sample.Passed || !sample.UsageComplete {
			return fmt.Errorf("Plan6 real model task %s or usage accounting failed", task.id)
		}
		delete(report, "failed_task")
	}
	return a.verifyPlan6ModelStates(ctx, env, modelID, report)
}

func verifyPlan6DiscoveryOrder(evidence string) error {
	var calls []struct {
		Tool      string                          `json:"tool"`
		Status    string                          `json:"status"`
		ErrorCode string                          `json:"error_code"`
		Input     struct{ Category, Name string } `json:"input"`
	}
	if err := json.Unmarshal([]byte(evidence), &calls); err != nil {
		return err
	}
	described, searched, invoked := map[string]bool{}, false, false
	for _, call := range calls {
		key := call.Input.Category + ":" + call.Input.Name
		switch call.Tool {
		case "tool.search":
			searched = searched || call.Status == "succeeded" && call.Input.Category == "alert"
		case "tool.describe":
			if call.Status == "succeeded" && call.Input.Category == "alert" && searched {
				described[key] = true
			}
		case "tool.invoke":
			if call.Input.Category != "alert" {
				continue
			}
			if call.Status == "failed" && call.ErrorCode == "TOOL_DESCRIPTION_REQUIRED" {
				// The runtime rejected this attempt before business dispatch.
				// Only actual dispatched calls can satisfy the invocation gate.
				continue
			}
			if strings.HasSuffix(call.Input.Name, ".commit") || !described[key] {
				return fmt.Errorf("alert invocation %s lacked prior authorized search/describe", key)
			}
			invoked = invoked || call.Status == "succeeded"
		}
	}
	if !searched || !invoked {
		return fmt.Errorf("model did not autonomously search, describe and invoke an alert tool")
	}
	return nil
}
