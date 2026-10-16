package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Use a fresh owned Host after the backend rollout replaces legacy test OS
// sidecars. Restore its uninstalled Collector state afterwards.
func (a *App) verifyPlan6IdentityQueries(ctx context.Context, env *E2EEnvironment, base map[string]any, emit func(string) error) (proof map[string]any, failure error) {
	host, failure := a.preparePlan6IdentityHost(ctx, env)
	if failure != nil {
		return nil, failure
	}
	previous := env.State.Values["m7_host_id"]
	env.State.Values["m7_host_id"] = host
	defer func() { env.State.Values["m7_host_id"] = previous }()
	enterprise, cluster, account := env.State.Values["enterprise_id"], env.State.Values["m3_cluster_id"], env.State.Values["p6_evaluator_id"]
	for _, id := range []string{enterprise, host, cluster, account} {
		if uuid.Validate(id) != nil {
			return nil, fmt.Errorf("identity Query needs owned fixture identities")
		}
	}
	distribution, profiles, _, failure := a.verifyM7Catalog(ctx, env)
	if failure != nil {
		return nil, failure
	}
	if failure = a.applyM7CollectorActionWithImage(ctx, env, "host", host, "install", distribution, profiles, "", "p6-identity"); failure != nil {
		return nil, failure
	}
	grant := uuid.NewString()
	proof = map[string]any{"name": "real-query-preserves-resource-and-source-identity", "passed": false, "host_id": host, "cluster_id": cluster}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()
		_, err := a.postgresQuery(c, env, fmt.Sprintf("DELETE FROM data_authorization_grants WHERE id='%s' AND enterprise_id='%s' AND subject_id='%s'; UPDATE service_accounts SET authorization_version=authorization_version+1 WHERE id='%s' AND enterprise_id='%s';", grant, enterprise, account, account, enterprise))
		failure = errors.Join(failure, err, a.applyM7CollectorActionWithImage(c, env, "host", host, "uninstall", distribution, profiles, "", "p6-identity"))
		failure = errors.Join(failure, a.waitM7HostCollector(c, env, "uninstalled"))
		proof["passed"] = failure == nil
		if failure != nil {
			proof["error"] = failure.Error()
		}
	}()
	if failure = a.waitM7HostCollector(ctx, env, "converged"); failure != nil {
		return proof, failure
	}
	_, failure = a.postgresQuery(ctx, env, fmt.Sprintf("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES('%s','%s','service_account','%s','host','%s'); UPDATE service_accounts SET authorization_version=authorization_version+1 WHERE id='%s' AND enterprise_id='%s';", grant, enterprise, account, host, account, enterprise))
	if failure != nil {
		return proof, failure
	}
	if failure = a.generateM7HostSignals(ctx, env, "plan6-identity"); failure != nil {
		return proof, failure
	}
	if failure = emit("95"); failure != nil {
		return proof, failure
	}
	makeSpec := func() map[string]any {
		raw, _ := json.Marshal(base)
		out := map[string]any{}
		_ = json.Unmarshal(raw, &out)
		out["match_by"], out["mode"] = []string{}, "independent"
		out["channel_ids"], out["route_ids"] = []string{}, []string{}
		out["resources"] = map[string]any{"mode": "explicit", "types": []string{"host", "kubernetes_cluster"}, "ids": []string{cluster, host}, "source_ids": []string{}, "matchers": []any{}}
		rules := out["rules"].([]any)
		rules[0].(map[string]any)["expression"] = fmt.Sprintf(`argus_plan6_cpu_percent{argus_resource_id="%s"}`, cluster)
		other := rules[1].(map[string]any)
		other["expression"], other["threshold"], other["recovery_threshold"], other["unit"] = "argus_m7_e2e_gauge_plan6_identity", 5, 1, ""
		return out
	}
	separate := makeSpec()
	independent, failure := a.plan6WaitTrial(ctx, env, "identity-different-resources-independent", separate, func(value map[string]any) bool {
		rows, _ := value["observations"].([]any)
		if len(rows) != 2 {
			return false
		}
		left, right := rows[0].(map[string]any), rows[1].(map[string]any)
		return fmt.Sprint(left["resource_ids"]) != fmt.Sprint(right["resource_ids"]) && fmt.Sprint(left["source_ids"]) != fmt.Sprint(right["source_ids"]) && left["firing"] == true && right["firing"] == true
	})
	proof["different_resources_independent"] = independent
	if failure != nil {
		return proof, failure
	}
	separate["mode"] = "every"
	joined, failure := a.plan6WaitTrial(ctx, env, "identity-different-resources-and", separate, func(value map[string]any) bool {
		rows, _ := value["observations"].([]any)
		return len(rows) == 0 && value["health"] == "no_data"
	})
	proof["different_resources_and"] = joined
	if failure != nil {
		return proof, failure
	}
	// Host CPU time is scraped by hostmetrics, while the gauge above arrives
	// through OTLP. Both belong to the same Host but distinct trusted sources.
	bySource := makeSpec()
	bySource["resources"].(map[string]any)["ids"] = []string{host}
	bySource["resources"].(map[string]any)["types"] = []string{"host"}
	first := bySource["rules"].([]any)[0].(map[string]any)
	first["expression"], first["threshold"], first["recovery_threshold"], first["unit"] = `count by (argus_resource_id, argus_source_id) ({__name__=~"system_cpu_time.*"})`, 0, -1, ""
	sourceMembers, failure := a.plan6WaitTrial(ctx, env, "identity-same-resource-distinct-sources", bySource, func(value map[string]any) bool {
		rows, _ := value["observations"].([]any)
		if len(rows) != 2 {
			return false
		}
		left, right := rows[0].(map[string]any), rows[1].(map[string]any)
		return fmt.Sprint(left["resource_ids"]) == fmt.Sprint(right["resource_ids"]) && fmt.Sprint(left["source_ids"]) != fmt.Sprint(right["source_ids"]) && left["firing"] == true && right["firing"] == true
	})
	proof["same_resource_distinct_sources_independent"] = sourceMembers
	if failure != nil {
		return proof, failure
	}
	bySource["mode"] = "every"
	sourceJoined, failure := a.plan6WaitTrial(ctx, env, "identity-distinct-sources-and", bySource, func(value map[string]any) bool {
		rows, _ := value["observations"].([]any)
		return len(rows) == 0 && value["health"] == "no_data"
	})
	proof["same_resource_distinct_sources_and"] = sourceJoined
	return proof, failure
}
