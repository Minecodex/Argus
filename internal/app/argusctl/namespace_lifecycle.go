package argusctl

import (
	"context"
	"slices"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func (cfg *InstallConfig) applicationNamespaces() []string {
	result := []string{}
	for _, name := range []string{cfg.Spec.Namespaces.System, cfg.Spec.Namespaces.Observability, cfg.Spec.Namespaces.Sandbox} {
		if name != "" && !slices.Contains(result, name) {
			result = append(result, name)
		}
	}
	return result
}

func deleteOwnedNamespace(ctx context.Context, client kubernetes.Interface, name, releaseID string) error {
	namespace, err := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if namespace.Labels["argus.io/release-id"] != releaseID || namespace.Labels["app.kubernetes.io/part-of"] != "argus" {
		return nil
	}
	err = client.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &namespace.UID, ResourceVersion: &namespace.ResourceVersion}})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// Keep the CSI driver alive until sandbox PVCs and PVs have been reclaimed.
// The platform namespace may also contain the driver, so it must be last.
func teardownNamespaces(ctx context.Context, client kubernetes.Interface, cfg *InstallConfig, removeStorage func() error) error {
	if err := deleteOwnedNamespace(ctx, client, cfg.Spec.Namespaces.Sandbox, cfg.Spec.ReleaseID); err != nil {
		return err
	}
	if err := removeStorage(); err != nil {
		return err
	}
	for _, name := range cfg.applicationNamespaces() {
		if name == cfg.Spec.Namespaces.Sandbox {
			continue
		}
		if err := deleteOwnedNamespace(ctx, client, name, cfg.Spec.ReleaseID); err != nil {
			return err
		}
	}
	return nil
}

func removeDedicatedWorkspaceNamespace(ctx context.Context, client kubernetes.Interface, cfg *InstallConfig, namespace *corev1.Namespace) error {
	if slices.Contains(cfg.applicationNamespaces(), namespace.Name) || namespace.Labels["argus.io/plane"] != "workspace-storage" {
		return nil
	}
	return deleteOwnedNamespace(ctx, client, namespace.Name, cfg.Spec.ReleaseID)
}
