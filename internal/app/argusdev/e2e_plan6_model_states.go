package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) verifyPlan6ModelStates(ctx context.Context, env *E2EEnvironment, modelID string, report map[string]any) (failure error) {
	client, _ := scenarioHTTP(env)
	base, err := client.JSON(ctx, "p6-model-state-base", "enterprise", http.MethodGet, "/alerts/configurations/"+env.State.Values["p6_policy_id"], 200, nil, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	stopTelemetry, err := a.startPlan6ModelTelemetry(ctx, env)
	if err != nil {
		return err
	}
	defer func() { failure = errors.Join(failure, stopTelemetry()) }()
	rows := []map[string]any{}
	report["read_state_scenarios"] = rows
	for _, state := range []string{"normal", "no_data", "error", "muted"} {
		name := map[string]string{"normal": "Plan6 正常无事件", "no_data": "Plan6 缺失数据", "error": "Plan6 查询失败", "muted": "Plan6 已屏蔽"}[state]
		raw, _ := json.Marshal(base["spec"])
		spec := map[string]any{}
		_ = json.Unmarshal(raw, &spec)
		spec["channel_ids"], spec["route_ids"], spec["no_data_behavior"], spec["error_behavior"] = []string{}, []string{}, "report", "report"
		spec["enabled"] = true
		rules := spec["rules"].([]any)
		switch state {
		case "normal":
			for _, r := range rules {
				r.(map[string]any)["threshold"], r.(map[string]any)["recovery_threshold"] = 200, 175
			}
		case "no_data":
			for i, r := range rules {
				r.(map[string]any)["expression"] = fmt.Sprintf("argus_plan6_absent_%d", i)
			}
		case "error":
			spec["mode"] = "every"
			rules[0].(map[string]any)["expression"] = "argus_plan6_cpu_percent / 0"
		}
		policy, err := a.plan6Config(ctx, env, "model-state-"+state, map[string]any{"kind": "policy", "operation": "create", "name": name, "spec": spec})
		if err != nil {
			return err
		}
		id, _ := stringField(policy, "id")
		if state == "muted" {
			_, err = a.plan6Config(ctx, env, "model-state-silence", map[string]any{"kind": "silence", "operation": "create", "name": "Plan6 查询屏蔽事实", "spec": map[string]any{"enabled": true, "resource_ids": []string{env.State.Values["m3_cluster_id"]}, "policy_ids": []string{id}, "metric": "", "matchers": []any{}, "starts_at": time.Now().UTC().Add(-time.Minute), "ends_at": time.Now().UTC().Add(time.Hour), "reason": "模型只读状态验收"}})
			if err != nil {
				return err
			}
		}
		wanted := state
		if state == "muted" {
			wanted = "normal"
		}
		baseline, err := a.plan6Wait(ctx, env, "model-state-ready-"+state, "/alerts/instances?policy_id="+id, func(v map[string]any) bool {
			health := false
			evaluations, _ := v["evaluations"].([]any)
			for _, r := range evaluations {
				row := r.(map[string]any)
				health = health || row["policy_id"] == id && row["health"] == wanted && row["last_evaluated_at"] != nil
			}
			if state != "muted" {
				return health
			}
			for _, row := range objectItems(v) {
				s, _ := row["state"].(map[string]any)
				if row["policy_id"] == id && s["notification_state"] == "muted" {
					return health
				}
			}
			return false
		})
		if err != nil {
			return err
		}
		before, err := a.plan6ConfigurationFacts(ctx, env)
		if err != nil {
			return err
		}
		conversation, err := client.JSON(ctx, "p6-state-conversation-"+state, "enterprise", http.MethodPost, "/conversations", 201, map[string]any{"title": name + "只读验收", "selected_model_id": modelID}, enterpriseHeaders(env, "p6-state-conversation-"+state))
		if err != nil {
			return err
		}
		conversationID, _ := stringField(conversation, "id")
		report["failed_task"] = "read-state-" + state
		sample, err := a.realModelTask(ctx, env, conversationID, "p6-read-state-"+state, "请查询“"+name+"”的当前告警和评估状态，只读，不修改配置。说明资源范围、最近评估、数据完整性，并区分正常无事件、无数据、查询失败和已屏蔽；零事件本身不能作为健康依据。", nil, nil)
		if err != nil {
			return err
		}
		evidence, err := a.postgresQuery(ctx, env, fmt.Sprintf("SELECT COALESCE(json_agg(json_build_object('tool',c.tool_id,'input',c.input,'status',c.status,'sequence',c.call_sequence,'created_at',c.created_at,'error_code',c.error_code,'output',r.projection::jsonb->'summary') ORDER BY c.created_at,c.call_sequence),'[]') FROM tool_calls c LEFT JOIN tool_results r ON r.tool_call_id=c.id WHERE c.run_id='%s';", sample.RunID))
		if err != nil {
			return err
		}
		if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan6-tools-read-state-"+state+".json"), []byte(strings.TrimSpace(evidence))); err != nil {
			return err
		}
		sampleData, _ := json.MarshalIndent(sample, "", "  ")
		if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan6-model-sample-"+state+".json"), sampleData); err != nil {
			return err
		}
		if err = verifyPlan6DiscoveryOrder(evidence); err != nil {
			return err
		}
		after, err := a.plan6ConfigurationFacts(ctx, env)
		if err != nil || before != after {
			return fmt.Errorf("read-only state %s changed configuration: %v", state, err)
		}
		count, err := a.postgresQuery(ctx, env, fmt.Sprintf("SELECT count(*) FROM pending_actions WHERE run_id='%s';", sample.RunID))
		if err != nil || strings.TrimSpace(count) != "0" {
			return fmt.Errorf("read-only state %s proposed a mutation: %v", state, err)
		}
		if err = verifyPlan6StateProjection(evidence, id, wanted, state == "muted", state == "normal", env.State.Values["m3_cluster_id"]); err != nil {
			return err
		}
		if sample.RunStatus != "succeeded" || !sample.UsageComplete {
			return fmt.Errorf("read state %s did not finish with complete usage", state)
		}
		sample.Passed = true
		row := map[string]any{"state": state, "policy_id": id, "sample": sample, "database_baseline": baseline, "tool_calls": json.RawMessage(strings.TrimSpace(evidence)), "no_business_mutation": true, "passed": true}
		rows = append(rows, row)
		report["read_state_scenarios"] = rows
		data, _ := json.MarshalIndent(row, "", "  ")
		if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan6-model-state-"+state+".json"), data); err != nil {
			return err
		}
		delete(report, "failed_task")
	}
	return nil
}

