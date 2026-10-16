package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) verifyPlan6RuntimeFaults(ctx context.Context, env *E2EEnvironment, policyID string) (failure error) {
	worker, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Get(ctx, "argus-worker", metav1.GetOptions{})
	if err != nil {
		return err
	}
	query, err := env.Kube.Client.AppsV1().Deployments(env.ObservNS).Get(ctx, "argus-telemetry-query", metav1.GetOptions{})
	if err != nil {
		return err
	}
	redis, err := env.Kube.Client.AppsV1().StatefulSets(env.SystemNS).Get(ctx, "argus-redis", metav1.GetOptions{})
	if err != nil {
		return err
	}
	for _, name := range []string{env.SystemNS, env.ObservNS} {
		namespace, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if namespace.Labels["argus.io/release-id"] != env.ReleaseID || namespace.UID == "" {
			return fmt.Errorf("Plan6 fault gate requires an owned namespace")
		}
	}
	if worker.UID == "" || query.UID == "" || redis.UID == "" {
		return fmt.Errorf("Plan6 fault gate requires concrete workload identities")
	}
	workerReplicas, queryReplicas := int32(1), int32(1)
	if worker.Spec.Replicas != nil {
		workerReplicas = *worker.Spec.Replicas
	}
	if query.Spec.Replicas != nil {
		queryReplicas = *query.Spec.Replicas
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 4*time.Minute)
		defer cancel()
		failure = errors.Join(failure, env.Kube.ScaleDeployment(cleanup, env.SystemNS, worker.Name, workerReplicas), env.Kube.ScaleDeployment(cleanup, env.ObservNS, query.Name, queryReplicas))
	}()
	facts := func() (string, error) {
		value, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT COALESCE(json_agg(json_build_object('id',id,'fingerprint',fingerprint,'started_at',started_at,'handling_version',handling_version) ORDER BY id),'[]') FROM alert_instances WHERE policy_id='%s' AND state->>'kind'='condition' AND lifecycle='firing';`, policyID))
		return strings.TrimSpace(value), err
	}
	before, err := facts()
	if err != nil {
		return err
	}
	if before == "[]" {
		return fmt.Errorf("Plan6 fault gate requires live firing conditions")
	}
	if err = env.Kube.ScaleDeployment(ctx, env.SystemNS, worker.Name, 2); err != nil {
		return err
	}
	if err = env.Kube.WaitDeployment(ctx, env.SystemNS, worker.Name, 3*time.Minute); err != nil {
		return err
	}
	password, err := dataCredentialValue(ctx, env, "redis-password")
	if err != nil {
		return err
	}
	if _, err = env.Kube.Exec(ctx, env.SystemNS, "app.kubernetes.io/name=argus-redis", "redis", "redis-cli", "-a", password, "FLUSHALL"); err != nil {
		return err
	}
	password = ""
	if err = env.Kube.PatchDeployment(ctx, env.SystemNS, worker.Name, map[string]any{"spec": map[string]any{"template": map[string]any{"metadata": map[string]any{"annotations": map[string]string{"argus.io/plan6-restart": time.Now().UTC().Format(time.RFC3339Nano)}}}}}); err != nil {
		return err
	}
	if err = env.Kube.WaitDeployment(ctx, env.SystemNS, worker.Name, 3*time.Minute); err != nil {
		return err
	}
	// Stop the real Query dependency. Failed evaluation must retain firing state.
	if err = env.Kube.ScaleDeployment(ctx, env.ObservNS, query.Name, 0); err != nil {
		return err
	}
	if _, err = a.plan6Wait(ctx, env, "query-unavailable", "/alerts/instances?policy_id="+policyID, func(v map[string]any) bool {
		unhealthy := false
		for _, row := range objectItems(v) {
			state, _ := row["state"].(map[string]any)
			if state["kind"] == "condition" {
				unhealthy = unhealthy || row["health"] == "error"
			}
		}
		return unhealthy
	}); err != nil {
		return err
	}
	during, err := facts()
	if err != nil || before != during {
		return fmt.Errorf("Plan6 dependency failure lost or recovered live conditions: %v", err)
	}
	if err = env.Kube.ScaleDeployment(ctx, env.ObservNS, query.Name, queryReplicas); err != nil {
		return err
	}
	if err = env.Kube.WaitDeployment(ctx, env.ObservNS, query.Name, 3*time.Minute); err != nil {
		return err
	}
	if _, err = a.plan6Wait(ctx, env, "query-restored", "/alerts/instances?policy_id="+policyID, func(v map[string]any) bool {
		n := 0
		for _, row := range objectItems(v) {
			state, _ := row["state"].(map[string]any)
			if state["kind"] == "condition" && row["health"] == "normal" && row["lifecycle"] == "firing" {
				n++
			}
		}
		return n == 2
	}); err != nil {
		return err
	}
	after, err := facts()
	if err != nil || before != after {
		return fmt.Errorf("Plan6 restart/Redis reset duplicated or lost conditions: %v", err)
	}
	duplicate, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT count(*) FROM (SELECT instance_id,event_type,count(*) FROM alert_events WHERE policy_id='%s' AND event_type='first' GROUP BY instance_id,event_type HAVING count(*)>1) duplicate;`, policyID))
	if err != nil || strings.TrimSpace(duplicate) != "0" {
		return fmt.Errorf("Plan6 two Workers duplicated first events: %v", err)
	}
	raw, _ := json.MarshalIndent(map[string]any{"passed": true, "worker_uid": worker.UID, "query_uid": query.UID, "redis_uid": redis.UID, "two_workers": true, "worker_restart": true, "redis_flush": true, "query_failure_retains_firing": true, "before": json.RawMessage(before), "during": json.RawMessage(during), "after": json.RawMessage(after)}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan6-runtime-faults.json"), raw)
}
