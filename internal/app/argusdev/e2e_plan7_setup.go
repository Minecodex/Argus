package argusdev

import (
	"context"
	"fmt"
	"net/http"
)

func (a *App) preparePlan7ControlModel(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	if env.Options.RealModel == nil {
		return fmt.Errorf("Plan7 requires the selected model context budget")
	}
	value, err := client.JSON(ctx, "p7-control-model", "enterprise", http.MethodPost, "/enterprise/ai-models/test-and-create", 201,
		map[string]any{"name": "Plan7 deterministic control", "base_url": "https://argus-replay-model." + env.ReplayNamespace() + ".svc/v1", "model_id": "argus-replay-chat_completions", "api_protocol": "chat_completions", "api_key": "plan7-fixture-only", "context_window_tokens": env.Options.RealModel.ContextWindow, "max_output_tokens": env.Options.RealModel.MaxOutput, "input_price_per_million": 0.1, "output_price_per_million": 0.2}, enterpriseHeaders(env, "p7-control-model"))
	if err != nil {
		return err
	}
	if value["compatible"] != true {
		return fmt.Errorf("Plan7 control fixture compatibility failed")
	}
	id, err := stringField(value, "model", "id")
	if err != nil {
		return err
	}
	if _, err = client.JSON(ctx, "p7-control-quota", "enterprise", http.MethodPost, "/enterprise/model-quotas", 200, map[string]any{"model_id": id, "subject_type": "user", "subject_id": env.State.Values["admin_user_id"], "monthly_amount": 1000}, enterpriseHeaders(env, "p7-control-quota")); err != nil {
		return err
	}
	// M4 intentionally exhausts its own fixture quota. Plan7 owns a separate
	// deterministic fixture; the later real-provider phase uses GLM separately.
	env.State.Values["m4_model_id"] = id
	return nil
}
