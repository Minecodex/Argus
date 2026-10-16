package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Two ordinary Chat messages exercise automatic discovery and a correction.
// Neither supplies dashboard_context or a scripted model tool plan. The oracle
// verifies new sealed queries, file analysis and presentation provenance.
func (a *App) planV2ModelAutomatic(ctx context.Context, env *E2EEnvironment) (samples []p5BenchmarkSample, failure error) {
	return a.planV2AutomaticTasks(ctx, env, true)
}

func (a *App) planV2AutomaticTasks(ctx context.Context, env *E2EEnvironment, correction bool) (samples []p5BenchmarkSample, failure error) {
	convo, err := a.p5Conversation(ctx, env, "PlanV2 automatic analysis and correction")
	if err != nil {
		return nil, err
	}
	defer func() {
		failure = errors.Join(failure, a.p5DeleteWorkspace(context.WithoutCancel(ctx), env, convo))
	}()
	board := env.State.Values["p2_files_dashboard_id"]
	if uuid.Validate(board) != nil {
		return nil, fmt.Errorf("missing published automatic-analysis fixture")
	}
	revision, err := a.postgresQuery(ctx, env, "SELECT active_revision_id::text FROM dashboards WHERE id='"+board+"';")
	if err != nil {
		return nil, err
	}
	revision = strings.TrimSpace(revision)
	if uuid.Validate(revision) != nil {
		return nil, fmt.Errorf("invalid published fixture revision")
	}
	for _, task := range []struct {
		name, prompt string
		seconds      int
	}{
		{"p2-auto-discovery", "帮我分析 PlanV2 three signal files 最近半小时的指标、日志和调用链，检查全部适用统计图并给出结论。", 1800},
		{"p2-auto-correction", "刚才的时间范围不对，改成最近一小时，重新取数并分析同一仪表盘的全部统计图。", 3600},
	} {
		sample, e := a.realModelTask(ctx, env, convo, task.name, task.prompt+planV2ConclusionPrompt, nil, nil)
		if e == nil {
			e = a.planV2VerifyModelQueries(ctx, env, convo, sample.RunID, []planV2ExpectedQuery{{Board: board, Revision: revision, Seconds: task.seconds, Panels: []string{"metrics", "logs", "traces"}, Explicit: true}})
		}
		if e == nil {
			var observed struct {
				Presentations int `json:"presentations"`
				Matching      int `json:"matching"`
				HumanIDs      int `json:"human_ids"`
			}
			value, checkErr := a.postgresQuery(ctx, env, `SELECT json_build_object(
 'presentations',(SELECT count(*) FROM tool_presentations p JOIN tool_calls t ON t.id=p.tool_call_id WHERE t.run_id='`+sample.RunID+`' AND p.tool_version='argus.dashboard.query_result/v1'),
 'matching',(SELECT count(*) FROM tool_presentations p JOIN tool_calls t ON t.id=p.tool_call_id JOIN dashboard_query_jobs j ON j.id=(p.detail_data->>'job_id')::uuid WHERE t.run_id='`+sample.RunID+`' AND p.tool_version='argus.dashboard.query_result/v1' AND p.detail_data->>'from'=j.manifest->'execution'->>'from' AND p.detail_data->>'to'=j.manifest->'execution'->>'to'),
 'human_ids',(SELECT jsonb_array_length(selection->'dashboard_ids') FROM dashboard_run_contexts WHERE run_id='`+sample.RunID+`'));`)
			if checkErr != nil {
				e = checkErr
			} else if json.Unmarshal([]byte(value), &observed) != nil || observed.Presentations < 1 || observed.Matching < 1 || observed.HumanIDs != 0 {
				e = fmt.Errorf("automatic analysis presentation did not match the immutable query: %+v", observed)
			}
			body, _ := json.MarshalIndent(map[string]any{"run_id": sample.RunID, "seconds": task.seconds, "observed": observed, "passed": e == nil}, "", "  ")
			e = errors.Join(e, writePrivate(filepath.Join(env.Options.Artifacts, task.name+".json"), body))
		}
		sample.Passed = e == nil && sample.RunStatus == "succeeded" && sample.UsageComplete
		samples = append(samples, sample)
		if !sample.Passed {
			return samples, errors.Join(e, fmt.Errorf("automatic analysis %s failed: status=%s usage_complete=%t", task.name, sample.RunStatus, sample.UsageComplete))
		}
		if !correction {
			break
		}
	}
	return samples, nil
}
