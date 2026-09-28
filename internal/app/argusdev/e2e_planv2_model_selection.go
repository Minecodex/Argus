package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

func (a *App) verifyPlanV2ModelSelection(ctx context.Context, env *E2EEnvironment) (p5BenchmarkSample, error) {
	convo, err := a.p5Conversation(ctx, env, "PlanV2 real selection")
	if err != nil {
		return p5BenchmarkSample{}, err
	}
	sample, err := a.realModelTask(ctx, env, convo, "p2-select-dashboard", "帮我看看仪表盘有没有问题。", nil, map[string]any{"mode": "analyze", "dashboard_ids": []string{}})
	if err != nil {
		return sample, err
	}
	if uuid.Validate(sample.RunID) != nil {
		return sample, fmt.Errorf("invalid model selection Run")
	}
	var facts struct {
		Lists  int    `json:"list_calls"`
		Scoped int    `json:"scoped_calls"`
		Reply  string `json:"reply"`
	}
	value, err := a.postgresQuery(ctx, env, `SELECT json_build_object(
 'list_calls',(SELECT count(*) FROM tool_calls WHERE run_id='`+sample.RunID+`' AND tool_id='tool.invoke' AND input->>'category'='dashboard' AND input->>'name'='list' AND status='succeeded'),
 'scoped_calls',(SELECT count(*) FROM tool_calls WHERE run_id='`+sample.RunID+`' AND tool_id='tool.invoke' AND input->>'category'='dashboard' AND (input->>'name' IN ('get','context.resolve') OR input->>'name' LIKE 'query%')),
 'reply',(SELECT payload->>'content' FROM conversation_events WHERE run_id='`+sample.RunID+`' AND event_type='assistant_message' AND coalesce(payload->>'content','')<>'' ORDER BY sequence DESC LIMIT 1));`)
	if err != nil {
		return sample, err
	}
	if err = json.Unmarshal([]byte(value), &facts); err != nil {
		return sample, err
	}
	asked := false
	for _, word := range []string{"选择", "选中", "哪", "指定", "告诉", "select", "choose", "which"} {
		asked = asked || strings.Contains(strings.ToLower(facts.Reply), word)
	}
	structuredHint := strings.Contains(facts.Reply, "@") || strings.Contains(facts.Reply, "选择器") || strings.Contains(facts.Reply, "下拉")
	sample.Passed = facts.Lists > 0 && facts.Scoped == 0 && asked && structuredHint && sample.UsageComplete && (sample.RunStatus == "succeeded" || sample.RunStatus == "waiting_input")
	body, _ := json.MarshalIndent(map[string]any{"run_id": sample.RunID, "facts": facts, "asked_user": asked, "structured_selection_hint": structuredHint, "passed": sample.Passed, "scope": "Chinese clarification smoke case; not a general semantic quality score"}, "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "planv2-model-selection.json"), body); err != nil {
		return sample, err
	}
	if !sample.Passed {
		return sample, fmt.Errorf("PlanV2 model did not list dashboards and wait for explicit selection")
	}
	return sample, nil
}
