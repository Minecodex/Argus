package collectormanager

import (
	"context"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

type ownedKubernetesDelete struct {
	get    func() (metav1.Object, error)
	remove func(metav1.DeleteOptions) error
	object metav1.Object
}

func (manager Manager) uninstallKubernetes(ctx context.Context, command *connectorv1.CollectorManagementCommand, client kubernetes.Interface) (Result, error) {
	namespace, err := client.CoreV1().Namespaces().Get(ctx, KubernetesNamespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return buildResult(command, "uninstalled"), nil
	}
	if err != nil {
		return Result{}, err
	}
	if namespace.Labels["app.kubernetes.io/part-of"] != "argus" {
		return Result{}, ErrInvalidCommand
	}
	lease, err := acquireKubernetesPublish(ctx, client, command.GetCollectorId(), command.GetDesiredRevision(), command.GetConfigSha256())
	if err != nil {
		return Result{}, err
	}
	defer lease.release()
	ctx = lease.ctx
	objects := []ownedKubernetesDelete{
		{get: func() (metav1.Object, error) {
			return client.CoreV1().PersistentVolumeClaims(KubernetesNamespace).Get(ctx, KubernetesGatewayStateClaim, metav1.GetOptions{})
		}, remove: func(options metav1.DeleteOptions) error {
			return client.CoreV1().PersistentVolumeClaims(KubernetesNamespace).Delete(ctx, KubernetesGatewayStateClaim, options)
		}},
		{get: func() (metav1.Object, error) {
			return client.AppsV1().DaemonSets(KubernetesNamespace).Get(ctx, "argus-otelcol-agent", metav1.GetOptions{})
		}, remove: func(options metav1.DeleteOptions) error {
			return client.AppsV1().DaemonSets(KubernetesNamespace).Delete(ctx, "argus-otelcol-agent", options)
		}},
		{get: func() (metav1.Object, error) {
			return client.AppsV1().Deployments(KubernetesNamespace).Get(ctx, "argus-otelcol-gateway", metav1.GetOptions{})
		}, remove: func(options metav1.DeleteOptions) error {
			return client.AppsV1().Deployments(KubernetesNamespace).Delete(ctx, "argus-otelcol-gateway", options)
		}},
		{get: func() (metav1.Object, error) {
			return client.BatchV1().Jobs(KubernetesNamespace).Get(ctx, collectorValidationJob, metav1.GetOptions{})
		}, remove: func(options metav1.DeleteOptions) error {
			return client.BatchV1().Jobs(KubernetesNamespace).Delete(ctx, collectorValidationJob, options)
		}},
		{get: func() (metav1.Object, error) {
			return client.CoreV1().Services(KubernetesNamespace).Get(ctx, "argus-otelcol-gateway", metav1.GetOptions{})
		}, remove: func(options metav1.DeleteOptions) error {
			return client.CoreV1().Services(KubernetesNamespace).Delete(ctx, "argus-otelcol-gateway", options)
		}},
		{get: func() (metav1.Object, error) {
			return client.CoreV1().ServiceAccounts(KubernetesNamespace).Get(ctx, "argus-otelcol", metav1.GetOptions{})
		}, remove: func(options metav1.DeleteOptions) error {
			return client.CoreV1().ServiceAccounts(KubernetesNamespace).Delete(ctx, "argus-otelcol", options)
		}},
		{get: func() (metav1.Object, error) {
			return client.RbacV1().Roles(KubernetesNamespace).Get(ctx, "argus-otelcol-identity", metav1.GetOptions{})
		}, remove: func(options metav1.DeleteOptions) error {
			return client.RbacV1().Roles(KubernetesNamespace).Delete(ctx, "argus-otelcol-identity", options)
		}},
		{get: func() (metav1.Object, error) {
			return client.RbacV1().RoleBindings(KubernetesNamespace).Get(ctx, "argus-otelcol-identity", metav1.GetOptions{})
		}, remove: func(options metav1.DeleteOptions) error {
			return client.RbacV1().RoleBindings(KubernetesNamespace).Delete(ctx, "argus-otelcol-identity", options)
		}},
	}
	for _, name := range []string{configSlotBase, configSlotNext} {
		objects = append(objects, ownedKubernetesDelete{get: func() (metav1.Object, error) {
			return client.CoreV1().ConfigMaps(KubernetesNamespace).Get(ctx, name, metav1.GetOptions{})
		}, remove: func(options metav1.DeleteOptions) error {
			return client.CoreV1().ConfigMaps(KubernetesNamespace).Delete(ctx, name, options)
		}})
	}
	for _, name := range []string{collectorIdentitySecret, secretSlotBase, secretSlotNext} {
		objects = append(objects, ownedKubernetesDelete{get: func() (metav1.Object, error) {
			return client.CoreV1().Secrets(KubernetesNamespace).Get(ctx, name, metav1.GetOptions{})
		}, remove: func(options metav1.DeleteOptions) error {
			return client.CoreV1().Secrets(KubernetesNamespace).Delete(ctx, name, options)
		}})
	}
	// Verify the complete deletion set before deleting the first object.
	for i := range objects {
		object, err := objects[i].get()
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return Result{}, err
		}
		if object.GetLabels()["argus.io/collector-id"] != command.GetCollectorId() {
			return Result{}, ErrInvalidCommand
		}
		objects[i].object = object
	}
	propagation := metav1.DeletePropagationForeground
	for _, item := range objects {
		if item.object != nil {
			uid := item.object.GetUID()
			if err := item.remove(metav1.DeleteOptions{PropagationPolicy: &propagation, Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
				return Result{}, err
			}
		}
	}
	if err = wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, 2*time.Minute, true, func(context.Context) (bool, error) {
		for _, item := range objects {
			if item.object == nil {
				continue
			}
			_, err := item.get()
			if err == nil {
				return false, nil
			}
			if !apierrors.IsNotFound(err) {
				return false, err
			}
		}
		return true, nil
	}); err != nil {
		return Result{}, err
	}
	if err = lease.finish("", command.GetDesiredRevision(), command.GetConfigSha256()); err != nil {
		return Result{}, err
	}
	return buildResult(command, "uninstalled"), nil
}
