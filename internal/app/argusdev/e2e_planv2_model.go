package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/prometheus/prometheus/promql/parser"
)

// Reuses the strict, secret-reference-only P5 model configuration and provider
// usage accounting. Natural language tasks do not contain Replay tool plans.
func (a *App) runPlanV2RealModel(ctx context.Context, env *E2EEnvironment) (failure error) {
	cfg := env.Options.RealModel
	if err := cfg.validate(); err != nil {
		return err
	}
	groups := env.Options.PlanV2ModelGroups
	if len(groups) == 0 {
		groups = []string{"core", "authoring", "conditions", "failures"}
	}
	planned := 0
	for _, g := range groups {
		planned += map[string]int{"core": 4, "authoring": 1, "conditions": 4, "failures": 4}[g]
	}
	report := p5BenchmarkReport{Kind: "planv2-real-model-functional/v3", Model: cfg.ModelID, Protocol: cfg.Protocol, Groups: groups, Planned: planned, Samples: []p5BenchmarkSample{}}
	defer func() {
		report.Completed = len(report.Samples)
		for _, sample := range report.Samples {
			if sample.Passed {
				report.Succeeded++
			}
			report.TotalTokens += sample.TotalTokens
			report.ModelCalls += sample.ModelCalls
		}
		report.Passed = failure == nil && report.Succeeded == report.Planned
		report.SuccessRate = float64(report.Succeeded) / float64(report.Planned)
		body, _ := json.MarshalIndent(report, "", "  ")
		failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "planv2-real-model.json"), body))
	}()
	client, _ := scenarioHTTP(env)
	request := cfg.modelRequest()
	request["name"] = "PlanV2 real model"
	created, err := client.JSON(ctx, "p2-real-model", "enterprise", http.MethodPost, "/enterprise/ai-models/test-and-create", 201, request, enterpriseHeaders(env, "p2-real-model"))
	if err != nil {
		return err
	}
	if created["compatible"] != true {
		return fmt.Errorf("PlanV2 model compatibility failed")
	}
	model, err := stringField(created, "model", "id")
	if err != nil {
		return err
	}
	if _, err := client.JSON(ctx, "p2-real-model-quota", "enterprise", http.MethodPost, "/enterprise/model-quotas", 200, map[string]any{"model_id": model, "subject_type": "user", "subject_id": env.State.Values["admin_user_id"], "monthly_amount": cfg.MonthlyAmount}, enterpriseHeaders(env, "p2-real-model-quota")); err != nil {
		return err
	}
	previous := env.State.Values["m4_model_id"]
	env.State.Values["p2_fixture_replay_model_id"] = previous
	env.State.Values["m4_model_id"] = model
	defer func() { env.State.Values["m4_model_id"] = previous }()
	caseErrors := []error{}
	if slices.Contains(groups, "core") {
		caseErrors = append(caseErrors, a.planV2ModelCore(ctx, env, &report))
	}
	for _, suite := range []struct {
		name string
		run  func(context.Context, *E2EEnvironment) ([]p5BenchmarkSample, error)
	}{{"authoring", a.planV2ModelAuthoring}, {"conditions", a.planV2ModelConditions}, {"failures", a.planV2ModelFailures}} {
		if !slices.Contains(groups, suite.name) {
			continue
		}
		samples, suiteErr := suite.run(ctx, env)
		report.Samples = append(report.Samples, samples...)
		caseErrors = append(caseErrors, suiteErr)
	}
	return errors.Join(caseErrors...)
}

