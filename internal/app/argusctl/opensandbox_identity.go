package argusctl

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (pin *SharedSandboxController) validate() error {
	if pin == nil {
		return nil
	}
	if len(pin.Namespace) > 63 || !dnsLabel.MatchString(pin.Namespace) || !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(pin.ImageDigest) {
		return fmt.Errorf("openSandbox.sharedController requires a namespace and an immutable sha256 imageDigest")
	}
	return nil
}

func verifySharedSandboxController(ctx context.Context, clients *kubeClients, deployment *appsv1.Deployment, pin *SharedSandboxController) error {
	if err := pin.validate(); err != nil {
		return err
	}
	if deployment.Namespace != pin.Namespace || deployment.UID == "" || deployment.Spec.Selector == nil {
		return fmt.Errorf("shared OpenSandbox controller identity mismatch")
	}
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	if desired < 1 || deployment.Status.UpdatedReplicas != desired || deployment.Status.ReadyReplicas != desired {
		return fmt.Errorf("shared OpenSandbox controller rollout is not ready")
	}
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return err
	}
	pods, err := clients.typed.CoreV1().Pods(pin.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return err
	}
	verified := int32(0)
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		owner := metav1.GetControllerOf(&pod)
		if owner == nil || owner.Kind != "ReplicaSet" {
			continue
		}
		rs, err := clients.typed.AppsV1().ReplicaSets(pin.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if rs.UID != owner.UID || !controllerOwnedBy(rs.OwnerReferences, "Deployment", deployment.UID) {
			continue
		}
		found := false
		for _, container := range pod.Status.ContainerStatuses {
			if container.Name != "manager" {
				continue
			}
			imageID := container.ImageID
			if i := strings.LastIndex(imageID, "@"); i >= 0 {
				imageID = imageID[i+1:]
			}
			if !container.Ready || container.State.Running == nil || imageID != pin.ImageDigest {
				return fmt.Errorf("shared OpenSandbox controller running image/readiness mismatch in Pod %s", pod.Name)
			}
			found = true
		}
		if !found {
			return fmt.Errorf("shared OpenSandbox controller Pod has no ready manager: %s", pod.Name)
		}
		verified++
	}
	if verified != desired {
		return fmt.Errorf("shared OpenSandbox controller has %d verified replicas, expected %d", verified, desired)
	}
	return nil
}

func controllerOwnedBy(owners []metav1.OwnerReference, kind string, uid types.UID) bool {
	for _, owner := range owners {
		if owner.Controller != nil && *owner.Controller && owner.Kind == kind && owner.UID == uid {
			return true
		}
	}
	return false
}
