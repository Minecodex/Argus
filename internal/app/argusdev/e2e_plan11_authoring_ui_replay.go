package argusdev

import (
	"context"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/dashboard"
)

//go:embed fixtures/plan11-authoring-recorded.json
var plan11AuthoringRecordedJSON []byte

type plan11RecordedAuthoring struct {
	Locale           string         `json:"locale"`
	OriginalTrace    string         `json:"original_trace"`
	OriginalTraceSHA string         `json:"original_trace_sha256"`
	OriginalCall     string         `json:"original_tool_call_id"`
	Spec             dashboard.Spec `json:"spec"`
}

func plan11RecordedAuthoringSpecs() ([]plan11RecordedAuthoring, error) {
	var rows []plan11RecordedAuthoring
	if json.Unmarshal(plan11AuthoringRecordedJSON, &rows) != nil || len(rows) != 2 {
		return nil, errors.New("recorded authoring specifications are unavailable")
	}
	for i, row := range rows {
		digest, err := hex.DecodeString(row.OriginalTraceSHA)
		if err != nil || len(digest) != 32 || uuid.Validate(row.OriginalCall) != nil || row.Locale != []string{"zh", "en"}[i] || !plan11AuthoringSpecMatches(row.Spec) {
			return nil, errors.New("recorded authoring specification or provenance is invalid")
		}
		for _, panel := range row.Spec.Panels {
			if panel.Unit != "percent_ratio" {
				return nil, errors.New("recorded ratio unit changed")
			}
		}
	}
	return rows, nil
}

// This is L4 with a local scripted provider and real native services/telemetry.
// Autonomous L5 remains the immutable R69/R70 external-provider evidence.
func (a *App) plan11DeployedAuthoringUIReplay(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (failure error) {
	if env.Options.Plan8Phase != "plan11-authoring-ui-replay" || host == nil || len(plan8SelectedModelNames(env.Options.Plan8Phase)) != 0 {
		return errors.New("authoring UI replay requires its explicit zero external-model phase")
	}
	fixtures, err := plan11RecordedAuthoringSpecs()
	if err != nil {
		return err
	}
	proof := map[string]any{"schema_version": "argus.plan11_authoring_ui_replay/v1", "model_kind": "local_scripted_replay", "new_L5_evidence": false, "whole_plan_complete": false, "fixtures": fixtures}
	proof["backend_build"] = "m4e2e fixture prerequisite; normal production readback is a separate mandatory stage"
	cases := []plan11AuthoringCase{}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		failure = errors.Join(failure, a.plan11AuthoringReplayEvidence(cleanup, env, cases, proof))
		proof["cases"], proof["passed"] = cases, failure == nil
		if failure != nil {
			proof["error"] = string(redactDiagnostic([]byte(failure.Error())))
		}
		raw, e := json.MarshalIndent(proof, "", "  ")
		failure = errors.Join(failure, e, writePrivate(filepath.Join(env.Options.Artifacts, "plan11-authoring-ui-replay.json"), raw))
	}()
	for _, fixture := range fixtures {
		row, e := a.plan11AuthoringReplayCase(ctx, env, host, fixture)
		if row.Conversation != "" {
			cases = append(cases, row)
		}
		if e != nil {
			return e
		}
	}
	cookies, err := existingPortalBrowserCookies(env)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(cases)
	if err = a.runPlaywright(ctx, env, "plan11-authoring-real.spec.ts", map[string]string{
		"ARGUS_PLAN11_AUTHORING_REAL": "1", "ARGUS_PLAN11_AUTHORING_CASES": string(raw),
		"ARGUS_PLAN11_AUTHORING_COOKIES": cookies, "ARGUS_E2E_ARTIFACT_SUFFIX": "plan11-authoring-ui-replay",
	}); err != nil {
		return err
	}
	for _, row := range cases {
		failure = errors.Join(failure, a.plan11AuthoringFinalState(ctx, env, row))
	}
	return failure
}