func (a *App) planV2ModelCore(ctx context.Context, env *E2EEnvironment, report *p5BenchmarkReport) (failure error) {
	selection, selectionErr := a.verifyPlanV2ModelSelection(ctx, env)
	report.Samples = append(report.Samples, selection)
	caseErrors := []error{selectionErr}
	for _, mode := range []string{"builder", "dsl"} {
		convo, err := a.p5Conversation(ctx, env, "PlanV2 real create "+mode)
		if err != nil {
			return err
		}
		name := "PlanV2 model " + mode + " " + uuid.NewString()
		prompt := fmt.Sprintf("创建仪表盘，名称必须是 %s。先核实真实指标目录。添加一张 OTLP 来源的指标趋势图，查询 argus_m7_e2e_gauge_planv2，默认最近一小时，使用 %s 编辑来源。不要添加变量或额外图。生成草稿并提供发布预览，等待我确认；不要自行确认发布。", name, mode)
		sample, err := a.realModelTask(ctx, env, convo, "p2-create-"+mode, prompt, nil, map[string]any{"mode": "create", "dashboard_ids": []string{}})
		if err != nil {
			return err
		}
		valid, err := a.verifyPlanV2ModelCreation(ctx, env, sample.RunID, name, mode)
		if err == nil && valid {
			if waitErr := a.waitRunTerminal(ctx, env, sample.RunID); waitErr != nil {
				err = waitErr
			} else {
				sample, err = a.p5BenchmarkSample(ctx, env, "p2-create-"+mode, sample.RunID)
			}
		}
		sample.Passed = err == nil && valid && sample.RunStatus == "succeeded" && sample.UsageComplete
		report.Samples = append(report.Samples, sample)
		if !sample.Passed {
			caseErrors = append(caseErrors, errors.Join(err, fmt.Errorf("PlanV2 real %s creation failed: configuration=%t status=%s usage_complete=%t", mode, valid, sample.RunStatus, sample.UsageComplete)))
		}
	}
	convo, err := a.p5Conversation(ctx, env, "PlanV2 real analyze")
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithCancel(context.WithoutCancel(ctx))
		defer cancel()
		failure = errors.Join(failure, a.p5DeleteWorkspace(cleanup, env, convo))
	}()
	sample, err := a.realModelTask(ctx, env, convo, "p2-analyze", planV2ModelAnalysisPrompt, nil, map[string]any{"mode": "analyze", "dashboard_ids": []string{env.State.Values["p2_files_dashboard_id"]}})
	if err != nil {
		return err
	}
	valid, err := a.verifyPlanV2ModelAnalysis(ctx, env, convo, sample.RunID)
	sample.Passed = err == nil && valid && sample.RunStatus == "succeeded" && sample.UsageComplete
	report.Samples = append(report.Samples, sample)
	if !sample.Passed {
		caseErrors = append(caseErrors, errors.Join(err, fmt.Errorf("PlanV2 real analysis failed its file/coverage oracle")))
	}
	return errors.Join(caseErrors...)
}

func planV2ParseModelGroups(value string) ([]string, error) {
	if value == "" {
		return []string{"core", "authoring", "conditions", "failures"}, nil
	}
	groups := strings.Split(value, ",")
	seen := map[string]bool{}
	for _, g := range groups {
		if !slices.Contains([]string{"core", "authoring", "conditions", "failures"}, g) || seen[g] {
			return nil, fmt.Errorf("%w: invalid or duplicate PlanV2 model group", errUsage)
		}
		seen[g] = true
	}
	return groups, nil
}

type planV2CreationExpectation struct {
	Matches  func(dashboard.Spec) bool
	Bindings []dashboard.Binding
}

