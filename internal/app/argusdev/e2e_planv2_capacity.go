package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/kakj-go/Argus/internal/dashboard"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func planV2CapacitySpec() (dashboard.Spec, error) {
	gallery, err := planV2MetricGallery()
	if err != nil {
		return gallery, err
	}
	spec := dashboard.EmptySpec()
	for i := 0; i < 64; i++ {
		panel := gallery.Panels[1]
		panel.ID, panel.Title = fmt.Sprintf("metric_%02d", i), fmt.Sprintf("Metric %02d", i)
		panel.Layout = dashboard.Rectangle{X: (i % 4) * 3, Y: (i / 4) * 24, W: 3, H: 24, MinW: 3, MinH: 12}
		spec.Panels = append(spec.Panels, panel)
	}
	if report := dashboard.Validate(spec); !report.Valid {
		return spec, fmt.Errorf("capacity fixture: %+v", report.Issues)
	}
	return spec, nil
}

// This bounded acceptance measures the published maximum panel count against
// real Query replicas. It makes no production throughput claim.
func (a *App) runPlanV2Capacity(ctx context.Context, env *E2EEnvironment) (failure error) {
	spec, err := planV2CapacitySpec()
	if err != nil {
		return err
	}
	id, err := a.publishPlanV2Fixture(ctx, env, "PlanV2 64 panel capacity", spec)
	if err != nil {
		return err
	}
	env.State.Values["p2_capacity_id"] = id
	deployment, err := env.Kube.Client.AppsV1().Deployments(env.ObservNS).Get(ctx, "argus-telemetry-query", metav1.GetOptions{})
	if err != nil {
		return err
	}
	if deployment.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("unowned Query deployment")
	}
	original := int32(1)
	if deployment.Spec.Replicas != nil {
		original = *deployment.Spec.Replicas
	}
	scale := func(c context.Context, n int32) error {
		if err := env.Kube.PatchDeployment(c, env.ObservNS, deployment.Name, map[string]any{"spec": map[string]any{"replicas": n}}); err != nil {
			return err
		}
		return env.Kube.WaitDeployment(c, env.ObservNS, deployment.Name, 3*time.Minute)
	}
	if err = scale(ctx, 2); err != nil {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()
		if err := scale(c, original); failure == nil {
			failure = err
		}
	}()
	client, _ := scenarioHTTP(env)
	to := time.Now().UTC().Truncate(time.Second)
	from := to.Add(-15 * time.Minute)
	input := map[string]any{"from": from, "to": to, "resource_ids": []string{env.State.Values["m3_cluster_id"]}}
	measurements := []map[string]any{}
	defer func() {
		raw, _ := json.MarshalIndent(map[string]any{"dashboard_id": id, "query_replicas": 2, "measurements": measurements, "passed": failure == nil, "scope": "64 small real instant queries; not a production throughput benchmark"}, "", "  ")
		if err := writePrivate(filepath.Join(env.Options.Artifacts, "planv2-capacity.json"), raw); failure == nil {
			failure = err
		}
	}()
	for _, phase := range []string{"cold", "warm", "warm_second", "replacement_replicas"} {
		if phase == "replacement_replicas" {
			pods, err := env.Kube.Client.CoreV1().Pods(env.ObservNS).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=argus-telemetry-query"})
			if err != nil {
				return err
			}
			if len(pods.Items) != 2 {
				return fmt.Errorf("expected two live Query replicas")
			}
			previous := map[string]bool{}
			for _, pod := range pods.Items {
				if pod.Labels["argus.io/release-id"] != env.ReleaseID {
					return fmt.Errorf("refusing to replace a foreign Query pod")
				}
				uid := pod.UID
				previous[string(uid)] = true
				if err := env.Kube.Client.CoreV1().Pods(env.ObservNS).Delete(ctx, pod.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil {
					return err
				}
			}
			if err := env.Kube.WaitDeployment(ctx, env.ObservNS, deployment.Name, 3*time.Minute); err != nil {
				return err
			}
			// Deployment status can briefly describe terminating old Pods. Wait
			// for both new identities and the Server's actual RPC connection.
			deadline := time.Now().Add(3 * time.Minute)
			for {
				current, e := env.Kube.Client.CoreV1().Pods(env.ObservNS).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=argus-telemetry-query"})
				if e != nil {
					return e
				}
				ready := 0
				for _, pod := range current.Items {
					if previous[string(pod.UID)] || pod.DeletionTimestamp != nil {
						continue
					}
					for _, condition := range pod.Status.Conditions {
						if condition.Type == "Ready" && condition.Status == "True" {
							ready++
						}
					}
				}
				if ready == 2 && len(current.Items) == 2 {
					break
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("replacement Query replicas did not become ready")
				}
				if err := waitContext(ctx, time.Second); err != nil {
					return err
				}
			}
			if err := a.waitPlanV2QueryReady(ctx, env); err != nil {
				return err
			}
		}
		start := time.Now()
		value, err := client.JSON(ctx, "p2-capacity-"+phase, "enterprise", http.MethodPost, "/dashboards/"+id+"/execute", 200, input, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(value)
		var execution dashboard.Execution
		if err = json.Unmarshal(raw, &execution); err != nil {
			return err
		}
		if execution.Partial || len(execution.Panels) != 64 {
			return fmt.Errorf("capacity execution incomplete")
		}
		hits := 0
		for _, panel := range execution.Panels {
			if panel.Status != "success" {
				return fmt.Errorf("capacity panel %s: %s", panel.ID, panel.Status)
			}
			if panel.Targets[0].Meta.CacheHit {
				hits++
			}
		}
		elapsed := time.Since(start)
		if elapsed > time.Minute {
			return fmt.Errorf("64 panels exceeded execution deadline")
		}
		measurements = append(measurements, map[string]any{"phase": phase, "milliseconds": elapsed.Milliseconds(), "panels": 64, "cache_hits": hits})
	}
	return nil
}
