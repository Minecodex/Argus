package argusdev

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// P5 owns its fixtures. Unrelated onboarding/governance regressions retain
// their M3/M4/P4/M7/M10 gates without delaying every Workspace fault check.
func (a *App) prepareP5Scenario(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	roles, err := client.JSON(ctx, "p5-roles", "enterprise", http.MethodGet, "/enterprise/roles", 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	role, err := findItem(objectItems(roles), func(item map[string]any) bool {
		return item["builtin_key"] == "resource_admin" && item["builtin"] == true
	})
	if err != nil {
		return err
	}
	roleID, err := stringField(role, "id")
	if err != nil {
		return err
	}
	if _, err := client.JSON(ctx, "p5-resource-role", "enterprise", http.MethodPost, "/enterprise/role-bindings", 201, map[string]any{"subject_type": "user", "subject_id": env.State.Values["admin_user_id"], "role_id": roleID}, enterpriseHeaders(env, "p5-resource-role")); err != nil {
		return err
	}
	if err := a.refreshEnterpriseLogin(ctx, env); err != nil {
		return err
	}
	for _, protocol := range []string{"chat_completions", "responses"} {
		value, err := client.JSON(ctx, "p5-model-"+protocol, "enterprise", http.MethodPost, "/enterprise/ai-models/test-and-create", 201,
			map[string]any{"name": "P5 Replay " + protocol, "base_url": "https://argus-replay-model." + env.ReplayNamespace() + ".svc/v1", "model_id": "argus-replay-" + protocol, "api_protocol": protocol, "api_key": "p5-write-only-key", "context_window_tokens": 32768, "max_output_tokens": 1024, "input_price_per_million": 0.1, "output_price_per_million": 0.2}, enterpriseHeaders(env, "p5-model-"+protocol))
		if err != nil {
			return err
		}
		if value["compatible"] != true {
			return fmt.Errorf("P5 %s model compatibility failed", protocol)
		}
		id, err := stringField(value, "model", "id")
		if err != nil {
			return err
		}
		env.State.Values["p5_model_"+protocol] = id
		if protocol == "chat_completions" {
			env.State.Values["m4_model_id"] = id
		}
		if _, err := client.JSON(ctx, "p5-quota-"+protocol, "enterprise", http.MethodPost, "/enterprise/model-quotas", 200, map[string]any{"model_id": id, "subject_type": "user", "subject_id": env.State.Values["admin_user_id"], "monthly_amount": 1000}, enterpriseHeaders(env, "p5-quota-"+protocol)); err != nil {
			return err
		}
	}
	_, err = client.JSON(ctx, "p5-compute-quota", "platform", http.MethodPut, "/platform/sandbox/enterprise-quotas/"+env.State.Values["enterprise_id"], 200, map[string]any{"max_concurrent_sessions": 4, "monthly_session_seconds": 86400, "expected_version": 0}, map[string]string{"Origin": env.PlatformOrigin(), "X-CSRF-Token": env.State.Values["platform_csrf"]})
	return err
}

func (a *App) verifyP5Responses(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	value, err := client.JSON(ctx, "p5-responses-conversation", "enterprise", http.MethodPost, "/conversations", 201, map[string]any{"title": "P5 Responses native tools", "selected_model_id": env.State.Values["p5_model_responses"]}, enterpriseHeaders(env, "p5-responses-conversation"))
	if err != nil {
		return err
	}
	id, err := stringField(value, "id")
	if err != nil {
		return err
	}
	run, err := a.p5Run(ctx, env, id, "responses-native", []p5ToolStep{
		{"tool.search", map[string]any{"category": "host"}},
		{"tool.describe", map[string]any{"category": "host", "name": "list"}},
		{"tool.invoke", map[string]any{"category": "host", "name": "list", "arguments": map[string]any{}}},
	}, []string{})
	if err != nil {
		return err
	}
	count, err := a.postgresQuery(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+run+"' AND status='succeeded';")
	if err != nil {
		return err
	}
	if strings.TrimSpace(count) != "3" {
		return fmt.Errorf("P5 Responses native tool pairing failed")
	}
	return nil
}
