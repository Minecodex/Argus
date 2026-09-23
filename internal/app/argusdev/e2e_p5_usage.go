package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
)

func (a *App) verifyP5MissingUsage(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	convo, err := a.p5Conversation(ctx, env, "missing provider usage")
	if err != nil {
		return err
	}
	run := ""
	for i := 0; i < 2; i++ {
		key := fmt.Sprintf("p5-missing-usage-%d", i)
		value, err := client.JSON(ctx, key, "enterprise", http.MethodPost, "/conversations/"+convo+"/messages", 202, map[string]any{"content": "argus_e2e_usage_missing: retain these facts for the next turn", "file_ids": []string{}}, enterpriseHeaders(env, key))
		if err != nil {
			return err
		}
		run, err = stringField(value, "run", "run_id")
		if err != nil {
			return err
		}
		if err = a.waitRunTerminal(ctx, env, run); err != nil {
			return err
		}
	}
	if _, err = client.JSON(ctx, "p5-missing-usage-compact", "enterprise", http.MethodPost, "/runs/"+run+"/compact", 202, nil, enterpriseHeaders(env, "p5-missing-usage-compact")); err != nil {
		return err
	}
	if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM model_calls WHERE run_id='"+run+"' AND call_kind='compaction' AND status='succeeded';", "1", 2*time.Minute); err != nil {
		return err
	}
	if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM model_quota_reservations q JOIN model_calls c ON c.id=q.model_call_id WHERE c.run_id='"+run+"' AND c.call_kind='compaction' AND q.status='settled' AND q.usage_source='estimated';", "1", time.Minute); err != nil {
		return err
	}
	encoded, err := a.postgresQuery(ctx, env, p5BenchmarkSampleSQL(run))
	if err != nil {
		return err
	}
	var sample p5BenchmarkSample
	if err = json.Unmarshal([]byte(encoded), &sample); err != nil {
		return err
	}
	if sample.UsageComplete || sample.InputTokens != 0 || sample.OutputTokens != 0 || sample.ModelCalls < 2 {
		return fmt.Errorf("P5 missing usage passed the real-model completeness gate")
	}
	data, _ := json.Marshal(map[string]any{"fixture": "deterministic missing usage", "sample": sample, "compaction_settlement_source": "estimated", "expected_gate_rejected": true})
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-usage-provenance.json"), append(data, '\n'))
}
