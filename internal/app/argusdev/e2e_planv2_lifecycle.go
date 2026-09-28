package argusdev

import (
	"context"
	"net/http"
)

// Own a disposable Argus resource record. No Collector is installed in this
// cluster, and its deletion cannot alter the telemetry fixture used elsewhere.
func (a *App) preparePlanV2Lifecycle(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	preview, err := client.JSON(ctx, "p2-disposable-cluster", "enterprise", http.MethodPost, "/enterprise/kubernetes-clusters/actions/preview-create", 201, map[string]any{"name": "PlanV2 disposable shortcut", "api_server": "https://kubernetes.default.svc", "connection_mode": "in_cluster", "default_namespace": "default", "environment": "development", "labels": map[string]string{"test": "planv2-lifecycle"}}, enterpriseHeaders(env, "p2-disposable-cluster"))
	if err != nil {
		return err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return err
	}
	result, err := a.confirmPendingAction(ctx, env, "p2-disposable-confirm", ref)
	if err != nil {
		return err
	}
	id, err := stringField(result, "resource_ref", "resource_id")
	if err != nil {
		return err
	}
	env.State.Values["p2_disposable_cluster_id"] = id
	return a.refreshEnterpriseLogin(ctx, env)
}
