package collectormanager

import (
	"context"
	"slices"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const KubernetesGatewayStateClaim = "argus-otelcol-gateway-state"

// The default StorageClass may bind immediately or wait for the Gateway's
// first consumer. Never fall back to EmptyDir when provisioning is unavailable.
func ensureKubernetesGatewayState(ctx context.Context, client kubernetes.Interface, collector string) error {
	claims := client.CoreV1().PersistentVolumeClaims(KubernetesNamespace)
	current, err := claims.Get(ctx, KubernetesGatewayStateClaim, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		desired := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: KubernetesGatewayStateClaim, Namespace: KubernetesNamespace, Labels: map[string]string{"app.kubernetes.io/part-of": "argus", "argus.io/collector-id": collector}}, Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}},
		}}
		current, err = claims.Create(ctx, desired, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(err) {
			current, err = claims.Get(ctx, KubernetesGatewayStateClaim, metav1.GetOptions{})
		}
	}
	if err != nil {
		return err
	}
	filesystem := current.Spec.VolumeMode == nil || *current.Spec.VolumeMode == corev1.PersistentVolumeFilesystem
	access := slices.Contains(current.Spec.AccessModes, corev1.ReadWriteOnce) || slices.Contains(current.Spec.AccessModes, corev1.ReadWriteOncePod)
	size := current.Spec.Resources.Requests[corev1.ResourceStorage]
	if current.Labels["argus.io/collector-id"] != collector || current.Labels["app.kubernetes.io/part-of"] != "argus" || current.DeletionTimestamp != nil || current.Status.Phase == corev1.ClaimLost || !filesystem || !access || size.Cmp(resource.MustParse("1Gi")) < 0 {
		return ErrInvalidCommand
	}
	return nil
}
