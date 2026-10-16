package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Change only real observations, leaving the policy/version unchanged. A
// non-finite member must interrupt its own counters while an independent
// successful member continues, and must never manufacture a recovery.
func (a *App) verifyPlan6FailedMemberContinuity(ctx context.Context, env *E2EEnvironment, base map[string]any, emit func(string) error) (proof map[string]any, failure error) {
	client, _ := scenarioHTTP(env)
	raw, _ := json.Marshal(base)
	spec := map[string]any{}
	_ = json.Unmarshal(raw, &spec)
	spec["mode"], spec["for_seconds"], spec["recovery_seconds"] = "independent", 40, 40
	spec["channel_ids"], spec["route_ids"] = []string{}, []string{}
	spec["error_behavior"], spec["no_data_behavior"] = "keep", "keep"
	memory := spec["rules"].([]any)[1].(map[string]any)
	memory["expression"] = "argus_plan6_memory_percent / (argus_plan6_memory_percent - 90)"
	memory["threshold"], memory["recovery_threshold"], memory["unit"] = 0.5, 0.1, ""
	if failure = emit("100"); failure != nil {
		return nil, failure
	}
	config, failure := a.plan6Config(ctx, env, "continuity-member-failure", map[string]any{"kind": "policy", "operation": "create", "name": "Plan6 independent failed member continuity", "spec": spec})
	if failure != nil {
		return nil, failure
	}
	id, _ := stringField(config, "id")
	proof = map[string]any{"name": "failed-member-resets-counters-and-independent-member-continues", "policy_id": id, "passed": false}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		current, err := client.JSON(c, "p6-continuity-stop-read", "enterprise", http.MethodGet, "/alerts/configurations/"+id, 200, nil, enterpriseHeaders(env, ""))
		if err == nil {
			_, err = a.plan6Config(c, env, "continuity-stop", map[string]any{"kind": "policy", "operation": "disable", "id": id, "expected_version": current["version"]})
		}
		failure = errors.Join(failure, err)
		proof["passed"] = failure == nil
		if failure != nil {
			proof["error"] = failure.Error()
		}
	}()
	conditions := func(value map[string]any) []map[string]any {
		out := []map[string]any{}
		for _, row := range objectItems(value) {
			state, _ := row["state"].(map[string]any)
			if state["kind"] == "condition" {
				out = append(out, row)
			}
		}
		return out
	}
	wait := func(name string, predicate func([]map[string]any) bool) (map[string]any, error) {
		value, err := a.plan6Wait(ctx, env, "continuity-"+name, "/alerts/instances?policy_id="+id, func(value map[string]any) bool {
			rows := conditions(value)
			return len(rows) == 2 && predicate(rows)
		})
		proof[name] = value
		return value, err
	}
	all := func(life string) func([]map[string]any) bool {
		return func(rows []map[string]any) bool {
			return rows[0]["lifecycle"] == life && rows[1]["lifecycle"] == life && rows[0]["health"] == "normal" && rows[1]["health"] == "normal"
		}
	}
	if _, failure = wait("initial_pending", all("pending")); failure != nil {
		return proof, failure
	}
	// CPU remains high; memory is exactly 90, so the real PromQL denominator
	// becomes zero. This exercises the actual query/decoder failure path.
	if failure = emit("95"); failure != nil {
		return proof, failure
	}
	if _, failure = wait("independent_progress", func(rows []map[string]any) bool {
		cpu, failed := 0, 0
		for _, row := range rows {
			state := row["state"].(map[string]any)
			if row["health"] == "error" && row["lifecycle"] == "pending" && state["pending_since"] == nil && state["recovering_since"] == nil {
				failed++
			} else if row["health"] == "normal" && row["lifecycle"] == "firing" {
				cpu++
			}
		}
		return cpu == 1 && failed == 1
	}); failure != nil {
		return proof, failure
	}
	restarted := time.Now().UTC()
	if failure = emit("100"); failure != nil {
		return proof, failure
	}
	if _, failure = wait("member_restarts_pending", func(rows []map[string]any) bool {
		for _, row := range rows {
			state := row["state"].(map[string]any)
			if row["lifecycle"] == "pending" && row["health"] == "normal" {
				at, err := time.Parse(time.RFC3339Nano, fmt.Sprint(state["pending_since"]))
				return err == nil && !at.Before(restarted.Add(-10*time.Second))
			}
		}
		return false
	}); failure != nil {
		return proof, failure
	}
	if _, failure = wait("both_firing", all("firing")); failure != nil {
		return proof, failure
	}
	if failure = emit("60"); failure != nil {
		return proof, failure
	}
	if _, failure = wait("recovering_intermediate_slot", all("recovering")); failure != nil {
		return proof, failure
	}
	if failure = emit("95"); failure != nil {
		return proof, failure
	}
	if _, failure = wait("failed_recovery_is_interrupted", func(rows []map[string]any) bool {
		for _, row := range rows {
			state := row["state"].(map[string]any)
			if row["health"] == "error" && row["lifecycle"] == "recovering" && state["recovering_since"] == nil {
				return true
			}
		}
		return false
	}); failure != nil {
		return proof, failure
	}
	if failure = emit("60"); failure != nil {
		return proof, failure
	}
	if _, failure = wait("recovery_restarts", all("recovering")); failure != nil {
		return proof, failure
	}
	resolved, failure := a.plan6Wait(ctx, env, "continuity-resolved", "/alerts/history?policy_id="+id, func(value map[string]any) bool {
		rows := conditions(value)
		return len(rows) == 2 && all("resolved")(rows)
	})
	proof["resolved"] = resolved
	return proof, failure
}
