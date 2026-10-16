package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"path/filepath"
	"time"
)

func (a *App) plan6HostAlertLoop(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) error {
	client, _ := scenarioHTTP(env)
	e, actor := env.State.Values["enterprise_id"], env.State.Values["admin_user_id"]
	sa, role := uuid.NewString(), uuid.NewString()
	query := fmt.Sprintf("INSERT INTO service_accounts(id,enterprise_id,name) VALUES('%s','%s','Docker Host evaluator'); INSERT INTO roles(id,enterprise_id,name) VALUES('%s','%s','Docker Host metric reader'); INSERT INTO role_permissions(role_id,permission_id) VALUES('%s','host.read'); INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES(gen_random_uuid(),'%s','service_account','%s','%s'); INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES(gen_random_uuid(),'%s','service_account','%s','host','%s');", sa, e, role, e, role, e, sa, role, e, sa, h.HostID)
	if _, err := a.postgresQuery(ctx, env, query); err != nil {
		return err
	}
	if _, err := a.plan6Config(ctx, env, "host-evaluator", map[string]any{"kind": "settings", "operation": "create", "name": "Docker Host evaluator", "spec": map[string]any{"evaluator_id": sa}}); err != nil {
		return err
	}
	endpoint := "argus-replay-model." + env.ReplayNamespace() + ".svc"
	for _, provider := range []string{"dingtalk", "wecom", "feishu", "smtp"} {
		value := map[string]any{"webhook_url": "https://" + endpoint + "/fixtures/notifications/" + provider, "signing_secret": "plan6-fixture-signing"}
		if provider == "smtp" {
			value = map[string]any{"smtp_host": endpoint, "smtp_port": 465, "security": "tls", "username": "plan6-fixture", "password": "plan6-fixture-password", "from": "alerts@example.test", "to": []string{"ops@example.test"}}
		}
		raw, _ := json.Marshal(value)
		secret, err := client.JSON(ctx, "host-notify-secret-"+provider, "enterprise", http.MethodPost, "/enterprise/secrets", 201, map[string]any{"name": "Docker Host " + provider, "type": "notification", "value": string(raw)}, enterpriseHeaders(env, "host-notify-secret-"+provider))
		if err != nil {
			return err
		}
		sid, _ := stringField(secret, "id")
		cred, err := client.JSON(ctx, "host-notify-cred-"+provider, "enterprise", http.MethodPost, "/enterprise/credentials", 201, map[string]any{"name": "Docker Host " + provider, "protocol": "notification", "secret_id": sid}, enterpriseHeaders(env, "host-notify-cred-"+provider))
		if err != nil {
			return err
		}
		cid, _ := stringField(cred, "id")
		row, err := a.plan6Config(ctx, env, "host-notify-channel-"+provider, map[string]any{"kind": "channel", "operation": "create", "name": "Docker Host " + provider, "spec": map[string]any{"enabled": true, "provider": provider, "credential_id": cid, "display_address": "Owned protocol receiver", "mention_ids": []string{"fixture-user"}, "mention_all": false, "template": ""}})
		if err != nil {
			return err
		}
		id, _ := stringField(row, "id")
		h.Channels = append(h.Channels, id)
	}
	if _, err := a.plan6Config(ctx, env, "host-grouping", map[string]any{"kind": "grouping", "operation": "create", "name": "Docker Host notification grouping", "spec": map[string]any{"group_by": []string{"policy_id"}, "wait_seconds": 1, "interval_seconds": 1, "repeat_seconds": 86400, "timing": []string{"first", "repeat", "severity", "recovery"}}}); err != nil {
		return err
	}
	rules := []map[string]any{}
	for _, severity := range []string{"critical", "warning"} {
		threshold := h.TriggerCPU
		if severity == "warning" {
			threshold -= 0.5
		}
		rules = append(rules, map[string]any{"name": "Actual CPU " + severity, "expression": h.CPUExpression, "query_mode": "numeric", "operator": ">", "threshold": threshold, "recovery_threshold": threshold - 0.5, "severity": severity, "unit": "%"})
	}
	spec := map[string]any{"description": "Real Docker system CPU metrics via Bastion; bounded load scenario", "enabled": true, "resources": map[string]any{"mode": "explicit", "types": []string{"host"}, "ids": []string{h.HostID}, "matchers": []any{}, "source_ids": []string{h.SourceID}}, "rules": rules, "mode": "independent", "match_by": []string{"argus_resource_id", "argus_source_id"}, "interval_seconds": 10, "for_seconds": 20, "recovery_seconds": 20, "no_data_behavior": "report", "error_behavior": "keep", "channel_ids": h.Channels, "route_ids": []string{}, "labels": map[string]string{"suite": "docker-host-loop"}}
	trial, err := client.JSON(ctx, "host-alert-trial", "enterprise", http.MethodPost, "/alerts/trial", 200, spec, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	if trial["health"] != "normal" {
		return fmt.Errorf("real Host alert trial not complete")
	}
	row, err := a.plan6Config(ctx, env, "host-cpu-policy", map[string]any{"kind": "policy", "operation": "create", "name": "Plan6 Docker Host CPU pressure", "spec": spec})
	if err != nil {
		return err
	}
	h.PolicyID, _ = stringField(row, "id")
	if _, err = a.plan6DockerScript(ctx, h.Docker, h.Docker.Host, "systemd-run --unit=argus-hostloop-load --property=CPUQuota=200% /bin/bash -c 'while :; do :; done & while :; do :; done & wait'\n"); err != nil {
		return err
	}
	pending, err := a.plan6HostWaitInstances(ctx, env, h, "pending")
	if err != nil {
		return err
	}
	pendingRaw, _ := json.MarshalIndent(pending, "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "host-continuous-pending.json"), pendingRaw); err != nil {
		return err
	}
	current, err := a.plan6HostWaitInstances(ctx, env, h, "firing")
	if err != nil {
		return err
	}
	observed, err := a.plan6HostMetric(ctx, env, h, "cpu-loaded", h.CPUExpression)
	if err != nil {
		return err
	}
	if observed <= h.TriggerCPU {
		return fmt.Errorf("actual CPU load did not cross the recorded threshold")
	}
	for _, item := range objectItems(current) {
		state, _ := item["state"].(map[string]any)
		if state["kind"] != "condition" {
			continue
		}
		id, _ := stringField(item, "id")
		h.ConditionIDs = append(h.ConditionIDs, id)
	}
	if err = a.plan6HostWaitNotifications(ctx, env, h, "first"); err != nil {
		return err
	}
	if err = a.verifyPlan6Receipts(ctx, env, h.PolicyID, "first"); err != nil {
		return err
	}
	for _, first := range objectItems(current) {
		id, _ := stringField(first, "id")
		for _, op := range []string{"acknowledge", "assign", "comment"} {
			item, err := client.JSON(ctx, "host-handle-read-"+op, "enterprise", http.MethodGet, "/alerts/instances/"+id, 200, nil, enterpriseHeaders(env, ""))
			if err != nil {
				return err
			}
			instance, _ := item["instance"].(map[string]any)
			input := map[string]any{"operation": op, "expected_handling_version": instance["handling_version"]}
			if op == "assign" {
				input["owner_id"] = actor
			}
			if op == "comment" {
				input["comment"] = "Real Docker Host alert loop verified"
			}
			if _, err = client.JSON(ctx, "host-handle-"+id+"-"+op, "enterprise", http.MethodPost, "/alerts/instances/"+id+"/handle", 200, input, enterpriseHeaders(env, "host-handle-"+id+"-"+op)); err != nil {
				return err
			}
		}
	}
	silence, err := a.plan6Config(ctx, env, "host-silence", map[string]any{"kind": "silence", "operation": "create", "name": "Docker Host maintenance", "spec": map[string]any{"enabled": true, "resource_ids": []string{h.HostID}, "policy_ids": []string{h.PolicyID}, "metric": "", "matchers": []any{}, "starts_at": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), "ends_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), "reason": "Real Docker maintenance scenario"}})
	if err != nil {
		return err
	}
	if err = a.plan6HostWaitNoise(ctx, env, h, "muted"); err != nil {
		return err
	}
	if err = a.plan6HostDeleteConfig(ctx, env, "host-unsilence", "silence", silence); err != nil {
		return err
	}
	if err = a.plan6HostWaitNoiseCleared(ctx, env, h); err != nil {
		return err
	}
	inhibit, err := a.plan6Config(ctx, env, "host-inhibition", map[string]any{"kind": "inhibition", "operation": "create", "name": "Docker critical suppresses warning", "spec": map[string]any{"enabled": true, "resource_ids": []string{h.HostID}, "source": []map[string]string{{"key": "severity", "operator": "=", "value": "critical"}}, "target": []map[string]string{{"key": "severity", "operator": "=", "value": "warning"}}, "equal": []string{"resource_id", "policy_id"}, "reason": "Real Docker suppression scenario"}})
	if err != nil {
		return err
	}
	if err = a.plan6HostWaitNoise(ctx, env, h, "inhibited"); err != nil {
		return err
	}
	if err = a.plan6HostDeleteConfig(ctx, env, "host-uninhibit", "inhibition", inhibit); err != nil {
		return err
	}
	return a.plan6HostWaitNoiseCleared(ctx, env, h)
}

func (a *App) plan6HostWaitNoiseCleared(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) error {
	_, err := a.plan6Wait(ctx, env, "docker-host-noise-cleared", "/alerts/instances?policy_id="+h.PolicyID, func(v map[string]any) bool {
		items := objectItems(v)
		if len(items) != 2 {
			return false
		}
		for _, x := range items {
			s, _ := x["state"].(map[string]any)
			if x["lifecycle"] != "firing" || s["notification_state"] == "muted" || s["notification_state"] == "inhibited" {
				return false
			}
		}
		return true
	})
	return err
}

func (a *App) plan6HostWaitInstances(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop, life string) (map[string]any, error) {
	return a.plan6Wait(ctx, env, "docker-host-"+life, "/alerts/instances?policy_id="+h.PolicyID, func(v map[string]any) bool {
		n := 0
		for _, x := range objectItems(v) {
			s, _ := x["state"].(map[string]any)
			if x["lifecycle"] == life && s["kind"] == "condition" {
				n++
			}
		}
		return n == 2
	})
}
func (a *App) plan6HostWaitNoise(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop, state string) error {
	_, err := a.plan6Wait(ctx, env, "docker-host-"+state, "/alerts/instances?policy_id="+h.PolicyID, func(v map[string]any) bool {
		count := 0
		for _, x := range objectItems(v) {
			s, _ := x["state"].(map[string]any)
			if x["lifecycle"] == "firing" && s["notification_state"] == state {
				count++
			}
		}
		if state == "muted" {
			return count == 2
		}
		return count == 1
	})
	return err
}
func (a *App) plan6HostRecovery(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) error {
	if _, err := a.plan6DockerScript(ctx, h.Docker, h.Docker.Host, "systemctl stop argus-hostloop-load\n"); err != nil {
		return err
	}
	recovering, err := a.plan6HostWaitInstances(ctx, env, h, "recovering")
	if err != nil {
		return err
	}
	recoveringRaw, _ := json.MarshalIndent(recovering, "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "host-recovery-intermediate.json"), recoveringRaw); err != nil {
		return err
	}
	history, err := a.plan6Wait(ctx, env, "host-recovery-history", "/alerts/history?policy_id="+h.PolicyID, func(v map[string]any) bool {
		n := 0
		for _, x := range objectItems(v) {
			s, _ := x["state"].(map[string]any)
			if x["lifecycle"] == "resolved" && s["kind"] == "condition" {
				n++
			}
		}
		return n == 2
	})
	if err != nil {
		return err
	}
	if _, err = a.plan6HostMetric(ctx, env, h, "cpu-recovered", h.CPUExpression); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(history, "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "host-recovery-history.json"), raw); err != nil {
		return err
	}
	if err = a.plan6HostVerifyHistory(ctx, env, h); err != nil {
		return err
	}
	if err = a.plan6HostWaitNotifications(ctx, env, h, "recovery"); err != nil {
		return err
	}
	if err = a.verifyPlan6Receipts(ctx, env, h.PolicyID, "first", "recovery"); err != nil {
		return err
	}
	client, _ := scenarioHTTP(env)
	_, err = client.JSON(ctx, "host-history-export", "enterprise", http.MethodGet, "/alerts/history/export?policy_id="+h.PolicyID, 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	return a.plan6HostModelRead(ctx, env, h, "recovered")
}

func (a *App) plan6HostDeleteConfig(ctx context.Context, env *E2EEnvironment, key, kind string, row map[string]any) error {
	client, _ := scenarioHTTP(env)
	p, err := client.JSON(ctx, key+"-preview", "enterprise", http.MethodPost, "/alerts/configurations/preview", 202, map[string]any{"kind": kind, "operation": "delete", "id": row["id"], "expected_version": row["version"]}, enterpriseHeaders(env, key))
	if err != nil {
		return err
	}
	ref, _ := stringField(p, "action_ref")
	_, err = a.confirmPendingAction(ctx, env, key, ref)
	return err
}
