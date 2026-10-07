package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Isolated local tests use port-forwards rather than a cloud LoadBalancer.
// Docker Desktop may retain its finalizer even when it never allocated a LB.
// This exception applies only to an owned, unallocated, deleting test Service.
func (k *E2EKube) CleanupUnallocatedLocalService(ctx context.Context, namespace, name, releaseID string) error {
	if k.Context != "docker-desktop" {
		return nil
	}
	ns, err := k.Client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if ns.Labels["argus.io/release-id"] != releaseID {
		return fmt.Errorf("refusing local service cleanup outside E2E owner")
	}
	services := k.Client.CoreV1().Services(namespace)
	svc, err := services.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !ownedUnallocatedService(svc, namespace, releaseID) {
		return nil
	}
	uid, rv := svc.UID, svc.ResourceVersion
	if err = services.Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &rv}}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	current, err := services.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.UID != uid || current.DeletionTimestamp == nil || !ownedUnallocatedService(current, namespace, releaseID) || len(current.Finalizers) != 1 || current.Finalizers[0] != "service.kubernetes.io/load-balancer-cleanup" {
		return nil
	}
	patch, _ := json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": uid}, {"op": "test", "path": "/metadata/resourceVersion", "value": current.ResourceVersion}, {"op": "replace", "path": "/metadata/finalizers", "value": []string{}}})
	_, err = services.Patch(ctx, name, types.JSONPatchType, patch, metav1.PatchOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func ownedUnallocatedService(svc *corev1.Service, namespace, releaseID string) bool {
	return svc.Namespace == namespace && svc.Annotations["meta.helm.sh/release-name"] == releaseID+"-platform" && svc.Annotations["meta.helm.sh/release-namespace"] == namespace && svc.Spec.Type == corev1.ServiceTypeLoadBalancer && svc.Spec.LoadBalancerClass == nil && svc.Spec.LoadBalancerIP == "" && len(svc.Spec.ExternalIPs) == 0 && len(svc.Status.LoadBalancer.Ingress) == 0
}
