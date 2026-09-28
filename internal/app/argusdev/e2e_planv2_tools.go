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

	"github.com/kakj-go/Argus/internal/dashboard"
)

func dashboardReplayStep(name string, args map[string]any) p5ToolStep {
	return p5ToolStep{"tool.invoke", map[string]any{"category": "dashboard", "name": name, "arguments": args}}
}
func replayResult(index int, field string) map[string]any {
	return map[string]any{"$result_pointer": fmt.Sprintf("/%d/summary/%s", index, field)}
}
func planV2ToolCatalogInput(resource string, at time.Time) map[string]any {
	return map[string]any{"source_binding": dashboard.SourceBinding{SourceType: "otlp", CapabilityVersion: "v1"}, "signal": "metrics", "kind": "metrics", "resource_ids": []string{resource}, "from": at.Add(-time.Hour), "to": at, "search": "argus_m7_e2e_gauge_planv2", "limit": 10, "filters": []any{}, "selected_values": []string{}}
}
func (a *App) planV2ToolRun(ctx context.Context, env *E2EEnvironment, convo, name, mode string, ids []string, steps []p5ToolStep) (string, error) {
	raw, _ := json.Marshal(steps)
	client, _ := scenarioHTTP(env)
	value, err := client.JSON(ctx, "p2-tools-"+name, "enterprise", http.MethodPost, "/conversations/"+convo+"/messages", 202, map[string]any{"content": "Execute deterministic protocol acceptance: argus_e2e_plan_b64:" + base64.RawURLEncoding.EncodeToString(raw), "file_ids": []string{}, "dashboard_context": map[string]any{"mode": mode, "dashboard_ids": ids}}, enterpriseHeaders(env, p5RequestKey("p2-tools", name)))
	if err != nil {
		return "", err
	}
	return stringField(value, "run", "run_id")
}

// Exercises the native model/tool protocol with fixed test inputs. No part of
// this report evaluates natural-language generation or analytical conclusions.
func (a *App) runPlanV2Tools(ctx context.Context, env *E2EEnvironment) (failure error) {
	proof := map[string]any{"kind": "planv2-deterministic-tool-protocol/v1", "actual_model_evaluated": false}
	for _, mode := range []string{"builder", "dsl"} {
		convo, err := a.p5Conversation(ctx, env, "PlanV2 native creation "+mode)
		if err != nil {
			return err
		}
		spec, err := planV2ThreeSignals()
		if err != nil {
			return err
		}
		spec.Panels = spec.Panels[:1]
		panel := &spec.Panels[0]
		panel.ApplicableResourceTypes = []string{"host", "kubernetes_cluster"}
		panel.Targets[0].SourceDefinition.DSL.Expression = "argus_m7_e2e_gauge_planv2"
		if mode == "builder" {
			panel.AuthoringMode = "builder"
			panel.Targets[0].SourceDefinition = dashboard.Definition{Builder: &dashboard.Builder{Operation: "value", Metric: "argus_m7_e2e_gauge_planv2", Filters: []dashboard.Filter{}, GroupBy: []string{}}}
		}
		name := "PlanV2 protocol " + mode
		input := planV2DraftRequest(spec)
		input["name"] = name
		now := time.Now().UTC()
		steps := []p5ToolStep{
			dashboardReplayStep("catalog", planV2ToolCatalogInput(env.State.Values["m3_cluster_id"], now)),
			dashboardReplayStep("draft.create", input),
			dashboardReplayStep("publish.preview", map[string]any{"draft_id": replayResult(1, "id"), "expected_version": replayResult(1, "draft_version")}),
		}
		run, err := a.planV2ToolRun(ctx, env, convo, "create-"+mode, "create", []string{}, steps)
		if err != nil {
			return err
		}
		if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM pending_actions WHERE run_id='"+run+"' AND action_type='telemetry.dashboard.publish' AND status='awaiting_confirmation';", "1", 2*time.Minute); err != nil {
			return err
		}
		// Reuse the same strict definition, Catalog and public host-confirmation
		// oracle as the optional model harness; generation is supplied by Replay.
		valid, err := a.verifyPlanV2ModelCreation(ctx, env, run, name, mode)
		if err != nil {
			return err
		}
		if !valid {
			return fmt.Errorf("deterministic %s creation failed", mode)
		}
		proof[mode+"_host_confirmed"] = true
	}
	convo, err := a.p5Conversation(ctx, env, "PlanV2 selected tool files")
	if err != nil {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()
		if err := a.p5DeleteWorkspace(c, env, convo); failure == nil {
			failure = err
		}
	}()
	ids := []string{env.State.Values["p2_files_dashboard_id"], env.State.Values["p2_native_mapping_id"]}
	steps := []p5ToolStep{}
	for _, id := range ids {
		index := len(steps)
		steps = append(steps, dashboardReplayStep("context.resolve", map[string]any{"dashboard_id": id, "expected_version": 0, "changes": map[string]any{}, "evidence": []any{}}), dashboardReplayStep("query", map[string]any{"dashboard_id": id, "context_ref": replayResult(index, "context_ref")}))
	}
	steps = append(steps, dashboardReplayStep("budget.get", map[string]any{}))
	run, err := a.planV2ToolRun(ctx, env, convo, "selected-files", "analyze", ids, steps)
	if err != nil {
		return err
	}
	if err = a.waitRunTerminal(ctx, env, run); err != nil {
		return err
	}
	jobs, err := a.postgresQuery(ctx, env, "SELECT id::text FROM dashboard_query_jobs WHERE run_id='"+run+"' ORDER BY created_at;")
	if err != nil {
		return err
	}
	if len(strings.Fields(jobs)) != 2 {
		return fmt.Errorf("selected dashboard native queries did not both enqueue")
	}
	for _, job := range strings.Fields(jobs) {
		if _, err := a.waitPlanV2Files(ctx, env, "/conversations/"+convo+"/dashboard-queries/"+job); err != nil {
			return err
		}
	}
	if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM dashboard_run_budgets WHERE run_id='"+run+"' AND calls_remaining<256;", "1", time.Minute); err != nil {
		return err
	}
	proof["two_dashboard_jobs"], proof["shared_run_budget"], proof["defaults_resolved_without_overrides"] = strings.Fields(jobs), true, true
	raw, _ := json.MarshalIndent(proof, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "planv2-tool-protocol.json"), raw)
}