func (a *App) verifyPlanV2ModelCreation(ctx context.Context, env *E2EEnvironment, run, name, mode string, expectations ...planV2CreationExpectation) (bool, error) {
	if _, err := uuid.Parse(run); err != nil {
		return false, err
	}
	client, _ := scenarioHTTP(env)
	match := func(spec dashboard.Spec) bool { return planV2ModelSpecMatches(spec, mode) }
	if len(expectations) > 0 {
		match = expectations[0].Matches
	}
	items, err := client.JSONArray(ctx, "p2-model-drafts-"+mode, "enterprise", http.MethodGet, "/dashboard-drafts", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return false, err
	}
	id := ""
	for _, item := range items {
		if item["name"] != name {
			continue
		}
		raw, _ := json.Marshal(item["spec"])
		spec, e := dashboard.DecodeSpec(raw)
		if e != nil || !match(spec) || item["dashboard_id"] != nil {
			return false, nil
		}
		if len(expectations) > 0 {
			raw, _ = json.Marshal(item["proposed_bindings"])
			var bindings []dashboard.Binding
			if json.Unmarshal(raw, &bindings) != nil || !planV2BindingsEqual(bindings, expectations[0].Bindings) {
				return false, nil
			}
		}
		id, _ = item["id"].(string)
	}
	if id == "" {
		return false, nil
	}
	checks, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+run+"' AND status='succeeded' AND tool_id='tool.invoke' AND input->>'category'='dashboard' AND input->>'name'='catalog';")
	if err != nil || strings.TrimSpace(checks) == "0" {
		return false, err
	}
	actions, err := a.postgresQuery(ctx, env, "SELECT action_ref FROM pending_actions WHERE run_id='"+run+"' AND action_type='telemetry.dashboard.publish' AND status='awaiting_confirmation';")
	if err != nil {
		return false, err
	}
	refs := strings.Fields(actions)
	if len(refs) != 1 {
		return false, nil
	}
	if _, err := a.confirmPendingAction(ctx, env, "p2-model-confirm-"+mode, refs[0]); err != nil {
		return false, err
	}
	draft, err := client.JSON(ctx, "p2-model-published-"+mode, "enterprise", http.MethodGet, "/dashboard-drafts/"+id, 200, nil, enterpriseHeaders(env, ""))
	if err != nil || draft["status"] != "published" {
		return false, err
	}
	board, err := stringField(draft, "dashboard_id")
	if err != nil {
		return false, err
	}
	published, err := client.JSON(ctx, "p2-model-revision-"+mode, "enterprise", http.MethodGet, "/dashboards/"+board, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return false, err
	}
	if nestedMap(published, "dashboard")["name"] != name {
		return false, nil
	}
	raw, _ := json.Marshal(nestedMap(published, "revision")["spec"])
	spec, err := dashboard.DecodeSpec(raw)
	if err != nil || !match(spec) {
		return false, err
	}
	if len(expectations) > 0 {
		raw, err := a.postgresQuery(ctx, env, "SELECT coalesce(json_agg(json_build_object('resource_type',resource_type,'resource_id',resource_id)),'[]') FROM dashboard_bindings WHERE dashboard_id='"+board+"';")
		if err != nil {
			return false, err
		}
		var bindings []dashboard.Binding
		if json.Unmarshal([]byte(raw), &bindings) != nil || !planV2BindingsEqual(bindings, expectations[0].Bindings) {
			return false, nil
		}
	}
	return true, nil
}

func planV2ModelSpecMatches(spec dashboard.Spec, mode string) bool {
	if !dashboard.Validate(spec).Valid || len(spec.Panels) != 1 || len(spec.Variables) != 0 || spec.DefaultTimeRange.Kind != "relative" || spec.DefaultTimeRange.Seconds != 3600 {
		return false
	}
	p := spec.Panels[0]
	if p.Signal != "metrics" || p.Type != "timeseries" || p.AuthoringMode != mode || p.SourceBinding.SourceType != "otlp" || len(p.Targets) != 1 || !slices.Contains(p.ApplicableResourceTypes, "kubernetes_cluster") || !slices.Contains(p.ApplicableResourceTypes, "host") {
		return false
	}
	t := p.Targets[0]
	if t.QueryMode != "range" {
		return false
	}
	if mode == "builder" {
		b := t.SourceDefinition.Builder
		return b != nil && b.Operation == "value" && b.Metric == "argus_m7_e2e_gauge_planv2" && len(b.Filters) == 0
	}
	if t.SourceDefinition.DSL == nil {
		return false
	}
	expr, err := parser.NewParser(parser.Options{}).ParseExpr(t.SourceDefinition.DSL.Expression)
	return err == nil && expr.String() == "argus_m7_e2e_gauge_planv2"
}

