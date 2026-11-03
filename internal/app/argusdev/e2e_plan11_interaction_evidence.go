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

func (a *App) plan11InteractionModelEvidence(ctx context.Context, env *E2EEnvironment, model string, proof map[string]any) error {
	if uuid.Validate(model) != nil {
		return errors.New("interaction accounting has no scoped model identity")
	}
	raw, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT coalesce(json_agg(json_build_object('id',r.id,'conversation',r.conversation_id,'status',r.status,'stop_reason',r.stop_reason) ORDER BY r.created_at),'[]') FROM runs r WHERE r.model_id='%s' AND r.enterprise_id='%s'`, model, env.State.Values["enterprise_id"]))
	if err != nil {
		return err
	}
	var runs []struct {
		ID           string `json:"id"`
		Conversation string `json:"conversation"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &runs) != nil {
		return errors.New("invalid interaction run accounting response")
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan11-interaction-runs.json"), []byte(raw)); err != nil {
		return err
	}
	var failures []error
	for index, run := range runs {
		if uuid.Validate(run.ID) != nil || uuid.Validate(run.Conversation) != nil {
			return errors.New("interaction run evidence has an invalid scoped identity")
		}
		name := fmt.Sprintf("plan11-interaction-run-%02d", index+1)
		sample, sampleErr := a.p5BenchmarkSample(ctx, env, name, run.ID)
		if sampleErr == nil {
			sampleErr = a.planV2ModelEvidence(ctx, env, run.Conversation, sample, nil)
		}
		failures = append(failures, sampleErr)
		trace, traceErr := a.postgresQuery(ctx, env, plan11InteractionTraceSQL(run.ID))
		if traceErr == nil {
			traceErr = writePlan11ToolEvidence(filepath.Join(env.Options.Artifacts, name+"-tools.json"), trace)
		}
		failures = append(failures, traceErr)
	}
	proof["recorded_run_count"] = len(runs)
	proof["provider_call_acceptance_limit_total"] = 128
	proof["provider_probe_calls_separate"] = true
	return errors.Join(failures...)
}

func plan11InteractionTraceSQL(run string) string {
	return fmt.Sprintf(`SELECT coalesce(json_agg(json_build_object('id',t.id,'tool',t.tool_id,'source',t.source,'input',t.input,'status',t.status,'error_code',t.error_code,'projection',r.projection,'projection_hash',encode(r.projection_hash,'hex')) ORDER BY t.created_at,t.call_sequence),'[]') FROM tool_calls t LEFT JOIN tool_results r ON r.tool_call_id=t.id AND r.enterprise_id=t.enterprise_id WHERE t.run_id='%s'`, run)
}
