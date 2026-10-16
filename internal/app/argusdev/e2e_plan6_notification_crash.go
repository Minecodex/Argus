package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) crashPlan6NotificationWorker(ctx context.Context, env *E2EEnvironment, channelID, caseID string, sent bool) error {
	forward, err := env.Kube.PortForwardService(ctx, env.ReplayNamespace(), "argus-replay-model", []string{"0:8081"}, io.Discard)
	if err != nil {
		return err
	}
	defer forward.Stop()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(25 * time.Second)
	for {
		ready := false
		if sent {
			request, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/fixtures/notifications/", forward.Ports[0].Local), nil)
			response, e := client.Do(request)
			if e != nil {
				return e
			}
			var body struct {
				Items []map[string]any `json:"items"`
			}
			e = json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&body)
			_ = response.Body.Close()
			if e != nil {
				return e
			}
			for _, row := range body.Items {
				ready = ready || row["case"] == caseID && row["accepted"] == true
			}
		} else {
			value, e := a.postgresQuery(ctx, env, fmt.Sprintf("SELECT count(*) FROM alert_delivery_batches WHERE channel_id='%s' AND status='prepared' AND send_started_at IS NULL;", channelID))
			if e != nil {
				return e
			}
			ready = strings.TrimSpace(value) == "1"
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("notification crash barrier not reached (after_send=%t)", sent)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	namespace, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || namespace.UID == "" || namespace.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("notification crash requires owned namespace: %v", err)
	}
	worker, err := env.Kube.Client.AppsV1().Deployments(env.SystemNS).Get(ctx, "argus-worker", metav1.GetOptions{})
	if err != nil || worker.UID == "" {
		return fmt.Errorf("notification crash requires worker UID: %v", err)
	}
	pods, err := env.Kube.Client.CoreV1().Pods(env.SystemNS).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=argus-worker"})
	if err != nil || len(pods.Items) == 0 {
		return fmt.Errorf("owned worker pods unavailable: %v", err)
	}
	for _, pod := range pods.Items {
		owner := metav1.GetControllerOf(&pod)
		if owner == nil || owner.Kind != "ReplicaSet" {
			return fmt.Errorf("worker pod has no concrete controller")
		}
		rs, err := env.Kube.Client.AppsV1().ReplicaSets(env.SystemNS).Get(ctx, owner.Name, metav1.GetOptions{})
		if err != nil || rs.UID != owner.UID || metav1.GetControllerOf(rs) == nil || metav1.GetControllerOf(rs).UID != worker.UID {
			return fmt.Errorf("worker pod ownership changed: %v", err)
		}
		uid, zero := pod.UID, int64(0)
		if err = env.Kube.Client.CoreV1().Pods(env.SystemNS).Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero, Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil {
			return err
		}
	}
	// Make the lost last attempt reclaimable. Runtime's exhaustion callback must
	// reconcile prepared batches or Unknown sends; no delivery is rewritten here.
	if _, err = a.postgresQuery(ctx, env, fmt.Sprintf(`UPDATE runtime_tasks t SET lease_until=now()-interval '1 minute',max_attempts=attempt FROM alert_delivery_batches b WHERE b.owner_task_id=t.id AND b.channel_id='%s' AND t.enterprise_id='%s' AND t.queue='alert-notification' AND t.status IN ('running','leased');`, channelID, env.State.Values["enterprise_id"])); err != nil {
		return err
	}
	return env.Kube.WaitDeployment(ctx, env.SystemNS, worker.Name, 3*time.Minute)
}
