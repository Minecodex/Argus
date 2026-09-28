package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) checkPlanV2ServerHealth(ctx context.Context, env *E2EEnvironment, before bool) error {
	pods, err := env.Kube.Client.CoreV1().Pods(env.SystemNS).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=argus-server"})
	if err != nil {
		return err
	}
	if len(pods.Items) != 1 {
		return fmt.Errorf("PlanV2 expects one stable server Pod")
	}
	pod := pods.Items[0]
	limit, budget, restarts, ready := int64(0), "", int32(-1), false
	for _, c := range pod.Spec.Containers {
		if c.Name == "argus-server" {
			limit = c.Resources.Limits.Memory().Value()
			for _, e := range c.Env {
				if e.Name == "GOMEMLIMIT" {
					budget = e.Value
				}
			}
		}
	}
	for _, c := range pod.Status.ContainerStatuses {
		if c.Name == "argus-server" {
			restarts, ready = c.RestartCount, c.Ready
		}
	}
	stage := "after"
	if before {
		stage = "before"
		env.State.Values["p2_server_pod_uid"] = string(pod.UID)
	}
	proof := map[string]any{"stage": stage, "pod_uid": string(pod.UID), "same_pod": string(pod.UID) == env.State.Values["p2_server_pod_uid"], "restart_count": restarts, "ready": ready, "memory_limit_bytes": limit, "go_memory_limit": budget}
	encoded, _ := json.MarshalIndent(proof, "", "  ")
	if err := writePrivate(filepath.Join(env.Options.Artifacts, "planv2-server-health-"+stage+".json"), encoded); err != nil {
		return err
	}
	if !ready || restarts != 0 || limit != 256*1024*1024 || budget != "192MiB" || proof["same_pod"] != true {
		return fmt.Errorf("PlanV2 server memory/health invariant failed; see %s evidence", stage)
	}
	return nil
}