func (a *App) plan11AuthoringReplayCase(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop, fixture plan11RecordedAuthoring) (row plan11AuthoringCase, failure error) {
	row = plan11AuthoringCase{Name: "Plan11 recorded UI " + fixture.Locale + " " + uuid.NewString(), Decision: "cancel"}
	if fixture.Locale == "en" {
		row.Decision = "confirm"
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return row, err
	}
	input := planV2DraftRequest(fixture.Spec)
	input["name"], input["description"] = row.Name, "Recorded real-model specification; current real Host samples; local scripted UI acceptance."
	input["proposed_bindings"] = []dashboard.Binding{{ResourceType: "host", ResourceID: uuid.MustParse(host.HostID)}}
	draft, err := client.JSON(ctx, "recorded-authoring-create-"+fixture.Locale, "enterprise", http.MethodPost, "/dashboard-drafts", 201, input, enterpriseHeaders(env, "recorded-authoring-create-"+fixture.Locale))
	if err != nil {
		return row, err
	}
	row.Draft, err = stringField(draft, "id")
	if err != nil {
		return row, err
	}
	row.Conversation, err = a.p5Conversation(ctx, env, "plan11-recorded-authoring-"+fixture.Locale)
	if err != nil {
		return row, err
	}
	to := time.Now().UTC().Truncate(time.Second)
	parameters := map[string]any{"resource_mode": "explicit", "resource_types": []string{"host"}, "resource_ids": []string{host.HostID}, "from": to.Add(-30 * time.Minute).Format(time.RFC3339), "to": to.Format(time.RFC3339)}
	arguments := map[string]any{"draft_id": row.Draft, "expected_version": draft["draft_version"], "parameters": parameters}
	run, err := a.planV2ToolRun(ctx, env, row.Conversation, "recorded-authoring-"+fixture.Locale, "create", []string{}, []p5ToolStep{
		{"tool.search", map[string]any{"category": "dashboard"}},
		{"tool.describe", map[string]any{"category": "dashboard", "name": "draft.validate"}},
		{"tool.invoke", map[string]any{"category": "dashboard", "name": "draft.validate", "arguments": arguments}},
		{"tool.describe", map[string]any{"category": "dashboard", "name": "publish.preview"}},
		{"tool.invoke", map[string]any{"category": "dashboard", "name": "publish.preview", "arguments": arguments}},
	})
	if err != nil {
		return row, err
	}
	if err = a.waitRunStatus(ctx, env, run, "waiting_input", 90*time.Second); err != nil {
		return row, err
	}
	action, err := a.postgresQuery(ctx, env, "SELECT action_ref FROM pending_actions WHERE run_id='"+run+"' AND action_type='telemetry.dashboard.publish' AND status='awaiting_confirmation'")
	if err != nil || len(strings.Fields(action)) != 1 {
		return row, errors.Join(err, errors.New("recorded native preview lacks its exact pending action"))
	}
	row.Action = strings.TrimSpace(action)
	sample, err := a.postgresQuery(ctx, env, "SELECT preview->>'sample_json' FROM pending_actions WHERE action_ref='"+row.Action+"'")
	if err != nil || !json.Valid([]byte(sample)) {
		return row, errors.Join(err, errors.New("recorded preview sample is unavailable"))
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan11-authoring-ui-original-"+row.Decision+"-sample.json"), []byte(sample)); err != nil {
		return row, err
	}
	return row, a.plan11AuthoringBeforeConfirmation(ctx, env, run, host.HostID, row)
}

func (a *App) plan11AuthoringReplayEvidence(ctx context.Context, env *E2EEnvironment, cases []plan11AuthoringCase, proof map[string]any) error {
	var failure error
	calls := 0
	for _, row := range cases {
		if uuid.Validate(row.Conversation) != nil {
			return errors.New("invalid replay conversation")
		}
		raw, err := a.postgresQuery(ctx, env, "SELECT coalesce(json_agg(json_build_object('id',id,'model',model_id) ORDER BY created_at),'[]') FROM runs WHERE conversation_id='"+row.Conversation+"'")
		if err != nil {
			return err
		}
		var runs []struct{ ID, Model string }
		if json.Unmarshal([]byte(raw), &runs) != nil {
			return errors.New("invalid replay run ledger")
		}
		for i, run := range runs {
			if uuid.Validate(run.ID) != nil || run.Model != env.State.Values["m4_model_id"] {
				return errors.New("replay dispatched a foreign provider")
			}
			name := fmt.Sprintf("plan11-authoring-ui-%s-run-%02d", row.Decision, i+1)
			sample, e := a.p5BenchmarkSample(ctx, env, name, run.ID)
			if e != nil {
				failure = errors.Join(failure, e)
				continue
			}
			calls += sample.ModelCalls
			failure = errors.Join(failure, a.plan11MetadataModelLedger(ctx, env, name, run.ID, sample.ModelCalls))
			trace, e := a.postgresQuery(ctx, env, plan11InteractionTraceSQL(run.ID))
			if e == nil {
				e = writePlan11ToolEvidence(filepath.Join(env.Options.Artifacts, name+"-tools.json"), trace)
			}
			failure = errors.Join(failure, e)
			if e == nil {
				failure = errors.Join(failure, plan11AuthoringTraceAllowed(trace))
			}
		}
	}
	proof["local_scripted_provider_calls"], proof["external_business_model_calls"] = calls, 0
	return failure
}
