package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
)

// Export usage-only records per Run, without the global diagnostic limit.
// This fixed projection includes no prompts, headers, credentials or tool data.
func (a *App) plan11MetadataModelLedger(ctx context.Context, env *E2EEnvironment, name, run string, expected int) error {
	if uuid.Validate(run) != nil {
		return errors.New("model ledger requires an exact Run identity")
	}
	raw, err := a.postgresQuery(ctx, env, plan11MetadataModelLedgerSQL(run))
	if err != nil {
		return err
	}
	var calls []struct {
		ID  string `json:"id"`
		Run string `json:"run_id"`
	}
	if json.Unmarshal([]byte(raw), &calls) != nil {
		return errors.New("per-Run model ledger is invalid JSON")
	}
	if len(calls) != expected {
		return errors.New("per-Run model ledger does not match its actual sample call count")
	}
	seen := map[string]bool{}
	for _, call := range calls {
		if uuid.Validate(call.ID) != nil || seen[call.ID] || call.Run != run {
			return errors.New("model ledger contains a missing, duplicate or foreign call identity")
		}
		seen[call.ID] = true
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, name+"-model-calls.json"), []byte(raw))
}

func plan11MetadataModelLedgerSQL(run string) string {
	return fmt.Sprintf(`SELECT coalesce(json_agg(t),'[]') FROM (SELECT m.id,m.run_id,m.step_id,m.call_kind,m.status,m.error_code,m.stop_reason,m.input_tokens,m.output_tokens,m.input_usage_source,m.output_usage_source,m.cached_input_tokens,m.cached_input_usage_source,m.latency_ms,m.created_at,m.completed_at,m.dispatched_at,m.context_from_sequence,m.context_through_sequence,s.source_through_sequence AS compacted_through,encode(m.projection_hash,'hex') AS provider_request_hash,m.inline_query_receipts FROM model_calls m LEFT JOIN context_snapshots s ON s.id=m.context_snapshot_id AND s.enterprise_id=m.enterprise_id WHERE m.run_id='%s' ORDER BY m.created_at,m.id) t`, run)
}
