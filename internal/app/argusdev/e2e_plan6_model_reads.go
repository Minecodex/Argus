package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

func (a *App) verifyPlan6ModelReads(ctx context.Context, env *E2EEnvironment, task, runID string) error {
	evidence, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT COALESCE(json_agg(json_build_object('tool',c.tool_id,'input',c.input,'status',c.status,'output',r.projection::jsonb->'summary') ORDER BY c.created_at,c.call_sequence),'[]') FROM tool_calls c LEFT JOIN tool_results r ON r.tool_call_id=c.id WHERE c.run_id='%s';`, runID))
	if err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan6-read-results-"+task+".json"), []byte(strings.TrimSpace(evidence)+"\n")); err != nil {
		return err
	}
	var calls []struct {
		Tool, Status string
		Input        map[string]any
		Output       map[string]any
	}
	if err = json.Unmarshal([]byte(evidence), &calls); err != nil {
		return err
	}
	search, describe, invoke := -1, -1, -1
	for i, call := range calls {
		if call.Status != "succeeded" {
			continue
		}
		switch call.Tool {
		case "tool.search":
			if search < 0 {
				search = i
			}
		case "tool.describe":
			if describe < 0 {
				describe = i
			}
		case "tool.invoke":
			if invoke < 0 {
				invoke = i
			}
			name, _ := call.Input["name"].(string)
			if strings.HasSuffix(name, ".commit") {
				return fmt.Errorf("model exposed private Commit during %s", task)
			}
		}
	}
	if search < 0 || describe <= search || invoke <= describe {
		return fmt.Errorf("Plan6 %s missing ordered autonomous search/describe/invoke", task)
	}
	if task != "query" && task != "exists" && task != "missing" {
		return nil
	}
	policyRead, matchedPolicy, eventHealth := false, false, false
	for _, call := range calls {
		if call.Status != "succeeded" || call.Tool != "tool.invoke" {
			continue
		}
		name, _ := call.Input["name"].(string)
		category, _ := call.Input["category"].(string)
		if category != "alert" && !strings.Contains(name, "alert") {
			continue
		}
		if strings.Contains(name, "policy") {
			policyRead = true
			items, _ := call.Output["items"].([]any)
			if item, ok := call.Output["configuration"].(map[string]any); ok {
				items = append(items, item)
			}
			for _, raw := range items {
				item, _ := raw.(map[string]any)
				if item["name"] == "Plan6 不存在的策略" {
					return fmt.Errorf("model tool returned nonexistent policy")
				}
				if item["id"] == env.State.Values["p6_policy_id"] && item["name"] == "Plan6 real pressure" && item["health"] != nil && item["last_evaluated_at"] != nil {
					matchedPolicy = true
				}
			}
		}
		if page, ok := call.Output["page"].(map[string]any); ok {
			scope, _ := page["resource_ids"].([]any)
			evaluations, _ := page["evaluations"].([]any)
			hasScope := false
			for _, id := range scope {
				hasScope = hasScope || id == env.State.Values["m3_cluster_id"]
			}
			for _, raw := range evaluations {
				evaluation, _ := raw.(map[string]any)
				if hasScope && evaluation["policy_id"] == env.State.Values["p6_policy_id"] && evaluation["health"] != nil && evaluation["last_evaluated_at"] != nil {
					eventHealth = true
				}
			}
		}
	}
	if task == "query" && !eventHealth {
		return fmt.Errorf("model query omitted alert scope/evaluation health facts")
	}
	if task == "exists" && (!policyRead || !matchedPolicy) {
		return fmt.Errorf("model existence check did not read the actual policy and evaluation")
	}
	if task == "missing" {
		if !policyRead {
			return fmt.Errorf("model absence claim has no policy read")
		}
		reply, err := a.postgresQuery(ctx, env, "SELECT payload->>'content' FROM conversation_events WHERE run_id='"+runID+"' AND event_type='assistant_message' ORDER BY sequence DESC LIMIT 1;")
		if err != nil {
			return err
		}
		reply = strings.ReplaceAll(reply, "Plan6 不存在的策略", "")
		if !strings.Contains(reply, "不存在") && !strings.Contains(reply, "未找到") && !strings.Contains(reply, "没有找到") && !strings.Contains(reply, "未查询到") {
			return fmt.Errorf("model did not clearly report the absent policy")
		}
	}
	return nil
}
