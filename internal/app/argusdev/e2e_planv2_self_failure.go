package argusdev

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HTTP 4xx and gRPC ResourceExhausted are not server ERROR spans under OTel's
// conventions. Exercise a real, owned dependency outage so the self-monitoring
// fixture has an actual HTTP 503 entry span without fabricating telemetry.
func (a *App) planV2SelfDependencyFailure(ctx context.Context, env *E2EEnvironment) (failure error) {
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.ObservNS, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("Query namespace ownership is not established")
	}
	const name = "argus-telemetry-query"
	d, err := env.Kube.Client.AppsV1().Deployments(env.ObservNS).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	replicas := int32(1)
	if d.Spec.Replicas != nil {
		replicas = *d.Spec.Replicas
	}
	restored := false
	defer func() {
		if !restored {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
			defer cancel()
			failure = errors.Join(failure, env.Kube.ScaleDeployment(cleanup, env.ObservNS, name, replicas))
		}
	}()
	if err := env.Kube.ScaleDeployment(ctx, env.ObservNS, name, 0); err != nil {
		return err
	}
	deadline := time.Now().Add(2 * time.Minute)
	for {
		pods, e := env.Kube.Client.CoreV1().Pods(env.ObservNS).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + name})
		if e != nil {
			return e
		}
		if len(pods.Items) == 0 {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("owned Query did not stop")
		}
		if err := waitContext(ctx, time.Second); err != nil {
			return err
		}
	}
	client, _ := scenarioHTTP(env)
	if _, err := client.JSON(ctx, "p2-self-real-dependency-failure", "enterprise", http.MethodPost, "/enterprise/metrics/query", http.StatusServiceUnavailable, map[string]any{"query": "argus_m7_e2e_gauge_planv2", "resource_ids": []string{env.State.Values["m3_cluster_id"]}, "time_range": telemetryTimeRange(time.Hour), "budget": telemetryBudget(100)}, enterpriseHeaders(env, "")); err != nil {
		return err
	}
	if err := env.Kube.ScaleDeployment(ctx, env.ObservNS, name, replicas); err != nil {
		return err
	}
	if err := env.Kube.WaitDeployment(ctx, env.ObservNS, name, 3*time.Minute); err != nil {
		return err
	}
	if err := a.waitPlanV2QueryReady(ctx, env); err != nil {
		return err
	}
	restored = true
	return nil
}
