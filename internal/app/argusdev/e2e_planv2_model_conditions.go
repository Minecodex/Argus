package argusdev

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
)

func planV2ModelConditionSpec(pool string, seconds int) (dashboard.Spec, error) {
	spec, err := planV2ThreeSignals()
	if err != nil {
		return spec, err
	}
	spec.DefaultTimeRange = dashboard.TimeRange{Kind: "relative", Seconds: seconds}
	spec.Panels = spec.Panels[:2]
	spec.Variables = []dashboard.Variable{{ID: "pool", Name: "pool", Label: "Pool", IncludeAll: true, Default: dashboard.Selection{Values: []string{pool}}, Query: dashboard.CandidateQuery{Signal: "metrics", SourceBinding: dashboard.SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, Metric: "argus_planv2_selection", Field: "pool", Filters: []dashboard.Filter{}}}}
	p := &spec.Panels[0]
	p.Title = "指标总量"
	p.Type = "stat"
	p.Targets[0].QueryMode = "instant"
	p.Targets[0].SourceDefinition.DSL.Expression = `sum(last_over_time(argus_planv2_selection{pool="$pool"}[1h]))`
	p.Targets[0].ParameterBindings = []dashboard.ParameterBinding{{Parameter: "pool", Variable: "pool"}}
	p = &spec.Panels[1]
	p.Title = "业务日志"
	p.Targets[0].SourceDefinition.DSL.Expression = `body : "planv2 selection" AND severity_text = "$pool"`
	p.Targets[0].SourceDefinition.DSL.Pipeline = "limit 100"
	p.Targets[0].ParameterBindings = []dashboard.ParameterBinding{{Parameter: "pool", Variable: "pool", ValueMap: map[string]string{"blue": "INFO", "green": "ERROR"}}}
	if report := dashboard.Validate(spec); !report.Valid {
		return spec, fmt.Errorf("model condition fixture invalid: %+v", report.Issues)
	}
	return spec, nil
}

func (a *App) planV2LatestRevision(ctx context.Context, env *E2EEnvironment, board string) (string, error) {
	client, _ := scenarioHTTP(env)
	value, err := client.JSON(ctx, "p2-model-latest", "enterprise", http.MethodGet, "/dashboards/"+board, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return "", err
	}
	return stringField(value, "revision", "id")
}

