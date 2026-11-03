package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

func (a *App) plan11AuthoringBeforeConfirmation(ctx context.Context, env *E2EEnvironment, run, host string, row plan11AuthoringCase) error {
	raw, err := a.postgresQuery(ctx, env, plan11AuthoringBeforeSQL(run, host, env.State.Values["enterprise_id"], row))
	if err != nil {
		return err
	}
	var fields map[string]int
	if json.Unmarshal([]byte(raw), &fields) != nil {
		return errors.New("invalid before-confirmation counters")
	}
	if fields["validations"] < 1 || fields["previews"] != 1 || fields["other_actions"] != 0 || fields["published"] != 0 {
		return errors.New("publication validation/confirmation boundary incomplete")
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan11-authoring-"+row.Decision+"-before.json"), []byte(raw))
}

// Both entry points invoke the same scoped verifier. Require the current draft
// version and actual exact-Host sample, rather than an extra duplicate call.
func plan11AuthoringBeforeSQL(run, host, enterprise string, row plan11AuthoringCase) string {
	return fmt.Sprintf(`WITH checks AS (
 SELECT CASE WHEN t.input->>'name'='publish.preview' THEN r.projection->'summary'->'preview'->'validation' ELSE r.projection->'summary'->'validation' END AS validation,
 CASE WHEN t.input->>'name'='publish.preview' THEN (r.projection->'summary'->'preview'->>'sample_json')::jsonb ELSE r.projection->'summary'->'sample' END AS sample
 FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id AND r.enterprise_id=t.enterprise_id
 JOIN dashboard_drafts d ON d.enterprise_id=t.enterprise_id AND d.id::text=t.input->'arguments'->>'draft_id' AND d.status='editing' AND d.draft_version::text=t.input->'arguments'->>'expected_version'
 WHERE t.run_id='%s' AND t.status='succeeded' AND t.input->>'category'='dashboard' AND t.input->>'name' IN ('draft.validate','publish.preview') AND t.input->'arguments'->>'draft_id'='%s'
 ) SELECT json_build_object(
'validations',(SELECT count(*) FROM checks WHERE validation->>'valid'='true' AND jsonb_array_length(coalesce(sample->'execution'->'resources','[]'::jsonb))=1 AND sample->'execution'->'resources'->0->>'type'='host' AND sample->'execution'->'resources'->0->>'id'='%s' AND sample->'execution'->>'partial'='false'),
'previews',(SELECT count(*) FROM pending_actions WHERE run_id='%s' AND action_type='telemetry.dashboard.publish' AND status='awaiting_confirmation'),
'other_actions',(SELECT count(*) FROM pending_actions WHERE run_id='%s' AND action_type<>'telemetry.dashboard.publish'),
'published',(SELECT count(*) FROM dashboard_revisions WHERE enterprise_id='%s' AND name='%s'))`, run, row.Draft, host, run, run, enterprise, row.Name)
}

func (a *App) plan11AuthoringFinalState(ctx context.Context, env *E2EEnvironment, row plan11AuthoringCase) error {
	if uuid.Validate(row.Draft) != nil {
		return errors.New("invalid exact authoring draft identity")
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for {
		raw, err := a.postgresQuery(bounded, env, plan11AuthoringStateSQL(env.State.Values["enterprise_id"], row))
		if err != nil {
			return err
		}
		var state struct {
			DraftStatus string  `json:"draft_status"`
			Dashboard   *string `json:"dashboard_id"`
			Revisions   int
			Published   int    `json:"published_named"`
			Action      string `json:"action_status"`
			Active      int    `json:"active_runs"`
		}
		if json.Unmarshal([]byte(raw), &state) != nil {
			return errors.New("invalid final authoring counters")
		}
		valid := row.Decision == "cancel" && state.Action == "cancelled" && state.DraftStatus == "editing" && state.Dashboard == nil && state.Revisions == 0 && state.Published == 0
		valid = valid || row.Decision == "confirm" && state.Action == "succeeded" && state.DraftStatus == "published" && state.Dashboard != nil && state.Revisions == 1 && state.Published == 1
		if valid && state.Active == 0 {
			return writePrivate(filepath.Join(env.Options.Artifacts, "plan11-authoring-"+row.Decision+"-after.json"), []byte(raw))
		}
		if err = waitContext(bounded, time.Second); err != nil {
			return errors.Join(err, errors.New("actual UI decision did not converge to its single publication/cancellation outcome"))
		}
	}
}

func plan11AuthoringStateSQL(enterprise string, row plan11AuthoringCase) string {
	return fmt.Sprintf(`SELECT json_build_object('draft_status',d.status,'dashboard_id',d.dashboard_id,
'revisions',(SELECT count(*) FROM dashboard_revisions r WHERE r.enterprise_id=d.enterprise_id AND r.dashboard_id=d.dashboard_id),
'published_named',(SELECT count(*) FROM dashboard_revisions r WHERE r.enterprise_id=d.enterprise_id AND r.name=d.name),
'action_status',(SELECT status FROM pending_actions WHERE action_ref='%s' AND enterprise_id=d.enterprise_id),
'active_runs',(SELECT count(*) FROM runs WHERE conversation_id='%s' AND status IN ('pending','running','waiting_system')))
FROM dashboard_drafts d WHERE d.id='%s' AND d.enterprise_id='%s'`, row.Action, row.Conversation, row.Draft, enterprise)
}

func (a *App) plan11AuthoringEvidence(ctx context.Context, env *E2EEnvironment, model string, proof map[string]any) error {
	if uuid.Validate(model) != nil {
		return errors.New("invalid authoring model identity")
	}
	raw, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT coalesce(json_agg(json_build_object('id',r.id,'conversation',r.conversation_id,'status',r.status) ORDER BY r.created_at),'[]') FROM runs r JOIN conversations c ON c.id=r.conversation_id AND c.enterprise_id=r.enterprise_id WHERE r.model_id='%s' AND r.enterprise_id='%s' AND c.title IN ('Plan7 plan11-authoring-zh','Plan7 plan11-authoring-en')`, model, env.State.Values["enterprise_id"]))
	if err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan11-authoring-runs.json"), []byte(raw)); err != nil {
		return err
	}
	var runs []struct{ ID, Conversation, Status string }
	if json.Unmarshal([]byte(raw), &runs) != nil {
		return errors.New("invalid authoring run ledger")
	}
	var failure error
	calls := 0
	for i, run := range runs {
		if uuid.Validate(run.ID) != nil || uuid.Validate(run.Conversation) != nil {
			return errors.New("foreign or missing authoring run identity")
		}
		name := fmt.Sprintf("plan11-authoring-run-%02d", i+1)
		sample, e := a.p5BenchmarkSample(ctx, env, name, run.ID)
		if e != nil {
			failure = errors.Join(failure, e)
			continue
		}
		calls += sample.ModelCalls
		if sample.ModelCalls > 0 && (!sample.UsageComplete || !sample.CachedUsageComplete) {
			failure = errors.Join(failure, errors.New("authoring continuation usage incomplete"))
		}
		failure = errors.Join(failure, a.planV2ModelEvidence(ctx, env, run.Conversation, sample, nil), a.plan11MetadataModelLedger(ctx, env, name, run.ID, sample.ModelCalls))
		trace, e := a.postgresQuery(ctx, env, plan11InteractionTraceSQL(run.ID))
		if e == nil {
			e = writePlan11ToolEvidence(filepath.Join(env.Options.Artifacts, name+"-tools.json"), trace)
		}
		failure = errors.Join(failure, e)
		if e == nil {
			failure = errors.Join(failure, plan11AuthoringTraceAllowed(trace))
		}
	}
	proof["provider_calls"], proof["provider_acceptance_bound"], proof["provider_probe_calls_separate"] = calls, 64, true
	proof["recorded_runs"] = len(runs)
	if calls < 2 || calls > 64 {
		failure = errors.Join(failure, errors.New("authoring provider call count outside declared cohort bound"))
	}
	return failure
}
