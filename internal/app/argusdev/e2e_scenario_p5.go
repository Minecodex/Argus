package argusdev

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type p5ToolStep struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

func (a *App) runP5Scenario(ctx context.Context, env *E2EEnvironment) error {
	if err := a.prepareP5Scenario(ctx, env); err != nil {
		return err
	}
	if err := a.verifyP5LiteInstallation(ctx, env); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(a.stdout, "P5 native tools and recovery started")
	if err := a.verifyP5Native(ctx, env); err != nil {
		return err
	}
	if err := a.verifyP5BusinessDiscovery(ctx, env); err != nil {
		return err
	}
	if err := a.verifyP5AgentRecovery(ctx, env); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(a.stdout, "P5 native tools and recovery passed")
	if err := a.verifyP5Responses(ctx, env); err != nil {
		return err
	}
	if err := a.verifyP5MissingUsage(ctx, env); err != nil {
		return err
	}
	if err := a.verifyP5InvalidUsage(ctx, env); err != nil {
		return err
	}
	if err := a.verifyP5ContextSources(ctx, env); err != nil {
		return err
	}
	if err := a.verifyP5MCPCredentialBoundary(ctx, env); err != nil {
		return err
	}
	for _, mode := range []string{"agent-lite", "agent-sandbox"} {
		if mode == "agent-sandbox" {
			if err := a.enableP5Sandbox(ctx, env); err != nil {
				return err
			}
			if err := a.configureP5Workspace(ctx, env); err != nil {
				return err
			}
			if err := a.configureP5IdleWindow(ctx, env); err != nil {
				return err
			}
		}
		_, _ = fmt.Fprintf(a.stdout, "P5 %s Remote MCP started\n", mode)
		if err := a.verifyP5MCP(ctx, env, mode); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(a.stdout, "P5 %s Remote MCP passed\n", mode)
		if mode == "agent-sandbox" {
			_, _ = fmt.Fprintln(a.stdout, "P5 Workspace files and offline execution started")
			if err := a.verifyP5Workspace(ctx, env); err != nil {
				return err
			}
			if err := a.verifyP5ImportedSourceRevocation(ctx, env); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(a.stdout, "P5 Workspace files and offline execution passed")
		}
	}
	stats, err := a.postgresQuery(ctx, env, `WITH selected_runs AS (
 SELECT r.* FROM conversations c JOIN runs r ON r.conversation_id=c.id WHERE c.title LIKE 'P5 %'
), model_totals AS (
 SELECT coalesce(sum(input_tokens) FILTER(WHERE input_usage_source='provider'),0) AS input_tokens, coalesce(sum(output_tokens) FILTER(WHERE output_usage_source='provider'),0) AS output_tokens, coalesce(sum(latency_ms),0) AS model_latency_ms
 FROM model_calls WHERE run_id IN (SELECT id FROM selected_runs)
), first_results AS (
 SELECT run_id,min(updated_at) AS first_at FROM tool_calls WHERE status='succeeded' AND tool_id NOT IN ('tool.search','tool.describe') GROUP BY run_id
) SELECT json_build_object('model','deterministic replay','runs',count(*),
 'succeeded',count(*) FILTER(WHERE r.status='succeeded'),
 'cancelled_runs',count(*) FILTER(WHERE r.status='cancelled'),
 'result_unknown_runs',count(*) FILTER(WHERE r.stop_reason='result_unknown'),
 'run_success_rate',coalesce(round(count(*) FILTER(WHERE r.status='succeeded')::numeric/nullif(count(*),0),4),0),
 'mean_first_effective_result_ms',coalesce(round(avg(extract(epoch FROM (t.first_at-r.created_at))*1000)),0),
 'input_tokens',(SELECT input_tokens FROM model_totals),'output_tokens',(SELECT output_tokens FROM model_totals),
 'model_latency_ms',(SELECT model_latency_ms FROM model_totals)) FROM selected_runs r LEFT JOIN first_results t ON t.run_id=r.id;`)
	if err != nil {
		return err
	}
	if err := writePrivate(filepath.Join(env.Options.Artifacts, "p5-statistics.json"), []byte(strings.TrimSpace(stats)+"\n")); err != nil {
		return err
	}
	if env.Options.RealModel != nil {
		return a.runP5RealModelBenchmark(ctx, env)
	}
	return nil
}

func (a *App) p5Conversation(ctx context.Context, env *E2EEnvironment, title string) (string, error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", err
	}
	// Display titles may contain spaces or Unicode; HTTP idempotency keys
	// use a bounded, deterministic ASCII identity instead.
	requestName := p5RequestKey("p5-conversation", title)
	value, err := client.JSON(ctx, requestName, "enterprise", http.MethodPost, "/conversations", http.StatusCreated, map[string]any{"title": "P5 " + title, "selected_model_id": env.State.Values["m4_model_id"]}, enterpriseHeaders(env, requestName))
	if err != nil {
		return "", err
	}
	return stringField(value, "id")
}
func (a *App) p5Run(ctx context.Context, env *E2EEnvironment, conversation, name string, steps []p5ToolStep, files []string) (string, error) {
	run, err := a.p5StartRun(ctx, env, conversation, name, steps, files)
	if err != nil {
		return "", err
	}
	if err := a.waitRunTerminal(ctx, env, run); err != nil {
		return run, err
	}
	return run, nil
}

