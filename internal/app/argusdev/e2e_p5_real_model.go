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
)

type p5BenchmarkReport struct {
	Kind        string              `json:"kind"`
	Model       string              `json:"model"`
	Protocol    string              `json:"api_protocol"`
	Planned     int                 `json:"planned_tasks"`
	Passed      bool                `json:"passed"`
	Completed   int                 `json:"completed_tasks"`
	Succeeded   int                 `json:"successful_tasks"`
	SuccessRate float64             `json:"task_success_rate"`
	TotalTokens int64               `json:"total_tokens"`
	ModelCalls  int                 `json:"model_calls"`
	Samples     []p5BenchmarkSample `json:"samples"`
}

// Real tasks are natural-language messages, never Replay plans or scripted tool
// calls. Oracles use persisted tool facts and downloaded file contents.
func (a *App) runP5RealModelBenchmark(ctx context.Context, env *E2EEnvironment) (returnErr error) {
	config := env.Options.RealModel
	report := p5BenchmarkReport{Kind: "real-model-zh-functional-benchmark/v1", Model: config.ModelID, Protocol: config.Protocol, Planned: 6, Samples: []p5BenchmarkSample{}}
	defer func() {
		report.Passed = report.Passed && returnErr == nil
		report.Completed = len(report.Samples)
		for _, sample := range report.Samples {
			if sample.Passed {
				report.Succeeded++
			}
			report.TotalTokens += sample.TotalTokens
			report.ModelCalls += sample.ModelCalls
		}
		report.SuccessRate = float64(report.Succeeded) / float64(report.Planned)
		data, err := json.MarshalIndent(report, "", "  ")
		if err == nil {
			err = writePrivate(filepath.Join(env.Options.Artifacts, "p5-real-model-statistics.json"), append(data, '\n'))
		}
		returnErr = errors.Join(returnErr, err)
	}()
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	if err := config.validate(); err != nil {
		return err
	}
	created, err := client.JSON(ctx, "p5-real-model-create", "enterprise", http.MethodPost, "/enterprise/ai-models/test-and-create", 201, config.modelRequest(), enterpriseHeaders(env, "p5-real-model-create"))
	if err != nil {
		return err
	}
	if created["compatible"] != true {
		return fmt.Errorf("P5 real model failed compatibility checks")
	}
	model, err := stringField(created, "model", "id")
	if err != nil {
		return err
	}
	if _, err := client.JSON(ctx, "p5-real-model-quota", "enterprise", http.MethodPost, "/enterprise/model-quotas", 200, map[string]any{"model_id": model, "subject_type": "user", "subject_id": env.State.Values["admin_user_id"], "monthly_amount": config.MonthlyAmount}, enterpriseHeaders(env, "p5-real-model-quota")); err != nil {
		return err
	}
	previousModel := env.State.Values["m4_model_id"]
	env.State.Values["m4_model_id"] = model
	defer func() { env.State.Values["m4_model_id"] = previousModel }()
	_, _ = fmt.Fprintln(a.stdout, "P5 real Chinese model benchmark started (six tasks)")
	for _, task := range []struct{ id, prompt, category string }{
		{"hosts", "请实际查询我有权限查看的全部主机，告诉我有哪些主机及连接状态；没有数据就明确说明没有，不要创建资源。", "host"},
		{"kubernetes", "请实际查询当前可访问的 Kubernetes 集群清单，汇总集群及连接状态；空清单请如实说明，不要变更资源。", "k8s"},
		{"connectors", "请实际查询当前企业的 Connector 清单，汇总名称和在线情况；没有数据请如实说明，不要变更资源。", "connector"},
	} {
		conversation, err := a.p5Conversation(ctx, env, "real-"+task.id)
		if err != nil {
			return err
		}
		sample, err := a.p5RealTask(ctx, env, conversation, task.id, task.prompt, nil)
		if err != nil {
			return err
		}
		sample.Passed = sample.RunStatus == "succeeded" && sample.NativeLists[task.category] > 0
		report.Samples = append(report.Samples, sample)
	}
	conversation, err := a.p5Conversation(ctx, env, "real-files")
	if err != nil {
		return err
	}
	content := []byte("region,amount\nEast,10\nWest,7\nEast,20\nWest,3\n")
	upload, err := client.JSON(ctx, "p5-real-upload-create", "enterprise", http.MethodPost, "/conversations/"+conversation+"/workspace/uploads", 201, map[string]any{"name": "sales.csv", "byte_size": len(content)}, enterpriseHeaders(env, "p5-real-upload-create"))
	if err != nil {
		return err
	}
	defer func(workspaceConversation string) {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		returnErr = errors.Join(returnErr, a.p5DeleteWorkspace(cleanupCtx, env, workspaceConversation))
	}(conversation)
	uploadID, err := stringField(upload, "id")
	if err != nil {
		return err
	}
	data, _, err := p5FileHTTP(ctx, client, env, http.MethodPut, "/conversations/"+conversation+"/workspace/uploads/"+uploadID+"/content", content, 201, "")
	if err != nil {
		return err
	}
	var file map[string]any
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	fileID, err := stringField(file, "id")
	if err != nil {
		return err
	}
	for _, task := range []struct {
		id, prompt string
		factor     int
		files      []string
	}{
		{"sales", "请用 Python 实际分析附件 sales.csv，按 region 汇总 amount。将结果保存并发布为可下载的 summary.csv，只含 region,total 两列，每个 region 一行；不要仅在回复中展示文件路径。", 1, []string{fileID}},
		{"sales-followup", "请基于刚才同一工作区里的 summary.csv，将各地区的 total 乘以 2，保存为 doubled.csv 并发布供下载，保持 region,total 两列。", 2, nil},
	} {
		sample, err := a.p5RealTask(ctx, env, conversation, task.id, task.prompt, task.files)
		if err != nil {
			return err
		}
		valid, err := a.p5BenchmarkDelivery(ctx, env, conversation, sample.RunID, task.factor)
		if err != nil {
			return err
		}
		sample.Passed = sample.RunStatus == "succeeded" && valid
		report.Samples = append(report.Samples, sample)
	}
	connection, err := a.p5MCPConnection(ctx, env, "real-benchmark", "mode=json&auth=bearer", "bearer")
	if err != nil {
		return err
	}
	connectionID, err := stringField(connection, "id")
	if err != nil {
		return err
	}
	conversation, err = a.p5Conversation(ctx, env, "real-external-mcp")
	if err != nil {
		return err
	}
	if err := a.p5SelectConnections(ctx, env, conversation, []string{connectionID}); err != nil {
		return err
	}
	sample, err := a.p5RealTask(ctx, env, conversation, "external-mcp", "请通过本会话选择的 MCP 连接读取计数器当前的 writes 值并告诉我结果。只读，不要增加或重置计数器。", nil)
	if err != nil {
		return err
	}
	if sample.ExternalCalls > 0 {
		counter, err := a.p5Counter(ctx, env, sample.RunID)
		if err == nil {
			sample.Passed = sample.RunStatus == "succeeded" && counter == 0
		}
	}
	report.Samples = append(report.Samples, sample)
	report.Passed = len(report.Samples) == report.Planned
	for _, sample := range report.Samples {
		report.Passed = report.Passed && sample.Passed && sample.UsageComplete
	}
	if !report.Passed {
		return fmt.Errorf("P5 real-model functional benchmark failed; see p5-real-model-statistics.json")
	}
	_, _ = fmt.Fprintln(a.stdout, "P5 real Chinese model benchmark passed")
	return nil
}