func (a *App) planV2RepublishModelFixture(ctx context.Context, env *E2EEnvironment, board, name string, spec dashboard.Spec) error {
	client, _ := scenarioHTTP(env)
	input := planV2DraftRequest(spec)
	input["name"] = name
	input["dashboard_id"] = board
	draft, err := client.JSON(ctx, "p2-model-edit", "enterprise", http.MethodPost, planV2DraftPath, 201, input, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	id, err := stringField(draft, "id")
	if err != nil {
		return err
	}
	input["expected_version"] = draft["draft_version"]
	draft, err = client.JSON(ctx, "p2-model-save", "enterprise", http.MethodPatch, planV2DraftPath+"/"+id, 200, input, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	preview, err := client.JSON(ctx, "p2-model-republish-preview", "enterprise", http.MethodPost, planV2DraftPath+"/"+id+"/preview", 201, map[string]any{"expected_version": draft["draft_version"]}, enterpriseHeaders(env, "p2-model-republish-"+uuid.NewString()))
	if err != nil {
		return err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	_, err = a.confirmPendingAction(ctx, env, p5RequestKey("p2-model-republish-confirm", board), ref)
	return err
}

func (a *App) planV2ModelConditions(ctx context.Context, env *E2EEnvironment) (samples []p5BenchmarkSample, failure error) {
	_, _ = fmt.Fprintln(a.stdout, "PlanV2 real model: independent defaults, follow-up, latest revision and panel strategy")
	// Refresh known samples through the real receiver; a slow previous model
	// authoring case must not make this independent fixture expire.
	if _, err := env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-server", "argus-server", "/usr/local/bin/argus-telemetry-e2e", "--endpoint=127.0.0.1:4317", "--marker=planv2", "--resource-id="+env.State.Values["m3_cluster_id"]); err != nil {
		return nil, err
	}
	if err := a.waitPlanV2QueryReady(ctx, env); err != nil {
		return nil, err
	}
	aSpec, err := planV2ModelConditionSpec("blue", 3600)
	if err != nil {
		return nil, err
	}
	bSpec, err := planV2ModelConditionSpec("green", 7200)
	if err != nil {
		return nil, err
	}
	aID, err := a.publishPlanV2Fixture(ctx, env, "模型条件 A", aSpec)
	if err != nil {
		return nil, err
	}
	bID, err := a.publishPlanV2Fixture(ctx, env, "模型条件 B", bSpec)
	if err != nil {
		return nil, err
	}
	aRev, err := a.planV2LatestRevision(ctx, env, aID)
	if err != nil {
		return nil, err
	}
	bRev, err := a.planV2LatestRevision(ctx, env, bID)
	if err != nil {
		return nil, err
	}
	convo, err := a.p5Conversation(ctx, env, "PlanV2 condition inheritance")
	if err != nil {
		return nil, err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()
		failure = errors.Join(failure, a.p5DeleteWorkspace(c, env, convo))
	}()
	aWant := planV2ExpectedQuery{Board: aID, Revision: aRev, Seconds: 3600, Pool: "blue", Panels: []string{"metrics", "logs"}}
	bWant := planV2ExpectedQuery{Board: bID, Revision: bRev, Seconds: 7200, Pool: "green", Panels: []string{"metrics", "logs"}}
	runCase := func(id, prompt string, selection map[string]any, want []planV2ExpectedQuery) error {
		sample, err := a.realModelTask(ctx, env, convo, id, prompt+"\n"+planV2ConclusionPrompt, nil, selection)
		if err == nil {
			err = a.planV2VerifyModelQueries(ctx, env, convo, sample.RunID, want)
		}
		sample.Passed = err == nil && sample.RunStatus == "succeeded" && sample.UsageComplete
		if !sample.Passed {
			err = errors.Join(err, fmt.Errorf("%s did not satisfy model/usage/semantic oracle", id))
		}
		if sample.RunID != "" {
			err = errors.Join(err, a.planV2ModelEvidence(ctx, env, convo, sample, err))
		}
		samples = append(samples, sample)
		return err
	}
	if err = runCase("p2-multi-defaults", "检查所选两张仪表盘各自默认条件下的全部统计图，比较指标总量、日志记录数与 ERROR 日志数量，判断收到的样本是否存在问题。", map[string]any{"mode": "analyze", "dashboard_ids": []string{aID, bID}}, []planV2ExpectedQuery{aWant, bWant}); err != nil {
		return samples, err
	}
	clusterName, err := a.postgresQuery(ctx, env, "SELECT name FROM kubernetes_clusters WHERE id='"+env.State.Values["m3_cluster_id"]+"';")
	if err != nil {
		return samples, err
	}
	aWant.Seconds = 7200
	aWant.Pool = "green"
	aWant.Resources = []uuid.UUID{uuid.MustParse(env.State.Values["m3_cluster_id"])}
	aWant.Panels = []string{"metrics"}
	aWant.Explicit = true
	if err = runCase("p2-specific-conditions", fmt.Sprintf("这轮仅查看“模型条件 A”的指标总量，最近2小时，pool=green，资源仅限集群“%s”。暂时不查日志，也不查 B。", clusterName), nil, []planV2ExpectedQuery{aWant}); err != nil {
		return samples, err
	}
	aWant.Seconds = 10800
	if err = runCase("p2-followup-inheritance", "继续，仅把时间改为最近3小时，其他条件不变，仍只看 A 的指标总量。", nil, []planV2ExpectedQuery{aWant}); err != nil {
		return samples, err
	}
	// Change defaults and titles, preserving the semantic variable contract.
	// A's explicit conditions must survive; B must use its new own defaults.
	aSpec.DefaultTimeRange.Seconds = 14400
	aSpec.Panels[0].Title = "指标总量 R2"
	bSpec.DefaultTimeRange.Seconds = 18000
	bSpec.Variables[0].Default = dashboard.Selection{Values: []string{"blue"}}
	if err = a.planV2RepublishModelFixture(ctx, env, aID, "模型条件 A", aSpec); err != nil {
		return samples, err
	}
	if err = a.planV2RepublishModelFixture(ctx, env, bID, "模型条件 B", bSpec); err != nil {
		return samples, err
	}
	aWant.Revision, err = a.planV2LatestRevision(ctx, env, aID)
	if err != nil {
		return samples, err
	}
	bWant.Revision, err = a.planV2LatestRevision(ctx, env, bID)
	if err != nil {
		return samples, err
	}
	if aWant.Revision == aRev || bWant.Revision == bRev {
		return samples, fmt.Errorf("revision fixture was not republished")
	}
	aWant.Panels = []string{"metrics", "logs"}
	bWant.Pool = "blue"
	bWant.Seconds = 18000
	err = runCase("p2-latest-revision", "两张仪表盘刚有新发布版本。重新取数检查所选两张仪表盘全部适用统计图，保留我明确指定过的条件；没有明确指定过的条件使用各自当前发布的默认值。", nil, []planV2ExpectedQuery{aWant, bWant})
	return samples, err
}
