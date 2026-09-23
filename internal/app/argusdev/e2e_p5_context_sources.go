package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) verifyP5ContextSources(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	convo, err := a.p5Conversation(ctx, env, "model context provenance")
	if err != nil {
		return err
	}
	runs := []string{}
	for i := 0; i < 4; i++ {
		run, err := a.p5Run(ctx, env, convo, fmt.Sprintf("snapshot-source-%d", i), nil, []string{})
		if err != nil {
			return err
		}
		runs = append(runs, run)
		if i == 1 || i == 2 {
			key := fmt.Sprintf("p5-source-compact-%d", i)
			if _, err = client.JSON(ctx, key, "enterprise", http.MethodPost, "/runs/"+run+"/compact", 202, nil, enterpriseHeaders(env, key)); err != nil {
				return err
			}
			if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM context_snapshots WHERE conversation_id='"+convo+"';", fmt.Sprint(i), 2*time.Minute); err != nil {
				return err
			}
		}
	}
	query := `SELECT json_build_object(
 'snapshot_count',(SELECT count(*) FROM context_snapshots WHERE conversation_id='` + convo + `'),
 'inference_uses_first',(SELECT count(*) FROM model_calls m JOIN context_snapshots s ON s.id=m.context_snapshot_id AND s.snapshot_hash=m.context_snapshot_hash WHERE m.run_id='` + runs[2] + `' AND m.call_kind='inference' AND s.revision=1),
 'compaction_uses_first',(SELECT count(*) FROM model_calls m JOIN context_snapshots s ON s.id=m.context_snapshot_id AND s.snapshot_hash=m.context_snapshot_hash WHERE m.run_id='` + runs[2] + `' AND m.call_kind='compaction' AND s.revision=1),
 'next_run_uses_second',(SELECT count(*) FROM model_calls m JOIN context_snapshots s ON s.id=m.context_snapshot_id AND s.snapshot_hash=m.context_snapshot_hash WHERE m.run_id='` + runs[3] + `' AND m.call_kind='inference' AND s.revision=2),
 'cache_receipts',(SELECT count(*) FROM model_calls WHERE run_id IN ('` + strings.Join(runs, "','") + `') AND cached_input_tokens=16 AND input_tokens=32 AND cached_input_usage_source='provider'));
`
	encoded, err := a.postgresQuery(ctx, env, query)
	if err != nil {
		return err
	}
	var result map[string]any
	if err = json.Unmarshal([]byte(encoded), &result); err != nil {
		return err
	}
	for key, want := range map[string]float64{"snapshot_count": 2, "inference_uses_first": 1, "compaction_uses_first": 1, "next_run_uses_second": 1, "cache_receipts": 6} {
		if result[key] != want {
			return fmt.Errorf("P5 context provenance %s=%v, want %v", key, result[key], want)
		}
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-context-provenance.json"), append([]byte(strings.TrimSpace(encoded)), '\n'))
}
