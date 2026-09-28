package argusdev

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Core Pods do not all carry the namespace's release label. Verify the actual
// controller UID chain instead of inferring ownership from a Pod name/label.
func (k *E2EKube) VerifyDeploymentPod(ctx context.Context, namespace, deployment, release string, pod corev1.Pod) error {
	ns, err := k.Client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != release || pod.Namespace != namespace {
		return fmt.Errorf("Pod namespace ownership is not established")
	}
	owner := metav1.GetControllerOf(&pod)
	if owner == nil || owner.Kind != "ReplicaSet" || owner.APIVersion != "apps/v1" {
		return fmt.Errorf("Pod is not controlled by a ReplicaSet")
	}
	rs, err := k.Client.AppsV1().ReplicaSets(namespace).Get(ctx, owner.Name, metav1.GetOptions{})
	if err != nil || rs.UID != owner.UID {
		return fmt.Errorf("Pod ReplicaSet identity changed")
	}
	parent := metav1.GetControllerOf(rs)
	if parent == nil || parent.Kind != "Deployment" || parent.APIVersion != "apps/v1" || parent.Name != deployment {
		return fmt.Errorf("ReplicaSet belongs to another Deployment")
	}
	d, err := k.Client.AppsV1().Deployments(namespace).Get(ctx, deployment, metav1.GetOptions{})
	if err != nil || d.UID != parent.UID {
		return fmt.Errorf("Pod Deployment identity changed")
	}
	return nil
}
