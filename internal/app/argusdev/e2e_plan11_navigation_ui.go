package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/dashboardcontext"
)

// Actual scope-only messages test persisted UI without repeating the separately
// recorded full dashboard/network model workflows.
func (a *App) plan11DeployedNavigationUI(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (failure error) {
	if !plan11NavigationPhase(env.Options.Plan8Phase) || host == nil || env.Options.RealModel == nil {
		return errors.New("navigation UI requires its explicit owned real-model phase")
	}
	failure = a.plan11DashboardFiltersAndScope(ctx, env, host, false)
	if errors.Is(failure, errPlan11NavigationBoundary) || ctx.Err() != nil {
		return failure
	}
	model, err := a.plan7Model(ctx, env)
	if err != nil {
		return errors.Join(failure, err)
	}
	conversation, err := a.plan7ModelConversation(ctx, env, model, "plan11-scope-visibility")
	if err != nil {
		return errors.Join(failure, err)
	}
	rows := []map[string]any{}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()
		failure = errors.Join(failure, a.p5DeleteWorkspace(cleanup, env, conversation))
		raw, e := json.MarshalIndent(map[string]any{"passed": failure == nil, "scope": "actual_scope_admission_and_persisted_UI_not_repeated_data_analysis", "cases": rows, "whole_plan_complete": false}, "", "  ")
		failure = errors.Join(failure, e, writePrivate(filepath.Join(env.Options.Artifacts, "plan11-scope-visibility.json"), raw))
	}()
	for _, step := range []struct{ name, scope, prompt string }{
		{"plan11-scope-visibility-restricted", dashboardcontext.DashboardAnalysis, fmt.Sprintf("/分析仪表盘 将本轮限制为仅分析已发布仪表盘%s。现在不要取数、读取文件、查询目录、执行远程操作或变更；只简短确认范围，等待我后续问题。", host.DashboardID)},
		{"plan11-scope-visibility-reset", dashboardcontext.GeneralDiagnosis, "/综合诊断 退出仅仪表盘分析限制。现在不要取数、读取文件、查询目录、执行远程操作或变更；只简短确认恢复综合诊断，等待我后续问题。"},
	} {
		row, e := a.plan11ScopeVisibilityTask(ctx, env, conversation, step.name, step.scope, step.prompt)
		rows = append(rows, row)
		failure = errors.Join(failure, e)
		if errors.Is(e, errPlan11NavigationBoundary) || ctx.Err() != nil {
			return failure
		}
		if row["scope_matches"] == true {
			failure = errors.Join(failure, a.plan11ScopeBrowser(ctx, env, conversation, step.scope))
		}
	}
	return failure
}

func (a *App) plan11ScopeVisibilityTask(ctx context.Context, env *E2EEnvironment, conversation, name, scope, prompt string) (proof map[string]any, failure error) {
	proof = map[string]any{"name": name, "conversation_id": conversation, "expected_scope": scope, "passed": false}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	sample, err := a.realModelTask(bounded, env, conversation, name, prompt, nil, nil)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		if sample.RunID != "" {
			if failure != nil {
				failure = errors.Join(failure, a.plan8StopFailedModelTask(cleanup, env, sample.RunID))
			}
			trace, e := a.postgresQuery(cleanup, env, plan11InteractionTraceSQL(sample.RunID))
			if e == nil {
				e = writePlan11ToolEvidence(filepath.Join(env.Options.Artifacts, name+"-tools.json"), trace)
			}
			failure = errors.Join(failure, e, a.plan11MetadataModelLedger(cleanup, env, name, sample.RunID, sample.ModelCalls), a.planV2ModelEvidence(cleanup, env, conversation, sample, failure))
		}
		proof["sample"], proof["passed"] = sample, failure == nil
	}()
	if err != nil {
		return proof, err
	}
	if sample.RunStatus != "succeeded" || sample.ModelCalls < 1 || sample.ModelCalls > 4 || !sample.UsageComplete || !sample.CachedUsageComplete {
		return proof, errors.New("scope-only acknowledgment did not finish within its four-call gate")
	}
	actual, err := a.postgresQuery(bounded, env, "SELECT selection->>'task_scope' FROM dashboard_run_contexts WHERE run_id='"+sample.RunID+"'")
	if err != nil || strings.TrimSpace(actual) != scope {
		return proof, errors.Join(err, errors.New("scope-only admission did not persist the requested scope"))
	}
	proof["scope_matches"] = true
	trace, err := a.postgresQuery(bounded, env, plan11ModelToolTraceSQL(sample.RunID))
	if err != nil {
		return proof, err
	}
	var calls []plan8ModelToolFact
	if json.Unmarshal([]byte(trace), &calls) != nil {
		return proof, errors.New("invalid scope-only tool trace")
	}
	for _, call := range calls {
		if call.Tool != "tool.search" && call.Tool != "tool.describe" {
			return proof, errPlan11NavigationBoundary
		}
	}
	mutations, err := a.postgresQuery(bounded, env, "SELECT count(*) FROM pending_actions WHERE run_id='"+sample.RunID+"'")
	if err != nil || strings.TrimSpace(mutations) != "0" {
		return proof, errors.Join(err, errPlan11NavigationBoundary)
	}
	sample.Passed = true
	return proof, nil
}

func plan11NavigationCallBound(phase string) int {
	if phase == "plan11-navigation-publish" {
		return 136
	}
	return 72
}
