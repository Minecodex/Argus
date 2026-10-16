package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (a *App) verifyPlan6EvaluatorAuthorization(ctx context.Context, env *E2EEnvironment, policyID string) (failure error) {
	sa, enterprise, cluster := env.State.Values["p6_evaluator_id"], env.State.Values["enterprise_id"], env.State.Values["m3_cluster_id"]
	for _, id := range []string{sa, enterprise, cluster, policyID} {
		if uuid.Validate(id) != nil {
			return fmt.Errorf("invalid Plan6 authorization fixture identity")
		}
	}
	grant, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT id FROM data_authorization_grants WHERE enterprise_id='%s' AND subject_type='service_account' AND subject_id='%s' AND resource_type='kubernetes_cluster' AND resource_id='%s' LIMIT 1;`, enterprise, sa, cluster))
	if err != nil {
		return err
	}
	grant = strings.TrimSpace(grant)
	if uuid.Validate(grant) != nil {
		return fmt.Errorf("Plan6 explicit evaluator grant missing")
	}
	restore := func(c context.Context) error {
		_, err := a.postgresQuery(c, env, fmt.Sprintf(`INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES('%s','%s','service_account','%s','kubernetes_cluster','%s') ON CONFLICT(id) DO NOTHING; UPDATE service_accounts SET status='active',authorization_version=authorization_version+1 WHERE enterprise_id='%s' AND id='%s';`, grant, enterprise, sa, cluster, enterprise, sa))
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		failure = errors.Join(failure, restore(cleanup))
	}()
	conditionFacts := func(v map[string]any, healthy bool) bool {
		n := 0
		for _, row := range objectItems(v) {
			state, _ := row["state"].(map[string]any)
			if state["kind"] == "condition" && row["lifecycle"] == "firing" && (row["health"] == "normal") == healthy {
				n++
			}
		}
		return n == 2
	}
	if _, err = a.postgresQuery(ctx, env, fmt.Sprintf(`DELETE FROM data_authorization_grants WHERE id='%s' AND enterprise_id='%s'; UPDATE service_accounts SET authorization_version=authorization_version+1 WHERE id='%s' AND enterprise_id='%s';`, grant, enterprise, sa, enterprise)); err != nil {
		return err
	}
	revoked, err := a.plan6Wait(ctx, env, "evaluator-revoked", "/alerts/instances?policy_id="+policyID, func(v map[string]any) bool { return conditionFacts(v, false) })
	if err != nil {
		return err
	}
	if err = restore(ctx); err != nil {
		return err
	}
	if _, err = a.plan6Wait(ctx, env, "evaluator-reauthorized", "/alerts/instances?policy_id="+policyID, func(v map[string]any) bool { return conditionFacts(v, true) }); err != nil {
		return err
	}
	if _, err = a.postgresQuery(ctx, env, fmt.Sprintf(`UPDATE service_accounts SET status='disabled',authorization_version=authorization_version+1 WHERE enterprise_id='%s' AND id='%s';`, enterprise, sa)); err != nil {
		return err
	}
	stopped, err := a.plan6Wait(ctx, env, "evaluator-stopped", "/alerts/instances?policy_id="+policyID, func(v map[string]any) bool { return conditionFacts(v, false) })
	if err != nil {
		return err
	}
	client, _ := scenarioHTTP(env)
	if _, err = client.JSONArray(ctx, "p6-settings-readable-when-stopped", "enterprise", "GET", "/alerts/configurations?kind=settings", 200, nil, enterpriseHeaders(env, "")); err != nil {
		return err
	}
	if _, err = client.JSONArray(ctx, "p6-policy-readable-when-stopped", "enterprise", "GET", "/alerts/configurations?kind=policy", 200, nil, enterpriseHeaders(env, "")); err != nil {
		return err
	}
	if err = restore(ctx); err != nil {
		return err
	}
	if _, err = a.plan6Wait(ctx, env, "evaluator-restarted", "/alerts/instances?policy_id="+policyID, func(v map[string]any) bool { return conditionFacts(v, true) }); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(map[string]any{"passed": true, "explicit_grant_revocation": true, "account_stop_blocks_evaluation": true, "configurations_remain_authorized_readable": true, "revoked": revoked, "stopped": stopped}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan6-evaluator-authorization.json"), raw)
}
