package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
)

func (a *App) verifyPlan6EvaluationMatrix(ctx context.Context, env *E2EEnvironment, base map[string]any, emit func(string) error) (failure error) {
	client, _ := scenarioHTTP(env)
	rows := []map[string]any{}
	defer func() {
		raw, _ := json.MarshalIndent(map[string]any{"passed": failure == nil, "cases": rows, "source": "real OTLP, existing PromQL Query RPC and PostgreSQL slots"}, "", "  ")
		failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "plan6-evaluation-matrix.json"), raw))
	}()
	if err := emit("95"); err != nil {
		return err
	}
	for _, tc := range []struct {
		name, mode, expression, health string
		count                          int
		sustain                        int
	}{
		{"and-complete", "every", "argus_plan6_memory_percent", "normal", 1, 0},
		{"or-complete", "any", "argus_plan6_memory_percent", "normal", 1, 0},
		{"independent-missing", "independent", "argus_plan6_missing_percent", "no_data", 1, 0},
		{"and-missing", "every", "argus_plan6_missing_percent", "no_data", 0, 0},
		{"nonfinite-member", "every", "argus_plan6_memory_percent / 0", "error", 0, 0},
		{"nan-member", "every", "(argus_plan6_memory_percent - 90) / (argus_plan6_memory_percent - 90)", "error", 0, 0},
		{"continuous-slots", "every", "argus_plan6_memory_percent", "normal", 1, 20},
		{"counter-reset-rate", "every", "rate(argus_plan6_counter_total[1m])", "normal", 1, 0},
	} {
		if err := emit("95"); err != nil {
			return err
		}
		raw, _ := json.Marshal(base)
		var spec map[string]any
		_ = json.Unmarshal(raw, &spec)
		spec["mode"], spec["for_seconds"], spec["recovery_seconds"] = tc.mode, tc.sustain, tc.sustain
		spec["channel_ids"], spec["route_ids"] = []string{}, []string{}
		spec["rules"].([]any)[1].(map[string]any)["expression"] = tc.expression
		if tc.name == "counter-reset-rate" {
			rule := spec["rules"].([]any)[1].(map[string]any)
			rule["threshold"], rule["recovery_threshold"], rule["unit"] = 0.1, 0.05, "/s"
		}
		config, err := a.plan6Config(ctx, env, "matrix-"+tc.name, map[string]any{"kind": "policy", "operation": "create", "name": "Plan6 matrix " + tc.name, "spec": spec})
		if err != nil {
			return err
		}
		id, _ := stringField(config, "id")
		observation, err := a.plan6Wait(ctx, env, "matrix-"+tc.name, "/alerts/instances?policy_id="+id, func(v map[string]any) bool {
			healthOK := false
			evaluations, _ := v["evaluations"].([]any)
			for _, item := range evaluations {
				e, _ := item.(map[string]any)
				healthOK = healthOK || e["health"] == tc.health
			}
			count := 0
			for _, item := range objectItems(v) {
				s, _ := item["state"].(map[string]any)
				if s["kind"] == "condition" && item["lifecycle"] == "firing" {
					count++
				}
			}
			return healthOK && count == tc.count
		})
		if err != nil {
			return fmt.Errorf("evaluation matrix %s: %w", tc.name, err)
		}
		row := map[string]any{"name": tc.name, "policy_id": id, "firing": observation, "passed": true}
		rows = append(rows, row)
		if tc.sustain > 0 {
			if err = emit("60"); err != nil {
				return err
			}
			recovery, err := a.plan6Wait(ctx, env, "matrix-recovery", "/alerts/history?policy_id="+id, func(v map[string]any) bool {
				for _, item := range objectItems(v) {
					if item["lifecycle"] == "resolved" {
						return true
					}
				}
				return false
			})
			if err != nil {
				return err
			}
			row["resolved"] = recovery
		}
		current, err := client.JSON(ctx, "p6-matrix-current-"+tc.name, "enterprise", http.MethodGet, "/alerts/configurations/"+id, 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		version, _ := numberField(current, "version")
		if _, err = a.plan6Config(ctx, env, "matrix-stop-"+tc.name, map[string]any{"kind": "policy", "operation": "disable", "id": id, "expected_version": version}); err != nil {
			return err
		}
	}
	return emit("60")
}
