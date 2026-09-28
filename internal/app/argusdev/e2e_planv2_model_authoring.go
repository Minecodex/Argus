package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
)

func planV2BindingsEqual(got, want []dashboard.Binding) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[dashboard.Binding]bool{}
	for _, b := range got {
		if seen[b] || !slices.Contains(want, b) {
			return false
		}
		seen[b] = true
	}
	return true
}

// The model discovers IDs and candidate values; the prompt supplies user intent,
// not a Draft JSON or a sequence of tool calls. Publication uses the host oracle.
func (a *App) planV2ModelAuthoring(ctx context.Context, env *E2EEnvironment) ([]p5BenchmarkSample, error) {
	_, _ = fmt.Fprintln(a.stdout, "PlanV2 real model: variables and resource suggestions")
	var names struct {
		Host    string `json:"host"`
		Cluster string `json:"cluster"`
	}
	raw, err := a.postgresQuery(ctx, env, "SELECT json_build_object('host',(SELECT name FROM hosts WHERE id='"+env.State.Values["m7_host_id"]+"'),'cluster',(SELECT name FROM kubernetes_clusters WHERE id='"+env.State.Values["m3_cluster_id"]+"'));")
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(raw), &names); err != nil || names.Host == "" || names.Cluster == "" {
		return nil, errors.Join(err, fmt.Errorf("model authoring resource fixtures missing"))
	}
	convo, err := a.p5Conversation(ctx, env, "PlanV2 variables and shortcuts")
	if err != nil {
		return nil, err
	}
	name := "PlanV2 model variables " + uuid.NewString()
	prompt := fmt.Sprintf(`创建仪表盘，名称必须是 %s，默认最近一小时。先核实真实指标 argus_planv2_selection 的目录与 pool 标签候选。仅添加一个名为 pool 的单选查询变量，允许全部，默认 blue。添加两张 OTLP 指标趋势图，均用构建器读取这个指标的原值：一张通过 pool 标签引用这个变量，另一张不引用变量，保持全部 pool，均不聚合。不添加其他变量或图。建议把它同时关联到主机“%s”和集群“%s”的快捷入口，请从授权资源目录确认 ID；关联只是导航，不授予数据权限，也不能把已安装来源说成已有样本。两张图均适用于主机与集群。将这两项可选关联放入本次草稿预览，解释其作用及样本限制，等待我确认，不要直接发布。`, name, names.Host, names.Cluster)
	sample, err := a.realModelTask(ctx, env, convo, "p2-create-variables-bindings", prompt, nil, map[string]any{"mode": "create", "dashboard_ids": []string{}})
	if err != nil {
		return []p5BenchmarkSample{sample}, err
	}
	want := []dashboard.Binding{{ResourceType: "host", ResourceID: uuid.MustParse(env.State.Values["m7_host_id"])}, {ResourceType: "kubernetes_cluster", ResourceID: uuid.MustParse(env.State.Values["m3_cluster_id"])}}
	valid, err := a.verifyPlanV2ModelCreation(ctx, env, sample.RunID, name, "variables", planV2CreationExpectation{Matches: planV2VariableSpecMatches, Bindings: want})
	if err == nil && valid {
		err = a.waitRunTerminal(ctx, env, sample.RunID)
		if err == nil {
			sample, err = a.p5BenchmarkSample(ctx, env, sample.ID, sample.RunID)
		}
	}
	if err == nil && valid {
		calls, e := a.postgresQuery(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+sample.RunID+"' AND tool_id='tool.invoke' AND input->>'category'='dashboard' AND input->>'name'='catalog.resources' AND status='succeeded';")
		err, valid = e, strings.TrimSpace(calls) != "0" && strings.TrimSpace(calls) != ""
	}
	sample.Passed = err == nil && valid && sample.RunStatus == "succeeded" && sample.UsageComplete
	if !sample.Passed {
		err = errors.Join(err, fmt.Errorf("model variable generation / host-cluster suggestions failed"))
	}
	return []p5BenchmarkSample{sample}, err
}

func planV2VariableSpecMatches(spec dashboard.Spec) bool {
	if !dashboard.Validate(spec).Valid || len(spec.Variables) != 1 || len(spec.Panels) != 2 || spec.DefaultTimeRange.Kind != "relative" || spec.DefaultTimeRange.Seconds != 3600 {
		return false
	}
	v := spec.Variables[0]
	if v.Name != "pool" || v.Multiple || !v.IncludeAll || v.Default.All || !slices.Equal(v.Default.Values, []string{"blue"}) || v.Query.Signal != "metrics" || v.Query.Metric != "argus_planv2_selection" || v.Query.Field != "pool" || v.Query.SourceBinding.SourceType != "otlp" || len(v.Query.Filters) != 0 {
		return false
	}
	dependent, independent := 0, 0
	for _, p := range spec.Panels {
		if p.Type != "timeseries" || p.Signal != "metrics" || p.AuthoringMode != "builder" || p.SourceBinding.SourceType != "otlp" || len(p.Targets) != 1 || !slices.Contains(p.ApplicableResourceTypes, "host") || !slices.Contains(p.ApplicableResourceTypes, "kubernetes_cluster") {
			return false
		}
		t := p.Targets[0]
		b := t.SourceDefinition.Builder
		// GroupBy is inert for the value operation in the shared compiler.
		// Reject actual aggregations, not semantically irrelevant editor fields.
		if t.QueryMode != "range" || b == nil || b.Operation != "value" || b.Metric != "argus_planv2_selection" {
			return false
		}
		if len(b.Filters) == 0 && len(t.ParameterBindings) == 0 {
			independent++
			continue
		}
		if len(b.Filters) != 1 || b.Filters[0].Field != "pool" || b.Filters[0].Operator != "=" || b.Filters[0].Variable != "pool" || b.Filters[0].Value != "" || len(b.Filters[0].Values) != 0 {
			return false
		}
		dependent++
	}
	return dependent == 1 && independent == 1
}