func verifyPlan6StateProjection(raw, policy, health string, muted, empty bool, resource string) error {
	var calls []struct {
		Tool, Status string
		Output       map[string]any
	}
	if err := json.Unmarshal([]byte(raw), &calls); err != nil {
		return err
	}
	observedHealth, observedMute, observedEmpty := false, false, false
	for _, c := range calls {
		if c.Tool != "tool.invoke" || c.Status != "succeeded" {
			continue
		}
		page, ok := c.Output["page"].(map[string]any)
		if !ok {
			continue
		}
		scope, _ := page["resource_ids"].([]any)
		inScope := false
		for _, id := range scope {
			inScope = inScope || id == resource
		}
		if !inScope {
			continue
		}
		items, _ := page["items"].([]any)
		hasTargetEvent := false
		for _, r := range items {
			e := r.(map[string]any)
			hasTargetEvent = hasTargetEvent || e["policy_id"] == policy
		}
		evaluations, _ := page["evaluations"].([]any)
		for _, r := range evaluations {
			e := r.(map[string]any)
			if e["policy_id"] == policy && e["health"] == health && e["last_evaluated_at"] != nil {
				observedHealth = true
				// A complete authorized page may contain unrelated policies.
				// Absence is about the requested policy, never the global total.
				if !hasTargetEvent && page["complete"] == true && page["next_cursor"] == nil {
					observedEmpty = true
				}
			}
		}
		for _, r := range items {
			e := r.(map[string]any)
			s, _ := e["state"].(map[string]any)
			if e["policy_id"] == policy && s["notification_state"] == "muted" {
				observedMute = true
			}
		}
	}
	if !observedHealth || muted && !observedMute || empty && !observedEmpty {
		return fmt.Errorf("real model omitted required scope/evaluation/mute/empty facts for %s", policy)
	}
	return nil
}
