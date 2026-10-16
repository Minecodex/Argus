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

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Faults affect only this runner's owned database and workloads. All metric
// observations still travel through OTLP, storage and the real Query RPC.
func (a *App) verifyPlan6LifecycleEdges(ctx context.Context, env *E2EEnvironment, base map[string]any, emit func(string) error) (failure error) {
	client, _ := scenarioHTTP(env)
	enterprise, cluster := env.State.Values["enterprise_id"], env.State.Values["m3_cluster_id"]
	if uuid.Validate(enterprise) != nil || uuid.Validate(cluster) != nil {
		return fmt.Errorf("lifecycle edges need owned fixture identities")
	}
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("lifecycle edges require the owned namespace: %v", err)
	}
	cases := []map[string]any{}
	policies := []string{}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()
		for _, id := range policies {
			current, e := client.JSON(c, "p6-edge-stop-read-"+id, "enterprise", http.MethodGet, "/alerts/configurations/"+id, 200, nil, enterpriseHeaders(env, ""))
			if e == nil {
				_, e = a.plan6Config(c, env, "edge-stop-"+id, map[string]any{"kind": "policy", "operation": "disable", "id": id, "expected_version": current["version"]})
			}
			failure = errors.Join(failure, e)
		}
		failure = errors.Join(failure, emit("60"))
		raw, _ := json.MarshalIndent(map[string]any{"passed": failure == nil, "source": "real OTLP/Query; owned PostgreSQL and Worker fault controls", "cases": cases}, "", "  ")
		failure = errors.Join(failure, writePrivate(filepath.Join(env.Options.Artifacts, "plan6-lifecycle-edges.json"), raw))
	}()
	spec := func() map[string]any {
		b, _ := json.Marshal(base)
		out := map[string]any{}
		_ = json.Unmarshal(b, &out)
		out["channel_ids"], out["route_ids"] = []string{}, []string{}
		out["no_data_behavior"], out["error_behavior"] = "keep", "keep"
		return out
	}
	create := func(name string, body map[string]any) (string, error) {
		row, e := a.plan6Config(ctx, env, "edge-"+name, map[string]any{"kind": "policy", "operation": "create", "name": "Plan6 edge " + name, "spec": body})
		if e != nil {
			return "", e
		}
		id, e := stringField(row, "id")
		policies = append(policies, id)
		return id, e
	}
	waitLife := func(id, life string) (map[string]any, error) {
		return a.plan6Wait(ctx, env, "edge-"+life, "/alerts/instances?policy_id="+id, func(v map[string]any) bool {
			n := 0
			for _, row := range objectItems(v) {
				state, _ := row["state"].(map[string]any)
				if state["kind"] == "condition" && row["lifecycle"] == life {
					n++
				}
			}
			return n == 2
		})
	}
	if err = emit("95"); err != nil {
		return err
	}
	// A complete member plus missing authorized sources is still incomplete.
	partial := spec()
	partial["mode"] = "every"
	partial["resources"].(map[string]any)["source_ids"] = []string{}
	trial, err := client.JSON(ctx, "p6-edge-partial", "enterprise", http.MethodPost, "/alerts/trial", 200, partial, enterpriseHeaders(env, ""))
	if err != nil {
		return err
	}
	complete, incomplete := 0, 0
	for _, raw := range trial["outcomes"].([]any) {
		outcome := raw.(map[string]any)
		if outcome["complete"] == true {
			complete++
		} else {
			incomplete++
		}
	}
	if complete < 2 || incomplete == 0 || trial["health"] != "no_data" {
		return fmt.Errorf("partial source membership was wrongly complete: %v", trial)
	}
	cases = append(cases, map[string]any{"name": "partial-authorized-sources", "passed": true, "query": trial})
	// Both rule members have real data, but different requested dimensions.
	_, err = env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-server", "plan6-telemetry-fixture", "/usr/local/bin/argus-telemetry-e2e", "--endpoint=127.0.0.1:4317", "--resource-id="+cluster, "--plan6-alert-value=95", "--plan6-member-split")
	if err != nil {
		return err
	}
	members := spec()
	members["mode"], members["match_by"] = "every", []string{"plan6_member"}
	rules := members["rules"].([]any)
	rules[0].(map[string]any)["expression"] = "argus_plan6_member_cpu_percent"
	rules[1].(map[string]any)["expression"] = "argus_plan6_member_memory_percent"
	joined, err := a.plan6WaitTrial(ctx, env, "different-members", members, func(v map[string]any) bool {
		outcomes, _ := v["outcomes"].([]any)
		if len(outcomes) != 2 {
			return false
		}
		for _, raw := range outcomes {
			if raw.(map[string]any)["complete"] != true {
				return false
			}
		}
		obs, _ := v["observations"].([]any)
		return len(obs) == 0 && v["health"] == "no_data"
	})
	if err != nil {
		return err
	}
	cases = append(cases, map[string]any{"name": "complete-distinct-members-do-not-join", "passed": true, "query": joined})
	identities, err := a.verifyPlan6IdentityQueries(ctx, env, base, emit)
	if identities != nil {
		cases = append(cases, identities)
	}
	if err != nil {
		return err
	}
	continuity, err := a.verifyPlan6FailedMemberContinuity(ctx, env, base, emit)
	if continuity != nil {
		cases = append(cases, continuity)
	}
	if err != nil {
		return err
	}
	if err = emit("95"); err != nil {
		return err
	}
	// Check the intermediate state after a missed slot, not just eventual fire.
	sustain := spec()
	sustain["for_seconds"], sustain["recovery_seconds"] = 40, 40
	id, err := create("missed-slots", sustain)
	if err != nil {
		return err
	}
	pending, err := waitLife(id, "pending")
	if err != nil {
		return err
	}
	worker, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Get(ctx, "argus-worker", metav1.GetOptions{})
	if err != nil {
		return err
	}
	replicas := int32(1)
	if worker.Spec.Replicas != nil {
		replicas = *worker.Spec.Replicas
	}
	restoreWorker := func(c context.Context) error {
		current, e := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Get(c, worker.Name, metav1.GetOptions{})
		if e != nil || current.UID != worker.UID {
			return fmt.Errorf("owned Worker identity changed: %v", e)
		}
		if e = env.Kube.ScaleDeployment(c, env.SystemNS, worker.Name, replicas); e != nil {
			return e
		}
		return env.Kube.WaitDeployment(c, env.SystemNS, worker.Name, 2*time.Minute)
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()
		failure = errors.Join(failure, restoreWorker(c))
	}()
	if err = env.Kube.ScaleDeployment(ctx, env.SystemNS, worker.Name, 0); err != nil {
		return err
	}
	if err = waitContext(ctx, 35*time.Second); err != nil {
		return err
	}
	resumed := time.Now().UTC()
	if err = restoreWorker(ctx); err != nil {
		return err
	}
	reset, err := a.plan6Wait(ctx, env, "missed-slot-reset", "/alerts/instances?policy_id="+id, func(v map[string]any) bool {
		n := 0
		for _, row := range objectItems(v) {
			state, _ := row["state"].(map[string]any)
			if state["kind"] != "condition" {
				continue
			}
			at, _ := time.Parse(time.RFC3339Nano, fmt.Sprint(state["pending_since"]))
			if row["lifecycle"] == "pending" && !at.Before(resumed.Truncate(10*time.Second)) {
				n++
			}
		}
		return n == 2
	})
	if err != nil {
		return err
	}
	if _, err = waitLife(id, "firing"); err != nil {
		return err
	}
	if err = emit("60"); err != nil {
		return err
	}
	recovering, err := waitLife(id, "recovering")
	if err != nil {
		return err
	}
	cases = append(cases, map[string]any{"name": "missed-slots-reset-trigger-and-recovery-has-intermediate-state", "passed": true, "before": pending, "after_restart": reset, "recovering": recovering})
	// Separate source recovery from the target's still-firing observation.
	if err = emit("95"); err != nil {
		return err
	}
	noise := spec()
	noiseID, err := create("inhibition", noise)
	if err != nil {
		return err
	}
	if _, err = waitLife(noiseID, "firing"); err != nil {
		return err
	}
	inhibition, err := a.plan6Config(ctx, env, "edge-inhibition-configuration", map[string]any{"kind": "inhibition", "operation": "create", "name": "Plan6 edge inhibition", "spec": map[string]any{"enabled": true, "resource_ids": []string{cluster}, "source": []map[string]string{{"key": "severity", "operator": "=", "value": "critical"}, {"key": "policy_id", "operator": "=", "value": noiseID}}, "target": []map[string]string{{"key": "severity", "operator": "=", "value": "warning"}, {"key": "policy_id", "operator": "=", "value": noiseID}}, "equal": []string{"resource_id"}, "reason": "source recovery acceptance"}})
	if err != nil {
		return err
	}
	muted, err := a.plan6Wait(ctx, env, "edge-inhibited", "/alerts/instances?policy_id="+noiseID, func(v map[string]any) bool {
		for _, row := range objectItems(v) {
			s, _ := row["state"].(map[string]any)
			if s["metric"] == "argus_plan6_memory_percent" && s["notification_state"] == "inhibited" {
				return true
			}
		}
		return false
	})
	if err != nil {
		return err
	}
	_, err = env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-server", "plan6-telemetry-fixture", "/usr/local/bin/argus-telemetry-e2e", "--endpoint=127.0.0.1:4317", "--resource-id="+cluster, "--plan6-alert-value=60", "--plan6-memory-value=90")
	if err != nil {
		return err
	}
	unmuted, err := a.plan6Wait(ctx, env, "edge-uninhibited", "/alerts/instances?policy_id="+noiseID, func(v map[string]any) bool {
		for _, row := range objectItems(v) {
			s, _ := row["state"].(map[string]any)
			if s["metric"] == "argus_plan6_memory_percent" && row["lifecycle"] == "firing" && s["notification_state"] != "inhibited" {
				return true
			}
		}
		return false
	})
	if err != nil {
		return err
	}
	inhibitionID, _ := stringField(inhibition, "id")
	if _, err = a.plan6Config(ctx, env, "edge-inhibition-stop", map[string]any{"kind": "inhibition", "operation": "disable", "id": inhibitionID, "expected_version": inhibition["version"]}); err != nil {
		return err
	}
	events, err := a.postgresQuery(ctx, env, fmt.Sprintf("SELECT count(*) FROM alert_events WHERE policy_id='%s' AND event_type IN ('inhibited','uninhibited');", noiseID))
	if err != nil || strings.TrimSpace(events) == "0" {
		return fmt.Errorf("inhibition timeline missing: %v", err)
	}
	cases = append(cases, map[string]any{"name": "inhibition-source-recovers-target-stays-firing", "passed": true, "inhibited": muted, "uninhibited": unmuted, "history_events": strings.TrimSpace(events)})
	// Source and resource termination are explicit closures in the real Worker.
	if err = emit("95"); err != nil {
		return err
	}
	terminal := spec()
	terminal["resources"].(map[string]any)["source_ids"] = []string{}
	terminalID, err := create("termination", terminal)
	if err != nil {
		return err
	}
	if _, err = waitLife(terminalID, "firing"); err != nil {
		return err
	}
	source := base["resources"].(map[string]any)["source_ids"].([]string)[0]
	// A source has one row per configuration revision. Resolve its owning
	// collector once, preserving every revision as evidence rather than parsing
	// several psql JSON rows as one object.
	raw, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT COALESCE(json_agg(fact),'[]'::json) FROM (
SELECT c.id,c.status,(SELECT json_agg(s.config_revision ORDER BY s.config_revision)
FROM telemetry_sources s WHERE s.collector_id=c.id AND s.id='%s' AND s.enterprise_id='%s') AS source_revisions
FROM collector_instances c WHERE c.enterprise_id='%s' AND c.resource_id='%s'
AND c.resource_type='kubernetes_cluster' AND EXISTS (
SELECT 1 FROM telemetry_sources s WHERE s.collector_id=c.id AND s.id='%s'
AND s.enterprise_id=c.enterprise_id AND s.resource_id=c.resource_id)) fact;`, source, enterprise, enterprise, cluster, source))
	if err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan6-termination-source.json"), []byte(raw)); err != nil {
		return err
	}
	var collectors []struct{ ID, Status string }
	if err = json.Unmarshal([]byte(raw), &collectors); err != nil {
		return fmt.Errorf("decode termination source ownership: %w", err)
	}
	if len(collectors) != 1 || uuid.Validate(collectors[0].ID) != nil {
		return fmt.Errorf("termination source %s expected one owned collector, got %d; see plan6-termination-source.json", source, len(collectors))
	}
	collector := collectors[0]
	restoreSource := func(c context.Context) error {
		_, e := a.postgresQuery(c, env, fmt.Sprintf("UPDATE collector_instances SET status='%s' WHERE id='%s' AND enterprise_id='%s';", collector.Status, collector.ID, enterprise))
		return e
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		failure = errors.Join(failure, restoreSource(c))
	}()
	if _, err = a.postgresQuery(ctx, env, fmt.Sprintf("UPDATE collector_instances SET status='uninstalled' WHERE id='%s' AND enterprise_id='%s';", collector.ID, enterprise)); err != nil {
		return err
	}
	if err = a.waitPostgresValue(ctx, env, fmt.Sprintf("SELECT count(*) FROM alert_instances WHERE policy_id='%s' AND lifecycle='closed' AND state->>'closure_reason'='source_changed';", terminalID), "2", time.Minute); err != nil {
		return err
	}
	if err = restoreSource(ctx); err != nil {
		return err
	}
	if _, err = waitLife(terminalID, "firing"); err != nil {
		return err
	}
	restoreResource := func(c context.Context) error {
		_, e := a.postgresQuery(c, env, fmt.Sprintf("UPDATE kubernetes_clusters SET status='active' WHERE id='%s' AND enterprise_id='%s';", cluster, enterprise))
		return e
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		failure = errors.Join(failure, restoreResource(c))
	}()
	if _, err = a.postgresQuery(ctx, env, fmt.Sprintf("UPDATE kubernetes_clusters SET status='deleted' WHERE id='%s' AND enterprise_id='%s';", cluster, enterprise)); err != nil {
		return err
	}
	if err = a.waitPostgresValue(ctx, env, fmt.Sprintf("SELECT count(*) FROM alert_instances WHERE policy_id='%s' AND lifecycle='closed' AND state->>'closure_reason'='resource_deleted';", terminalID), "2", time.Minute); err != nil {
		return err
	}
	if err = restoreResource(ctx); err != nil {
		return err
	}
	cases = append(cases, map[string]any{"name": "source-and-resource-termination-have-explicit-closure-reasons", "passed": true, "policy_id": terminalID})
	return nil
}

func (a *App) plan6WaitTrial(ctx context.Context, env *E2EEnvironment, name string, spec map[string]any, ready func(map[string]any) bool) (map[string]any, error) {
	client, _ := scenarioHTTP(env)
	deadline := time.Now().Add(time.Minute)
	for attempt := 0; ; attempt++ {
		out, err := client.JSON(ctx, fmt.Sprintf("p6-trial-edge-%s-%d", name, attempt), "enterprise", http.MethodPost, "/alerts/trial", 200, spec, enterpriseHeaders(env, ""))
		if err != nil {
			return nil, err
		}
		if ready(out) {
			return out, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("real Query edge %s did not become ready: %v", name, out)
		}
		if err = waitContext(ctx, time.Second); err != nil {
			return nil, err
		}
	}
}