const planV2ModelAnalysisPrompt = `分析已明确选中的仪表盘。使用它保存的默认条件，检查全部适用统计图，不要修改查询或发布配置。实际取数并等待文件交付，使用离线工具读取和分析文件；在 Chat 说明检查范围、观测结果和未检查部分，不把已接收样本推算成全量。
为核对你确实读取了文件，请在 Chat 结论末尾附一个 JSON 代码块，结构为 {"files":[{"panel_id":"图 ID","sha256":"实际数据文件 SHA256","records":实际 JSON 记录数}]}，不需要另外生成或发布证明文件。每个数据文件一项，不包含 manifest。metrics 的记录数是返回序列数，logs 是日志行数，traces 是链路列表项数。必须计算实际文件字节，不能仅抄清单就声称完成分析。`

func (a *App) verifyPlanV2ModelAnalysis(ctx context.Context, env *E2EEnvironment, convo, run string) (bool, error) {
	if _, err := uuid.Parse(run); err != nil {
		return false, err
	}
	jobID, err := a.postgresQuery(ctx, env, "SELECT id::text FROM dashboard_query_jobs WHERE run_id='"+run+"' AND status='complete' ORDER BY created_at DESC LIMIT 1;")
	if err != nil || strings.TrimSpace(jobID) == "" {
		return false, err
	}
	job, err := a.waitPlanV2Files(ctx, env, "/conversations/"+convo+"/dashboard-queries/"+strings.TrimSpace(jobID))
	if err != nil {
		return false, err
	}
	if job.Manifest == nil || !job.Manifest.Complete || len(job.Manifest.Execution.Panels) != 3 {
		return false, nil
	}
	for _, p := range job.Manifest.Execution.Panels {
		if p.Status != "success" {
			return false, nil
		}
	}
	read, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+run+"' AND status='succeeded' AND tool_id IN ('read','bash');")
	if err != nil || strings.TrimSpace(read) == "0" {
		return false, err
	}
	client, _ := scenarioHTTP(env)
	reply, err := a.planV2ModelReply(ctx, env, run)
	if err != nil {
		return false, err
	}
	blocks := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```").FindAllStringSubmatch(reply, -1)
	if len(blocks) == 0 {
		return false, fmt.Errorf("Chat omitted file evidence")
	}
	content := []byte(strings.TrimSpace(blocks[len(blocks)-1][1]))
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "planv2-model-analysis-evidence.json"), content); err != nil {
		return false, err
	}
	var proof struct {
		Files []struct {
			PanelID string `json:"panel_id"`
			Hash    string `json:"sha256"`
			Records int    `json:"records"`
		} `json:"files"`
	}
	if json.Unmarshal(content, &proof) != nil || len(proof.Files) != 3 {
		return false, nil
	}
	expected := map[string]struct {
		hash    string
		records int
	}{}
	for _, f := range job.Files {
		if f.Kind != "data" {
			continue
		}
		if f.WorkspaceFileID == nil {
			return false, nil
		}
		body, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+convo+"/workspace/files/"+f.WorkspaceFileID.String()+"/content", nil, 200, "")
		if err != nil {
			return false, err
		}
		var data any
		if json.Unmarshal(body, &data) != nil {
			return false, nil
		}
		expected[f.PanelID] = struct {
			hash    string
			records int
		}{f.Hash, planV2RecordCount(data)}
	}
	for _, f := range proof.Files {
		want, ok := expected[f.PanelID]
		if !ok || f.Hash != want.hash || f.Records != want.records || f.Records <= 0 {
			return false, nil
		}
		delete(expected, f.PanelID)
	}
	return len(expected) == 0, nil
}

func planV2RecordCount(value any) int {
	switch v := value.(type) {
	case []any:
		return len(v)
	case map[string]any:
		for _, key := range []string{"rows", "traces", "result"} {
			if rows, ok := v[key].([]any); ok {
				return len(rows)
			}
		}
		n := 0
		for _, child := range v {
			n = max(n, planV2RecordCount(child))
		}
		return n
	}
	return 0
}
