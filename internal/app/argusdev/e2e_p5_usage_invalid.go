package argusdev

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) verifyP5InvalidUsage(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	convo, err := a.p5Conversation(ctx, env, "invalid inference usage")
	if err != nil {
		return err
	}
	steps, _ := json.Marshal([]p5ToolStep{{"tool.search", map[string]any{"category": "host", "query": "list"}}})
	post := func(convo, key, content string) (string, error) {
		value, err := client.JSON(ctx, key, "enterprise", http.MethodPost, "/conversations/"+convo+"/messages", 202, map[string]any{"content": content, "file_ids": []string{}}, enterpriseHeaders(env, key))
		if err != nil {
			return "", err
		}
		return stringField(value, "run", "run_id")
	}
	run, err := post(convo, "p5-invalid-usage", "argus_e2e_usage_inconsistent argus_e2e_plan_b64:"+base64.RawURLEncoding.EncodeToString(steps))
	if err != nil {
		return err
	}
	if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM runs WHERE id='"+run+"' AND status='failed' AND error_code='MODEL_USAGE_INVALID';", "1", time.Minute); err != nil {
		return err
	}
	if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM runtime_tasks WHERE run_id='"+run+"' AND queue='agent' AND status='succeeded';", "1", time.Minute); err != nil {
		return err
	}
	compConvo, err := a.p5Conversation(ctx, env, "invalid compaction usage")
	if err != nil {
		return err
	}
	compRun := ""
	for i := 0; i < 2; i++ {
		compRun, err = post(compConvo, fmt.Sprintf("p5-invalid-compaction-%d", i), "argus_e2e_compaction_usage_inconsistent: retain these facts for the next turn")
		if err != nil {
			return err
		}
		if err = a.waitRunTerminal(ctx, env, compRun); err != nil {
			return err
		}
	}
	if _, err = client.JSON(ctx, "p5-invalid-compact", "enterprise", http.MethodPost, "/runs/"+compRun+"/compact", 202, nil, enterpriseHeaders(env, "p5-invalid-compact")); err != nil {
		return err
	}
	if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM runtime_tasks WHERE run_id='"+compRun+"' AND queue='compaction' AND status='failed' AND last_error_code='MODEL_USAGE_INVALID' AND attempt=1;", "1", time.Minute); err != nil {
		return err
	}
	encoded, err := a.postgresQuery(ctx, env, `SELECT json_build_object(
 'invalid_calls',(SELECT count(*) FROM model_calls c JOIN model_quota_reservations q ON q.model_call_id=c.id JOIN run_steps s ON s.id=c.step_id WHERE c.run_id IN ('`+run+`','`+compRun+`') AND c.status='failed' AND c.error_code='MODEL_USAGE_INVALID' AND c.input_tokens=50 AND c.output_tokens=8 AND c.cached_input_tokens=80 AND c.input_usage_source='invalid' AND c.output_usage_source='invalid' AND c.cached_input_usage_source='invalid' AND q.status='settled' AND q.usage_source='invalid' AND q.settled_amount=q.reserved_amount AND c.amount=q.reserved_amount AND s.status='failed'),
 'inference_calls',(SELECT count(*) FROM model_calls WHERE run_id='`+run+`'),
 'tools_executed',(SELECT count(*) FROM tool_calls WHERE run_id='`+run+`'),
 'invalid_snapshots',(SELECT count(*) FROM context_snapshots WHERE conversation_id='`+compConvo+`'),
 'sponsor_preserved',(SELECT count(*) FROM runs WHERE id='`+compRun+`' AND status='succeeded'));
`)
	if err != nil {
		return err
	}
	var result map[string]any
	if err = json.Unmarshal([]byte(encoded), &result); err != nil {
		return err
	}
	for key, want := range map[string]float64{"invalid_calls": 2, "inference_calls": 1, "tools_executed": 0, "invalid_snapshots": 0, "sponsor_preserved": 1} {
		if result[key] != want {
			return fmt.Errorf("P5 invalid usage %s=%v, want %v", key, result[key], want)
		}
	}
	sample, err := a.p5BenchmarkSample(ctx, env, "invalid usage", run)
	if err != nil {
		return err
	}
	if sample.UsageComplete || sample.CachedUsageComplete || sample.TotalTokens != 0 || sample.CachedInputTokens != 0 {
		return fmt.Errorf("P5 invalid usage reached reported totals")
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-usage-consistency.json"), append([]byte(strings.TrimSpace(encoded)), '\n'))
}