func (a *App) p5RealTask(ctx context.Context, env *E2EEnvironment, conversation, id, prompt string, files []string) (p5BenchmarkSample, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return p5BenchmarkSample{}, err
	}
	if files == nil {
		files = []string{}
	}
	response, err := client.JSON(ctx, "p5-real-message-"+id, "enterprise", http.MethodPost, "/conversations/"+conversation+"/messages", 202, map[string]any{"content": prompt, "file_ids": files}, enterpriseHeaders(env, "p5-real-message-"+id))
	if err != nil {
		return p5BenchmarkSample{}, err
	}
	run, err := stringField(response, "run", "run_id")
	if err != nil {
		return p5BenchmarkSample{}, err
	}
	deadline := time.NewTimer(10 * time.Minute)
	defer deadline.Stop()
	for {
		state, err := client.JSON(ctx, "p5-real-wait-"+id, "enterprise", http.MethodGet, "/runs/"+run, 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return p5BenchmarkSample{}, err
		}
		switch state["status"] {
		case "succeeded", "failed", "cancelled", "timed_out", "waiting_input", "waiting_approval":
			return a.p5BenchmarkSample(ctx, env, id, run)
		}
		select {
		case <-ctx.Done():
			return p5BenchmarkSample{}, ctx.Err()
		case <-deadline.C:
			return p5BenchmarkSample{}, fmt.Errorf("P5 real-model task %s exceeded ten minutes", id)
		case <-time.After(time.Second):
		}
	}
}

func (a *App) p5DeleteWorkspace(ctx context.Context, env *E2EEnvironment, conversation string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	if _, err := client.JSON(ctx, "p5-real-workspace-delete", "enterprise", http.MethodDelete, "/conversations/"+conversation+"/workspace", 202, nil, enterpriseHeaders(env, "p5-real-workspace-delete")); err != nil {
		return err
	}
	for {
		status, err := a.postgresQuery(ctx, env, "SELECT status FROM workspaces WHERE conversation_id='"+conversation+"' ORDER BY created_at DESC LIMIT 1;")
		if err != nil {
			return err
		}
		if strings.TrimSpace(status) == "deleted" {
			return nil
		}
		if err := waitContext(ctx, time.Second); err != nil {
			return err
		}
	}
}