func (a *App) p5StartRun(ctx context.Context, env *E2EEnvironment, conversation, name string, steps []p5ToolStep, files []string) (string, error) {
	data, _ := json.Marshal(steps)
	content := "Execute this deterministic acceptance plan: argus_e2e_plan_b64:" + base64.RawURLEncoding.EncodeToString(data)
	client, err := scenarioHTTP(env)
	if err != nil {
		return "", err
	}
	value, err := client.JSON(ctx, "p5-message-"+name, "enterprise", http.MethodPost, "/conversations/"+conversation+"/messages", http.StatusAccepted, map[string]any{"content": content, "file_ids": files}, enterpriseHeaders(env, p5RequestKey("p5-message", name)))
	if err != nil {
		return "", err
	}
	return stringField(value, "run", "run_id")
}

func p5RequestKey(prefix, value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%s-%x", prefix, digest[:16])
}

func (a *App) verifyP5Native(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	if _, err := client.JSON(ctx, "p5-reset-model-quota", "enterprise", http.MethodPost, "/enterprise/model-quotas", http.StatusOK, map[string]any{"model_id": env.State.Values["m4_model_id"], "subject_type": "user", "subject_id": env.State.Values["admin_user_id"], "monthly_amount": 1000}, enterpriseHeaders(env, "p5-reset-model-quota")); err != nil {
		return err
	}
	conversation, err := a.p5Conversation(ctx, env, "native")
	if err != nil {
		return err
	}
	run, err := a.p5Run(ctx, env, conversation, "native", []p5ToolStep{
		{"tool.search", map[string]any{"category": "host", "query": "list"}},
		{"tool.describe", map[string]any{"category": "host", "name": "list"}},
		{"tool.invoke", map[string]any{"category": "host", "name": "list", "arguments": map[string]any{}}},
	}, []string{})
	if err != nil {
		return err
	}
	call, err := a.postgresQuery(ctx, env, "SELECT tool_call_id::text FROM tool_presentations WHERE conversation_id='"+conversation+"' LIMIT 1;")
	if err != nil {
		return err
	}
	call = strings.TrimSpace(call)
	if call == "" {
		return fmt.Errorf("P5 native Tool presentation was not persisted")
	}
	path := "/conversations/" + conversation + "/tool-presentations/" + call
	first, err := client.JSON(ctx, "p5-template-before-recovery", "enterprise", http.MethodGet, path, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	if first["runtime"] != "argus-template/v1" || first["template_source"] == "" {
		return fmt.Errorf("P5 native template missing")
	}
	check, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM model_calls WHERE run_id='"+run+"' AND status='succeeded' AND (tool_snapshot_hash='' OR dispatched_at IS NULL OR octet_length(projection_hash)<>32);")
	if err != nil {
		return err
	}
	if strings.TrimSpace(check) != "0" {
		return fmt.Errorf("P5 model dispatch snapshot is incomplete")
	}
	password, err := dataCredentialValue(ctx, env, "redis-password")
	if err != nil {
		return err
	}
	if _, err := env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-redis", "redis", "redis-cli", "-a", password, "FLUSHALL"); err != nil {
		return err
	}
	if err := env.Kube.DeletePods(ctx, env.SystemNS, "app.kubernetes.io/name=argus-worker"); err != nil {
		return err
	}
	if err := env.Kube.WaitDeployment(ctx, env.SystemNS, "argus-worker", 5*time.Minute); err != nil {
		return err
	}
	after, err := client.JSON(ctx, "p5-template-after-recovery", "enterprise", http.MethodGet, path, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	if after["template_hash"] != first["template_hash"] || after["template_source"] != first["template_source"] {
		return fmt.Errorf("P5 historical template changed during recovery")
	}
	hidden, err := a.p5Run(ctx, env, conversation, "hidden-commit", []p5ToolStep{{"tool.invoke", map[string]any{"category": "host", "name": "create.commit", "arguments": map[string]any{}}}}, []string{})
	if err != nil {
		return err
	}
	count, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+hidden+"' AND status='failed' AND error_code='TOOL_NOT_FOUND';")
	if err != nil {
		return err
	}
	if strings.TrimSpace(count) != "1" {
		return fmt.Errorf("P5 hidden Commit was not rejected")
	}
	if err := a.verifyNativePreview(ctx, env, "p5-host-cancel", "host", "create.preview", map[string]any{"name": "p5-preview-only-" + env.Options.RunID, "platform": "linux", "role": "managed_host", "control_path": "direct", "install_method": "manual", "ssh_path": "none", "architecture": "amd64"}); err != nil {
		return err
	}
	env.State.Values["p5_native_conversation_id"] = conversation
	return nil
}
