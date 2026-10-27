package collectormanager

import (
	"context"
	"errors"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type kubernetesRuntimeSnapshot struct {
	gateway        *appsv1.Deployment
	agent          *appsv1.DaemonSet
	writtenGateway *appsv1.Deployment
	writtenAgent   *appsv1.DaemonSet
}

func snapshotKubernetesRuntime(ctx context.Context, client kubernetes.Interface, collector string) (kubernetesRuntimeSnapshot, error) {
	var out kubernetesRuntimeSnapshot
	var err error
	out.gateway, err = client.AppsV1().Deployments(KubernetesNamespace).Get(ctx, "argus-otelcol-gateway", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		out.gateway = nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return out, err
	}
	if out.gateway != nil && out.gateway.Labels["argus.io/collector-id"] != collector {
		return out, ErrInvalidCommand
	}
	out.agent, err = client.AppsV1().DaemonSets(KubernetesNamespace).Get(ctx, "argus-otelcol-agent", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		out.agent = nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return out, err
	}
	if out.agent != nil && out.agent.Labels["argus.io/collector-id"] != collector {
		return out, ErrInvalidCommand
	}
	return out, nil
}

func (snapshot kubernetesRuntimeSnapshot) currentSlot() string {
	if snapshot.gateway != nil {
		for _, volume := range snapshot.gateway.Spec.Template.Spec.Volumes {
			if volume.Name == "config" && volume.ConfigMap != nil {
				return volume.ConfigMap.Name
			}
		}
	}
	return ""
}

func (snapshot *kubernetesRuntimeSnapshot) apply(ctx context.Context, client kubernetes.Interface, gateway *appsv1.Deployment, agent *appsv1.DaemonSet) error {
	var err error
	if gateway != nil {
		if snapshot.gateway == nil {
			snapshot.writtenGateway, err = client.AppsV1().Deployments(KubernetesNamespace).Create(ctx, gateway, metav1.CreateOptions{})
		} else {
			current := snapshot.gateway.DeepCopy()
			current.Labels, current.Annotations = gateway.Labels, gateway.Annotations
			current.Spec.Replicas, current.Spec.Template = gateway.Spec.Replicas, gateway.Spec.Template
			current.Spec.Strategy, current.Spec.MinReadySeconds = gateway.Spec.Strategy, gateway.Spec.MinReadySeconds
			snapshot.writtenGateway, err = client.AppsV1().Deployments(KubernetesNamespace).Update(ctx, current, metav1.UpdateOptions{})
		}
	}
	if err != nil {
		return err
	}
	if agent != nil {
		if snapshot.agent == nil {
			snapshot.writtenAgent, err = client.AppsV1().DaemonSets(KubernetesNamespace).Create(ctx, agent, metav1.CreateOptions{})
		} else {
			current := snapshot.agent.DeepCopy()
			current.Labels, current.Annotations = agent.Labels, agent.Annotations
			current.Spec.Template = agent.Spec.Template
			snapshot.writtenAgent, err = client.AppsV1().DaemonSets(KubernetesNamespace).Update(ctx, current, metav1.UpdateOptions{})
		}
	}
	return err
}

func (snapshot kubernetesRuntimeSnapshot) rollback(ctx context.Context, client kubernetes.Interface) error {
	var failures []error
	if written := snapshot.writtenGateway; written != nil {
		current, err := client.AppsV1().Deployments(KubernetesNamespace).Get(ctx, written.Name, metav1.GetOptions{})
		if err == nil && (current.UID != written.UID || current.Generation != written.Generation) {
			err = ErrRollbackFailed
		}
		if err == nil {
			if snapshot.gateway == nil {
				uid := current.UID
				err = client.AppsV1().Deployments(KubernetesNamespace).Delete(ctx, current.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
			} else {
				current.Spec = snapshot.gateway.Spec
				current.Labels, current.Annotations = snapshot.gateway.Labels, snapshot.gateway.Annotations
				_, err = client.AppsV1().Deployments(KubernetesNamespace).Update(ctx, current, metav1.UpdateOptions{})
			}
		}
		failures = append(failures, err)
	}
	if written := snapshot.writtenAgent; written != nil {
		current, err := client.AppsV1().DaemonSets(KubernetesNamespace).Get(ctx, written.Name, metav1.GetOptions{})
		if err == nil && (current.UID != written.UID || current.Generation != written.Generation) {
			err = ErrRollbackFailed
		}
		if err == nil {
			if snapshot.agent == nil {
				uid := current.UID
				err = client.AppsV1().DaemonSets(KubernetesNamespace).Delete(ctx, current.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
			} else {
				current.Spec = snapshot.agent.Spec
				current.Labels, current.Annotations = snapshot.agent.Labels, snapshot.agent.Annotations
				_, err = client.AppsV1().DaemonSets(KubernetesNamespace).Update(ctx, current, metav1.UpdateOptions{})
			}
		}
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}
