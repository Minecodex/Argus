package kubernetesworkbench

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestLiveHPAApprovalObservesRealCPUScaleUpAndDown(t *testing.T) {
	b := newLiveBehaviorFixture(t, "ARGUS_KUBERNETES_HPA_CONTEXT")
	if _, err := b.client.Discovery().RESTClient().Get().AbsPath("/apis/metrics.k8s.io/v1beta1/nodes").DoRaw(t.Context()); err != nil {
		t.Fatal("actual resource metrics API is required", err)
	}
	labels := map[string]any{"app": "load"}
	b.apply(t, "deployment", "load", "create", map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]any{"name": "load", "namespace": b.name},
		"spec": map[string]any{"replicas": int64(1), "selector": map[string]any{"matchLabels": labels}, "template": map[string]any{
			"metadata": map[string]any{"labels": labels}, "spec": map[string]any{"automountServiceAccountToken": false, "terminationGracePeriodSeconds": int64(0), "containers": []any{map[string]any{
				"name": "main", "image": "busybox:1.36", "imagePullPolicy": "Never", "command": []any{"sh", "-c", "while :; do :; done"},
				"resources": map[string]any{"requests": map[string]any{"cpu": "30m", "memory": "4Mi"}, "limits": map[string]any{"cpu": "50m", "memory": "24Mi"}},
			}}},
		}},
	})
	b.apply(t, "hpa", "load", "create", map[string]any{
		"apiVersion": "autoscaling/v2", "kind": "HorizontalPodAutoscaler", "metadata": map[string]any{"name": "load", "namespace": b.name},
		"spec": map[string]any{"scaleTargetRef": map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "name": "load"}, "minReplicas": int64(1), "maxReplicas": int64(2),
			"metrics":  []any{map[string]any{"type": "Resource", "resource": map[string]any{"name": "cpu", "target": map[string]any{"type": "Utilization", "averageUtilization": int64(50)}}}},
			"behavior": map[string]any{"scaleDown": map[string]any{"stabilizationWindowSeconds": int64(0), "policies": []any{map[string]any{"type": "Pods", "value": int64(1), "periodSeconds": int64(15)}}}},
		},
	})
	waitReplicas := func(want int32) {
		t.Helper()
		err := wait.PollUntilContextTimeout(t.Context(), time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
			hpa, err := b.client.AutoscalingV2().HorizontalPodAutoscalers(b.name).Get(ctx, "load", metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			deployment, err := b.client.AppsV1().Deployments(b.name).Get(ctx, "load", metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			return len(hpa.Status.CurrentMetrics) > 0 && hpa.Status.DesiredReplicas == want && deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == want && deployment.Status.ReadyReplicas == want, nil
		})
		if err != nil {
			hpa, _ := b.client.AutoscalingV2().HorizontalPodAutoscalers(b.name).Get(t.Context(), "load", metav1.GetOptions{})
			if hpa != nil {
				t.Fatalf("actual HPA did not converge to %d replicas: %v; observed=%+v", want, err, hpa.Status)
			}
			t.Fatal(err)
		}
	}
	waitReplicas(2)
	b.apply(t, "deployment", "load", "patch", map[string]any{"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"name": "main", "image": "busybox:1.36", "command": []any{"sleep", "600"}, "resources": map[string]any{"requests": map[string]any{"cpu": "30m", "memory": "4Mi"}, "limits": map[string]any{"cpu": "50m", "memory": "24Mi"}}}}}}}})
	waitReplicas(1)
	b.apply(t, "hpa", "load", "delete", nil)
	t.Log("real CPU resource metrics scaled an approved target 1->2 under bounded load, then 2->1 after an approved workload patch; HPA deletion observed")
}
