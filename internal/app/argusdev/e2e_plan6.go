package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a *App) plan6Config(ctx context.Context, env *E2EEnvironment, key string, input map[string]any) (map[string]any, error) {
	client, _ := scenarioHTTP(env)
	preview, err := client.JSON(ctx, "p6-preview-"+key, "enterprise", http.MethodPost, "/alerts/configurations/preview", 202, input, enterpriseHeaders(env, "p6-preview-"+key))
	if err != nil {
		return nil, err
	}
	ref, err := stringField(preview, "action_ref")
	if err != nil {
		return nil, err
	}
	if _, err = a.confirmPendingAction(ctx, env, "p6-confirm-"+key, ref); err != nil {
		return nil, err
	}
	items, err := client.JSONArray(ctx, "p6-config-"+key, "enterprise", http.MethodGet, "/alerts/configurations?kind="+fmt.Sprint(input["kind"]), 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item["name"] == input["name"] || item["id"] == input["id"] {
			return item, nil
		}
	}
	return nil, fmt.Errorf("Plan6 committed configuration missing")
}
func (a *App) plan6Wait(ctx context.Context, env *E2EEnvironment, name, path string, predicate func(map[string]any) bool) (map[string]any, error) {
	client, _ := scenarioHTTP(env)
	deadline := time.NewTimer(150 * time.Second)
	defer deadline.Stop()
	for attempt := 0; ; attempt++ {
		value, err := client.JSON(ctx, fmt.Sprintf("p6-%s-%d", name, attempt), "enterprise", http.MethodGet, path, 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return nil, err
		}
		if predicate(value) {
			return value, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return value, fmt.Errorf("Plan6 %s did not converge", name)
		case <-time.After(time.Second):
		}
	}
}
func (a *App) runPlan6Scenario(ctx context.Context, env *E2EEnvironment) error {
	client, _ := scenarioHTTP(env)
	if env.Options.RealModel == nil && (env.Options.Plan6Phase == "all" || env.Options.Plan6Phase == "model" || env.Options.Plan6Phase == "") {
		return fmt.Errorf("Plan6 requires the selected real model configuration")
	}
	if env.State.Values["p6_selfmonitor_ready"] != "true" || env.State.Values["p6_backend_build"] != "production" {
		return fmt.Errorf("Plan6 business acceptance requires completed self-monitoring and the normal backend")
	}
	enterprise, actor, cluster := env.State.Values["enterprise_id"], env.State.Values["admin_user_id"], env.State.Values["m3_cluster_id"]
	for _, id := range []string{enterprise, actor, cluster} {
		if uuid.Validate(id) != nil {
			return fmt.Errorf("Plan6 prerequisite identity missing")
		}
	}
	sa, role := uuid.NewString(), uuid.NewString()
	sql := fmt.Sprintf(`INSERT INTO service_accounts(id,enterprise_id,name) VALUES('%s','%s','Plan6 evaluator');INSERT INTO roles(id,enterprise_id,name) VALUES('%s','%s','Plan6 metric reader');INSERT INTO role_permissions(role_id,permission_id) VALUES('%s','host.read'),('%s','kubernetes.read');INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES(gen_random_uuid(),'%s','service_account','%s','%s');INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES(gen_random_uuid(),'%s','service_account','%s','kubernetes_cluster','%s');`, sa, enterprise, role, enterprise, role, role, enterprise, sa, role, enterprise, sa, cluster)
	if _, err := a.postgresQuery(ctx, env, sql); err != nil {
		return err
	}
	env.State.Values["p6_evaluator_id"] = sa
	if _, err := a.plan6Config(ctx, env, "evaluator", map[string]any{"kind": "settings", "operation": "create", "name": "Plan6 evaluator", "spec": map[string]any{"evaluator_id": sa}}); err != nil {
		return err
	}
	host := "argus-replay-model." + env.ReplayNamespace() + ".svc"
	channels := []string{}
	for _, provider := range []string{"dingtalk", "wecom", "feishu", "smtp"} {
		credential := map[string]any{"webhook_url": "https://" + host + "/fixtures/notifications/" + provider, "signing_secret": "plan6-fixture-signing"}
		if provider == "smtp" {
			credential = map[string]any{"smtp_host": host, "smtp_port": 465, "security": "tls", "username": "plan6-fixture", "password": "plan6-fixture-password", "from": "alerts@example.test", "to": []string{"ops@example.test"}}
		}
		encoded, _ := json.Marshal(credential)
		secret, err := client.JSON(ctx, "p6-secret-"+provider, "enterprise", http.MethodPost, "/enterprise/secrets", 201, map[string]any{"name": "Plan6 " + provider, "type": "notification", "value": string(encoded)}, enterpriseHeaders(env, "p6-secret-"+provider))
		if err != nil {
			return err
		}
		secretID, _ := stringField(secret, "id")
		key, err := client.JSON(ctx, "p6-credential-"+provider, "enterprise", http.MethodPost, "/enterprise/credentials", 201, map[string]any{"name": "Plan6 " + provider, "protocol": "notification", "secret_id": secretID}, enterpriseHeaders(env, "p6-credential-"+provider))
		if err != nil {
			return err
		}
		credentialID, _ := stringField(key, "id")
		channel, err := a.plan6Config(ctx, env, provider, map[string]any{"kind": "channel", "operation": "create", "name": "Plan6 " + provider, "spec": map[string]any{"enabled": true, "default_for_new_policies": provider == "feishu", "provider": provider, "credential_id": credentialID, "display_address": "Plan6 protocol receiver", "mention_ids": []string{"fixture-user"}, "mention_all": false, "template": ""}})
		if err != nil {
			return err
		}
		id, _ := stringField(channel, "id")
		channels = append(channels, id)
	}
	if _, err := a.plan6Config(ctx, env, "grouping", map[string]any{"kind": "grouping", "operation": "create", "name": "Plan6 grouping", "spec": map[string]any{"group_by": []string{"policy_id"}, "wait_seconds": 1, "interval_seconds": 1, "repeat_seconds": 86400, "timing": []string{"first", "repeat", "severity", "recovery"}}}); err != nil {
		return err
	}
	policy := map[string]any{"description": "Real OTLP ingestion and rule evaluation", "enabled": true, "resources": map[string]any{"mode": "explicit", "types": []string{"kubernetes_cluster"}, "ids": []string{cluster}, "matchers": []any{}, "source_ids": []string{}}, "rules": []map[string]any{{"name": "CPU", "expression": "argus_plan6_cpu_percent", "query_mode": "numeric", "operator": ">", "threshold": 85, "recovery_threshold": 75, "severity": "critical", "unit": "%"}, {"name": "Memory", "expression": "argus_plan6_memory_percent", "query_mode": "numeric", "operator": ">", "threshold": 80, "recovery_threshold": 70, "severity": "warning", "unit": "%"}}, "mode": "independent", "match_by": []string{"argus_resource_id", "argus_source_id"}, "interval_seconds": 10, "for_seconds": 0, "recovery_seconds": 0, "no_data_behavior": "report", "error_behavior": "report", "channel_ids": channels, "route_ids": []string{}, "labels": map[string]string{"env": "e2e"}}
	emit := func(value string) error {
		_, err := env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-server", "plan6-telemetry-fixture", "/usr/local/bin/argus-telemetry-e2e", "--endpoint=127.0.0.1:4317", "--marker=plan6", "--resource-id="+cluster, "--plan6-alert-value="+value)
		return err
	}
	if err := emit("95"); err != nil {
		return err
	}
	deadline := time.Now().Add(90 * time.Second)
	for attempt := 0; ; attempt++ {
		trial, err := client.JSON(ctx, fmt.Sprintf("p6-trial-%d", attempt), "enterprise", http.MethodPost, "/alerts/trial", 200, policy, enterpriseHeaders(env, ""))
		if err != nil {
			// Collector rollout can briefly interrupt the ingress upstream. The
			// request has no side effect and every failed response stays in evidence.
			if !strings.Contains(err.Error(), "got 502") && !strings.Contains(err.Error(), "got 503") || time.Now().After(deadline) {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				continue
			}
		}
		rawTrial, _ := json.MarshalIndent(trial, "", "  ")
		if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan6-trial-latest.json"), rawTrial); err != nil {
			return err
		}
		if trial["health"] == "normal" {
			break
		}
		// The cluster has other exporters that do not expose fixture metrics.
		// Explicitly select the real OTLP source, then require a complete trial.
		if observations, ok := trial["observations"].([]any); ok && len(observations) == 2 {
			selected := map[string]bool{}
			for _, item := range observations {
				row, _ := item.(map[string]any)
				for _, source := range row["source_ids"].([]any) {
					if id, ok := source.(string); ok && uuid.Validate(id) == nil {
						selected[id] = true
					}
				}
			}
			ids := []string{}
			for id := range selected {
				ids = append(ids, id)
			}
			policy["resources"].(map[string]any)["source_ids"] = ids
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("real metric observation unavailable")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	created, err := a.plan6Config(ctx, env, "policy", map[string]any{"kind": "policy", "operation": "create", "name": "Plan6 real pressure", "spec": policy})
	if err != nil {
		return err
	}
	policyID, _ := stringField(created, "id")
	env.State.Values["p6_policy_id"] = policyID
	current, err := a.plan6Wait(ctx, env, "firing", "/alerts/instances?policy_id="+policyID, func(v map[string]any) bool {
		rows := objectItems(v)
		firing := 0
		for _, row := range rows {
			if row["lifecycle"] == "firing" && row["health"] == "normal" {
				firing++
			}
		}
		return firing == 2
	})
	if err != nil {
		return err
	}
	if _, err = a.plan6Wait(ctx, env, "accepted", "/alerts/deliveries?policy_id="+policyID, func(v map[string]any) bool {
		n := 0
		for _, row := range objectItems(v) {
			if row["status"] == "accepted" {
				n++
			}
		}
		return n >= 8
	}); err != nil {
		return err
	}
	if err = a.verifyPlan6Receipts(ctx, env, policyID, "first"); err != nil {
		return err
	}
	if err = a.verifyPlan6RuntimeFaults(ctx, env, policyID); err != nil {
		return err
	}
	if err = a.verifyPlan6EvaluatorAuthorization(ctx, env, policyID); err != nil {
		return err
	}
	first := objectItems(current)[0]
	instanceID, _ := stringField(first, "id")
	version, _ := numberField(first, "handling_version")
	if _, err = client.JSON(ctx, "p6-acknowledge", "enterprise", http.MethodPost, "/alerts/instances/"+instanceID+"/handle", 200, map[string]any{"operation": "acknowledge", "expected_handling_version": version}, enterpriseHeaders(env, "p6-acknowledge")); err != nil {
		return err
	}
	silence, err := a.plan6Config(ctx, env, "silence", map[string]any{"kind": "silence", "operation": "create", "name": "Plan6 deployment", "spec": map[string]any{"enabled": true, "resource_ids": []string{cluster}, "policy_ids": []string{policyID}, "metric": "argus_plan6_cpu_percent", "matchers": []any{}, "starts_at": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), "ends_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), "reason": "Plan6 fixture deployment"}})
	if err != nil {
		return err
	}
	if _, err = a.plan6Wait(ctx, env, "muted", "/alerts/instances?policy_id="+policyID, func(v map[string]any) bool {
		for _, row := range objectItems(v) {
			state, _ := row["state"].(map[string]any)
			if state["notification_state"] == "muted" && row["lifecycle"] == "firing" {
				return true
			}
		}
		return false
	}); err != nil {
		return err
	}
	silenceID, _ := stringField(silence, "id")
	silenceVersion, _ := numberField(silence, "version")
	if _, err = a.plan6Config(ctx, env, "unsilence", map[string]any{"kind": "silence", "operation": "disable", "id": silenceID, "expected_version": silenceVersion, "name": "Plan6 deployment"}); err != nil {
		return err
	}
	if err = emit("60"); err != nil {
		return err
	}
	if _, err = a.plan6Wait(ctx, env, "history", "/alerts/history?policy_id="+policyID, func(v map[string]any) bool {
		n := 0
		for _, row := range objectItems(v) {
			state, _ := row["state"].(map[string]any)
			if state["kind"] == "condition" && row["lifecycle"] == "resolved" {
				n++
			}
		}
		return n == 2
	}); err != nil {
		return err
	}
	if _, err = a.plan6Wait(ctx, env, "recovery-deliveries", "/alerts/deliveries?policy_id="+policyID, func(v map[string]any) bool {
		n := 0
		for _, row := range objectItems(v) {
			m, _ := row["message"].(map[string]any)
			if row["status"] == "accepted" && m["event_type"] == "recovery" {
				n++
			}
		}
		return n >= 8
	}); err != nil {
		return err
	}
	if _, err = client.JSON(ctx, "p6-history-export", "enterprise", http.MethodGet, "/alerts/history/export?policy_id="+policyID, 200, nil, enterpriseHeaders(env, "")); err != nil {
		return err
	}
	if err = a.verifyPlan6Receipts(ctx, env, policyID); err != nil {
		return err
	}
	if err = a.verifyPlan6DynamicAdmission(ctx, env); err != nil {
		return err
	}
	if err = a.verifyPlan6StalePreviews(ctx, env, policy); err != nil {
		return err
	}
	if env.Options.Plan6Phase == "all" || env.Options.Plan6Phase == "runtime" || env.Options.Plan6Phase == "" {
		if err = a.verifyPlan6EvaluationMatrix(ctx, env, policy, emit); err != nil {
			return err
		}
		if err = a.verifyPlan6LifecycleEdges(ctx, env, policy, emit); err != nil {
			return err
		}
		if err = a.verifyPlan6NotificationFaults(ctx, env); err != nil {
			return err
		}
	}
	if env.Options.Plan6Phase == "all" || env.Options.Plan6Phase == "model" || env.Options.Plan6Phase == "" {
		if err = a.runPlan6RealModel(ctx, env); err != nil {
			return err
		}
	}
	if env.Options.Plan6Phase == "all" || env.Options.Plan6Phase == "browser" || env.Options.Plan6Phase == "" {
		controlURL, controlToken, stopControl, e := startPlan6BrowserSignals(ctx, emit, func(c context.Context, kind, ref string) error { return a.plan6BrowserFault(c, env, kind, ref) })
		if e != nil {
			return e
		}
		defer stopControl()
		if err = a.runPlaywright(ctx, env, "e2e/plan6-real.spec.ts", map[string]string{"ARGUS_PLAN6_E2E": "1", "ARGUS_PLAN6_PHASE": env.Options.Plan6Phase, "ARGUS_PLAN6_UNKNOWN_DELIVERY": env.State.Values["p6_unknown_delivery_id"], "ARGUS_PLAN6_USERNAME": env.State.Values["enterprise_username"], "ARGUS_PLAN6_PASSWORD": env.State.Values["enterprise_password"], "ARGUS_PLAN6_POLICY_ID": policyID, "ARGUS_PLAN6_RESOURCE_ID": cluster, "ARGUS_PLAN6_SIGNAL_CONTROL_URL": controlURL, "ARGUS_PLAN6_SIGNAL_CONTROL_TOKEN": controlToken, "ARGUS_PLAN6_MODEL_ID": env.State.Values["p6_model_id"], "ARGUS_PLAN6_CONVERSATION_QUERY": env.State.Values["p6_conversation_query"], "ARGUS_PLAN6_CONVERSATION_CANCEL": env.State.Values["p6_conversation_cancel"]}); err != nil {
			return err
		}
	}
	facts, err := a.postgresQuery(ctx, env, fmt.Sprintf("SELECT json_build_object('configurations',(SELECT count(*) FROM alert_configurations WHERE enterprise_id='%s'),'evaluations',(SELECT count(*) FROM alert_evaluations WHERE enterprise_id='%s'),'events',(SELECT count(*) FROM alert_events WHERE enterprise_id='%s'),'deliveries',(SELECT count(*) FROM alert_deliveries WHERE enterprise_id='%s'),'attempts',(SELECT count(*) FROM alert_delivery_attempts WHERE enterprise_id='%s'),'fenced_leases',(SELECT count(*) FROM credential_leases WHERE enterprise_id='%s' AND recipient_type='notification_worker'));", enterprise, enterprise, enterprise, enterprise, enterprise, enterprise))
	if err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan6-database-facts.json"), []byte(strings.TrimSpace(facts)+"\n")); err != nil {
		return err
	}
	result := map[string]any{"real_ingestion": true, "real_query_engine": true, "notification_protocols": []string{"dingtalk", "wecom", "feishu", "smtp"}, "external_contact_points": false, "policy_id": policyID}
	raw, _ := json.MarshalIndent(result, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan6-runtime.json"), raw)
}

var _ = strconv.Itoa
