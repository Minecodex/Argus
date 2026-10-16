package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func nativeTemplateStep(category, name string, input map[string]any) p5ToolStep {
	return p5ToolStep{"tool.invoke", map[string]any{"category": category, "name": name, "arguments": input}}
}

func (a *App) runToolTemplateScenario(ctx context.Context, env *E2EEnvironment) error {
	if err := a.configurePlanV2SelfMonitoring(ctx, env); err != nil {
		return err
	}
	if _, err := env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-server", "argus-server", "/usr/local/bin/argus-telemetry-e2e", "--endpoint=127.0.0.1:4317", "--marker=planv2", "--resource-id="+env.State.Values["m3_cluster_id"]); err != nil {
		return err
	}
	if err := a.waitPlanV2QueryReady(ctx, env); err != nil {
		return err
	}
	if err := a.templateREDObservations(ctx, env); err != nil {
		return err
	}
	if err := a.configureP5Workspace(ctx, env); err != nil {
		return err
	}
	// This acceptance exercises many unrelated tool results in one Chatbox.
	// Its deterministic provider declares a 1M context for repeated category
	// discovery and the expanded health fixture; it
	// does not test a model's reasoning or the separate compaction gate.
	client, _ := scenarioHTTP(env)
	model, err := client.JSON(ctx, "templates-model", "enterprise", http.MethodPost, "/enterprise/ai-models/test-and-create", 201, map[string]any{"name": "Template acceptance replay", "base_url": "https://argus-replay-model." + env.ReplayNamespace() + ".svc/v1", "model_id": "argus-replay-chat_completions", "api_protocol": "chat_completions", "api_key": "template-replay-fixture-key", "context_window_tokens": 1048576, "max_output_tokens": 1024, "input_price_per_million": 0.1, "output_price_per_million": 0.2}, enterpriseHeaders(env, "templates-model"))
	if err != nil {
		return err
	}
	modelID, err := stringField(model, "model", "id")
	if err != nil {
		return err
	}
	if _, err = client.JSON(ctx, "templates-quota", "enterprise", http.MethodPost, "/enterprise/model-quotas", 200, map[string]any{"model_id": modelID, "subject_type": "user", "subject_id": env.State.Values["admin_user_id"], "monthly_amount": 1000}, enterpriseHeaders(env, "templates-quota")); err != nil {
		return err
	}
	env.State.Values["m4_model_id"] = modelID
	spec, err := planV2ThreeSignals()
	if err != nil {
		return err
	}
	addTemplateREDPanel(&spec)
	addTemplateSamplingPanels(&spec)
	id, err := a.publishPlanV2Fixture(ctx, env, "Tool template three signals", spec)
	if err != nil {
		return err
	}
	gallery, err := planV2MetricGallery()
	if err != nil {
		return err
	}
	galleryID, err := a.publishPlanV2Fixture(ctx, env, "Tool template eleven charts", gallery)
	if err != nil {
		return err
	}
	cluster, host := env.State.Values["m3_cluster_id"], env.State.Values["m7_host_id"]
	collector, err := a.postgresQuery(ctx, env, "SELECT id::text FROM collector_instances WHERE enterprise_id='"+env.State.Values["enterprise_id"]+"' LIMIT 1;")
	if err != nil {
		return err
	}
	connector, err := a.postgresQuery(ctx, env, "SELECT id::text FROM connectors WHERE enterprise_id='"+env.State.Values["enterprise_id"]+"' LIMIT 1;")
	if err != nil {
		return err
	}
	version, err := a.postgresQuery(ctx, env, "SELECT resource_version FROM hosts WHERE id='"+host+"';")
	if err != nil {
		return err
	}
	var hostVersion int64
	if _, err = fmt.Sscan(strings.TrimSpace(version), &hostVersion); err != nil {
		return err
	}
	now := time.Now().UTC()
	common := map[string]any{"resource_ids": []string{cluster}, "from": now.Add(-time.Hour).Format(time.RFC3339), "to": now.Format(time.RFC3339)}
	metric := map[string]any{}
	for k, v := range common {
		metric[k] = v
	}
	metric["query"], metric["step_seconds"] = "argus_m7_e2e_gauge_planv2", float64(30)
	logs := map[string]any{}
	for k, v := range common {
		logs[k] = v
	}
	logs["query"] = "* | limit 10"
	trace := map[string]any{}
	for k, v := range common {
		trace[k] = v
	}
	trace["document"] = spec.Panels[2].Targets[0].SourceDefinition.DSL.Expression
	policyID, eventID, err := a.templateBusinessFacts(ctx, env)
	if err != nil {
		return err
	}
	if err = a.templateConnectorFacts(ctx, env); err != nil {
		return err
	}
	steps := []p5ToolStep{nativeTemplateStep("host", "list", map[string]any{}), nativeTemplateStep("host", "get", map[string]any{"host_id": host}), nativeTemplateStep("k8s", "cluster.list", map[string]any{}), nativeTemplateStep("k8s", "cluster.get", map[string]any{"cluster_id": cluster}), nativeTemplateStep("connector", "list", map[string]any{}), nativeTemplateStep("connector", "get", map[string]any{"connector_id": strings.TrimSpace(connector)}), nativeTemplateStep("connector", "collector.list", map[string]any{}), nativeTemplateStep("connector", "collector.get", map[string]any{"collector_id": strings.TrimSpace(collector)}), nativeTemplateStep("metric", "query", metric), nativeTemplateStep("log", "query", logs), nativeTemplateStep("trace", "query", trace), nativeTemplateStep("alert", "policy.list", map[string]any{}), nativeTemplateStep("alert", "event.query", map[string]any{}), nativeTemplateStep("alert", "history.query", map[string]any{}), nativeTemplateStep("alert", "notification.query", map[string]any{})}
	steps = append(steps, nativeTemplateStep("alert", "policy.get", map[string]any{"id": policyID}), nativeTemplateStep("alert", "event.get", map[string]any{"id": eventID}))
	steps = append(steps, nativeTemplateStep("alert", "event.get", map[string]any{"id": env.State.Values["template_any_instance_id"]}), nativeTemplateStep("connector", "get", map[string]any{"connector_id": env.State.Values["template_host_connector_id"]}), nativeTemplateStep("connector", "get", map[string]any{"connector_id": env.State.Values["template_cluster_connector_id"]}))
	for _, health := range []string{"error", "no_data", "authorization_denied", "recovered"} {
		steps = append(steps, nativeTemplateStep("alert", "event.get", map[string]any{"id": env.State.Values["template_health_"+health]}))
	}
	for _, health := range []string{"no_data", "error"} {
		steps = append(steps, nativeTemplateStep("alert", "event.get", map[string]any{"id": env.State.Values["template_kind_"+health]}))
	}
	queries, err := a.templateQueryShapeSteps(ctx, env, common)
	if err != nil {
		return err
	}
	steps = append(steps, queries...)
	steps = append(steps, templateCompletenessSteps(common)...)
	steps = append(steps, templateSamplingSteps(cluster)...)
	catalogSteps := []p5ToolStep{}
	for _, kind := range []string{"metrics", "fields", "values"} {
		input := map[string]any{"source_binding": map[string]any{"source_type": "otlp", "capability_version": "v1"}, "signal": "metrics", "kind": kind, "resource_ids": []string{cluster}, "from": common["from"], "to": common["to"], "limit": 20, "selected_values": []string{}, "filters": []any{}}
		if kind == "values" {
			input["field"] = "__name__"
			input["selected_values"] = []string{"argus_m7_e2e_gauge_planv2"}
		}
		catalogSteps = append(catalogSteps, dashboardReplayStep("catalog", input))
	}
	catalogSteps = append(catalogSteps, dashboardReplayStep("catalog.resources", map[string]any{}))
	// Alert tools deliberately require a successful, earlier exact description.
	// Keep the real gate, and include discovery cards in this acceptance chain.
	batches := [][]p5ToolStep{}
	prepared := []p5ToolStep{}
	for _, step := range steps {
		group := []p5ToolStep{}
		if step.Arguments["category"] == "alert" {
			name := step.Arguments["name"].(string)
			group = append(group, p5ToolStep{"tool.search", map[string]any{"category": "alert"}}, p5ToolStep{"tool.describe", map[string]any{"category": "alert", "name": name}})
		}
		group = append(group, step)
		if len(prepared)+len(group) > 23 {
			batches = append(batches, prepared)
			prepared = []p5ToolStep{}
		}
		prepared = append(prepared, group...)
	}
	// Keep related discovery/describe/invoke steps within the same Run, and
	// respect the production 24-round Run limit without enlarging that limit.
	if len(prepared) > 0 {
		batches = append(batches, prepared)
	}
	catalogBatch := len(batches)
	batches = append(batches, catalogSteps)
	steps = []p5ToolStep{}
	for _, dashboardID := range []string{id, galleryID} {
		index := len(steps)
		steps = append(steps, dashboardReplayStep("get", map[string]any{"dashboard_id": dashboardID}), dashboardReplayStep("context.resolve", map[string]any{"dashboard_id": dashboardID, "expected_version": 0, "changes": map[string]any{"resources": map[string]any{"all": false, "ids": []string{cluster}}}, "evidence": []any{map[string]any{"path": "/resources", "origin": "ai_selection", "reason": "Use one authorized cluster so histogram bucket identities are not merged across resources"}}}), dashboardReplayStep("query", map[string]any{"dashboard_id": dashboardID, "context_ref": replayResult(index+1, "context_ref")}))
	}
	batches = append(batches, steps)
	convo, err := a.p5Conversation(ctx, env, "Tool templates real chain")
	if err != nil {
		return err
	}
	runs := []string{}
	for index, batch := range batches {
		mode := "none"
		// Native data discovery belongs to the existing dashboard authoring
		// mode; preserve the real gate rather than weakening its permission.
		if index == catalogBatch {
			mode = "create"
		}
		run, err := a.planV2ToolRun(ctx, env, convo, fmt.Sprintf("template-v2-%d", index), mode, []string{}, batch)
		if err != nil {
			return err
		}
		if err = a.waitRunTerminal(ctx, env, run); err != nil {
			return err
		}
		runs = append(runs, run)
	}
	failed, err := a.postgresQuery(ctx, env, "SELECT t.tool_id||':'||COALESCE(t.error_code,'') FROM tool_calls t JOIN runs r ON r.id=t.run_id WHERE r.conversation_id='"+convo+"' AND t.status NOT IN ('succeeded');")
	if err != nil {
		return err
	}
	if strings.TrimSpace(failed) != "" {
		return fmt.Errorf("native template tools failed: %s", failed)
	}
	jobs, err := a.postgresQuery(ctx, env, "SELECT id::text FROM dashboard_query_jobs WHERE conversation_id='"+convo+"' ORDER BY created_at;")
	if err != nil {
		return err
	}
	if len(strings.Fields(jobs)) != 2 {
		return fmt.Errorf("expected two independent Query jobs, got %s", jobs)
	}
	for _, job := range strings.Fields(jobs) {
		if _, err = a.waitPlanV2Files(ctx, env, "/conversations/"+convo+"/dashboard-queries/"+job); err != nil {
			return err
		}
	}
	if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM tool_presentation_snapshots s JOIN tool_presentations p ON p.id=s.presentation_id WHERE p.conversation_id='"+convo+"';", "2", time.Minute); err != nil {
		return err
	}
	calls, err := a.postgresQuery(ctx, env, "SELECT tool_call_id::text FROM tool_presentations WHERE conversation_id='"+convo+"' ORDER BY created_at;")
	if err != nil {
		return err
	}
	previewConversation, err := a.p5Conversation(ctx, env, "Tool templates change preview")
	if err != nil {
		return err
	}
	if err = a.templatePresentationRecovery(ctx, env, convo, strings.Fields(calls)); err != nil {
		return err
	}
	previewRun, err := a.planV2ToolRun(ctx, env, previewConversation, "template-v2-preview", "none", []string{}, []p5ToolStep{nativeTemplateStep("host", "update.preview", map[string]any{"host_id": host, "expected_version": hostVersion, "name": "Tool template owned host preview"})})
	if err != nil {
		return err
	}
	if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM pending_actions WHERE run_id='"+previewRun+"' AND status='awaiting_confirmation';", "1", time.Minute); err != nil {
		return err
	}
	evidence := []map[string]any{}
	for index, call := range strings.Fields(calls) {
		if uuid.Validate(call) != nil {
			return fmt.Errorf("invalid saved ToolCall identity")
		}
		path := "/conversations/" + convo + "/tool-presentations/" + call
		view, err := client.JSON(ctx, fmt.Sprintf("templates-get-%d", index), "enterprise", http.MethodGet, path, 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		if view["runtime"] != "argus-template/v2" {
			return fmt.Errorf("non-v2 native presentation %s", call)
		}
		data, err := client.JSON(ctx, fmt.Sprintf("templates-read-%d", index), "enterprise", http.MethodPost, path+"/read", 200, map[string]any{"operation": "result.detail", "parameters": map[string]any{}}, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		fact := map[string]any{"tool_call_id": call, "hash": view["template_hash"], "runtime": view["runtime"], "read_status": data["status"]}
		if detail, ok := data["data"].(map[string]any); ok {
			metricProof, e := validateTemplateMetricPreview(detail)
			if e != nil {
				encoded, _ := json.MarshalIndent(redactJSON(detail), "", "  ")
				_ = writePrivate(filepath.Join(env.Options.Artifacts, "metric-sampling-failed-read.json"), encoded)
				return e
			}
			if len(metricProof) > 0 {
				fact["metric_sampling"] = metricProof
				if detail["view"] == "metric-query" {
					if e = a.templateExactMetricPages(ctx, env, path, 600); e != nil {
						return e
					}
					fact["exact_metric_samples"] = 600
				}
			}
			fact["tool"], fact["entry"] = detail["tool"], detail["view"]
			if detail["view"] == "dashboard-result" {
				exact, e := a.templateDashboardMetricPages(ctx, env, path, detail)
				if e != nil {
					return e
				}
				fact["exact_dashboard_metric_samples"] = exact
				proof, e := a.templateDashboardAPMProof(ctx, env, path, detail)
				if e != nil {
					return e
				}
				fact["dashboard_apm_previews"] = proof
			}
			fact["log_result_type"] = detail["result_type"]
			if rows, ok := detail["items"].([]any); ok && detail["view"] == "log-query" && len(rows) > 0 {
				fact["log_first"] = rows[0]
			}
			if views, ok := detail["result"].([]any); ok && detail["view"] == "trace-result" {
				fact["trace_views"] = views
				for root, raw := range views {
					view, _ := raw.(map[string]any)
					if err := validateTemplateAPMLimitView(view); err != nil {
						return err
					}
					key, _ := view["key"].(string)
					if strings.HasPrefix(key, "template_red_") {
						if err := a.templateExactAPMPages(ctx, env, path, root, 600); err != nil {
							return err
						}
						fact["exact_apm_rows_per_root"] = 600
					}
				}
			}
			fact["result_id"] = detail["id"]
			if rows, ok := detail["items"].([]any); ok && detail["view"] == "connector-list" {
				fact["connectors"] = rows
			}
			if instance, ok := detail["instance"].(map[string]any); ok {
				fact["result_id"], fact["alert_health"] = instance["id"], instance["health"]
				fact["last_evaluated_at"] = instance["last_evaluated_at"]
				if state, ok := instance["state"].(map[string]any); ok {
					fact["alert_name"], fact["last_successful_at"], fact["alert_kind"] = state["name"], state["last_successful_at"], state["kind"]
				}
			}
			if relations, ok := detail["related_resources"].([]any); ok {
				fact["related_resources"] = relations
			}
			if collector, ok := detail["collector"].(map[string]any); ok {
				value, present := collector["last_seen_at"]
				fact["collector_last_seen_at"], fact["collector_last_seen_present"] = value, present
			}
		}
		evidence = append(evidence, fact)
	}
	_, err = client.JSON(ctx, "templates-unbound-parameters", "enterprise", http.MethodPost, "/conversations/"+convo+"/tool-presentations/"+strings.Fields(calls)[0]+"/read", 422, map[string]any{"operation": "result.detail", "parameters": map[string]any{"resource_id": uuid.New().String()}}, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	var redFixture map[string]any
	_ = json.Unmarshal([]byte(env.State.Values["template_red_fixture"]), &redFixture)
	payload, _ := json.MarshalIndent(map[string]any{"kind": "tool-template-v2-real-chain", "actual_model_evaluated": false, "conversation_id": convo, "run_ids": runs, "jobs": strings.Fields(jobs), "presentations": evidence, "sealed_attempts": 2, "red_fixture": redFixture}, "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "tool-template-v2-real-chain.json"), payload); err != nil {
		return err
	}
	return a.runPlaywright(ctx, env, "e2e/tool-template-v2-real.spec.ts", map[string]string{"ARGUS_TEMPLATE_E2E": "1", "ARGUS_TEMPLATE_READ_EVIDENCE": filepath.Join(env.Options.Artifacts, "tool-template-v2-real-chain.json"), "ARGUS_TEMPLATE_CONVERSATION": convo, "ARGUS_TEMPLATE_PRESENTATION_COUNT": fmt.Sprint(len(evidence)), "ARGUS_TEMPLATE_PREVIEW_CONVERSATION": previewConversation, "ARGUS_TEMPLATE_USERNAME": env.State.Values["enterprise_username"], "ARGUS_TEMPLATE_PASSWORD": env.State.Values["enterprise_password"]})
}

func (a *App) templateReadIntegration(ctx context.Context, env *E2EEnvironment) (result error) {
	restore, err := a.pauseTemplateContractWorker(ctx, env)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, restore()) }()
	password, err := dataCredentialValue(ctx, env, "postgresql-password")
	if err != nil {
		return err
	}
	forward, err := env.Kube.PortForwardService(ctx, env.SystemNS, "argus-postgresql", []string{"0:5432"}, io.Discard)
	if err != nil {
		return err
	}
	defer forward.Stop()
	// Evaluation PostgreSQL uses its existing plain local socket protocol. This
	// connection is confined to this run's loopback Kubernetes port-forward.
	connection := url.URL{Scheme: "postgres", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(int(forward.Ports[0].Local))), Path: "argus", User: url.UserPassword("argus", password), RawQuery: "sslmode=disable"}
	testEnv := map[string]string{"ARGUS_TEMPLATE_TEST_DATABASE_URL": connection.String(), "ARGUS_P5_TEST_DATABASE_URL": connection.String(), "ARGUS_DASHBOARD_TEST_DATABASE_URL": connection.String(), "ARGUS_ALERT_TEST_DATABASE_URL": connection.String()}
	if err := a.runner.Run(ctx, testEnv, "go", "test", "./internal/alerting", "-run", "TestPostgresEvaluationIsolatesMissingRulesAndKeepsCompositeUnknown", "-count=1", "-v"); err != nil {
		return err
	}
	if err := a.runner.Run(ctx, testEnv, "go", "test", "./internal/transport/httpapi", "-run", "TestPostgresPresentationReadOwnershipRevocationPagingAndHistoricalHash|TestConversationSSESlowConsumersStayBoundedAndCancel", "-count=1", "-v"); err != nil {
		return err
	}
	if err := a.runner.Run(ctx, testEnv, "go", "test", "./internal/toolgateway", "-run", "TestPostgresConnectorPresentationRelationsRespectCurrentAccess", "-count=1", "-v"); err != nil {
		return err
	}
	return a.runner.Run(ctx, testEnv, "go", "test", "./internal/dashboard", "-run", "TestPostgresAutomaticAnalysisCorrectionAndFrozenPresentation|TestPostgresPresentationSealingIdempotencyFailureAndAttemptIsolation", "-count=1", "-v")
}
