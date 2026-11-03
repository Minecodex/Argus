package argusdev

import (
	"context"
	"errors"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
)

type plan11DeploymentState struct {
	Namespace, Name string
	NamespaceUID    types.UID
	UID             types.UID
	Original        *appsv1.Deployment
	ExpectedImage   string
}

func plan11CaptureDeployment(ctx context.Context, env *E2EEnvironment, namespace, name string) (*plan11DeploymentState, error) {
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil || env.ReleaseID == "" || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return nil, errors.New("comparison namespace ownership mismatch")
	}
	d, err := env.Kube.Client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil || d.Labels["argus.io/release-id"] != env.ReleaseID || d.Spec.Replicas == nil || *d.Spec.Replicas != 1 {
		return nil, errors.New("comparison requires the owned single-replica evaluation deployment")
	}
	index := plan11ContainerIndex(d, name)
	if index < 0 {
		return nil, errors.New("comparison primary container missing")
	}
	return &plan11DeploymentState{Namespace: namespace, Name: name, NamespaceUID: ns.UID, UID: d.UID, Original: d.DeepCopy(), ExpectedImage: d.Spec.Template.Spec.Containers[index].Image}, nil
}

func plan11ContainerIndex(d *appsv1.Deployment, name string) int {
	for i, c := range d.Spec.Template.Spec.Containers {
		if c.Name == name {
			return i
		}
	}
	if len(d.Spec.Template.Spec.Containers) == 1 {
		return 0
	}
	return -1
}

// Namespace/deployment identity and optimistic resourceVersion checks apply to
// every mutation, including recovery. Do not overwrite an operator's new image.
func plan11EditDeployment(ctx context.Context, env *E2EEnvironment, s *plan11DeploymentState, edit func(*appsv1.Deployment, int) error) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, s.Namespace, metav1.GetOptions{})
		if err != nil || ns.UID != s.NamespaceUID || ns.Labels["argus.io/release-id"] != env.ReleaseID {
			return errors.New("comparison namespace identity changed")
		}
		d, err := env.Kube.Client.AppsV1().Deployments(s.Namespace).Get(ctx, s.Name, metav1.GetOptions{})
		if err != nil || d.UID != s.UID || d.Labels["argus.io/release-id"] != env.ReleaseID {
			return errors.New("comparison deployment identity changed")
		}
		index := plan11ContainerIndex(d, s.Name)
		if index < 0 || d.Spec.Template.Spec.Containers[index].Image != s.ExpectedImage {
			return errors.New("comparison image changed outside the profile transaction")
		}
		if err = edit(d, index); err != nil {
			return err
		}
		updated, err := env.Kube.Client.AppsV1().Deployments(s.Namespace).Update(ctx, d, metav1.UpdateOptions{})
		if err == nil {
			s.ExpectedImage = updated.Spec.Template.Spec.Containers[index].Image
		}
		return err
	})
}

func plan11SetReplicas(ctx context.Context, env *E2EEnvironment, s *plan11DeploymentState, replicas int32) error {
	if err := plan11EditDeployment(ctx, env, s, func(d *appsv1.Deployment, _ int) error {
		if d.Spec.Replicas == nil || (*d.Spec.Replicas != 0 && *d.Spec.Replicas != *s.Original.Spec.Replicas) {
			return errors.New("comparison replica state changed outside the transaction")
		}
		d.Spec.Replicas = &replicas
		return nil
	}); err != nil {
		return err
	}
	return env.Kube.WaitDeployment(ctx, s.Namespace, s.Name, 3*time.Minute)
}

func plan11SwitchBackend(ctx context.Context, env *E2EEnvironment, states []*plan11DeploymentState, image string, restoring bool) error {
	if len(states) != 2 || states[0].Name != "argus-server" || states[1].Name != "argus-worker" {
		return errors.New("comparison must switch Server and Worker together")
	}
	// Stop the scheduler first; no old and new execution pools may overlap.
	for _, s := range []*plan11DeploymentState{states[1], states[0]} {
		if err := plan11SetReplicas(ctx, env, s, 0); err != nil {
			return err
		}
	}
	for _, s := range states {
		if err := plan11EditDeployment(ctx, env, s, func(d *appsv1.Deployment, i int) error {
			if restoring {
				original := s.Original.Spec.Template.Spec.Containers[plan11ContainerIndex(s.Original, s.Name)]
				d.Spec.Template.Spec.Containers[i].Image = original.Image
				plan11CopyRevisionEnv(&d.Spec.Template.Spec.Containers[i], original.Env)
			} else {
				d.Spec.Template.Spec.Containers[i].Image = image
				plan11CopyRevisionEnv(&d.Spec.Template.Spec.Containers[i], []corev1.EnvVar{{Name: "ARGUS_TOOL_IMPLEMENTATION_REVISION", Value: image}})
			}
			return nil
		}); err != nil {
			return err
		}
	}
	for _, s := range states {
		if err := plan11SetReplicas(ctx, env, s, *s.Original.Spec.Replicas); err != nil {
			return err
		}
	}
	return nil
}

func plan11CopyRevisionEnv(container *corev1.Container, source []corev1.EnvVar) {
	out := []corev1.EnvVar{}
	for _, v := range container.Env {
		if v.Name != "ARGUS_TOOL_IMPLEMENTATION_REVISION" {
			out = append(out, v)
		}
	}
	for _, v := range source {
		if v.Name == "ARGUS_TOOL_IMPLEMENTATION_REVISION" {
			out = append(out, v)
		}
	}
	container.Env = out
}

func plan11RecoveryContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 8*time.Minute)
}

func plan11ProfileDescription(states []*plan11DeploymentState) []map[string]any {
	out := []map[string]any{}
	for _, s := range states {
		out = append(out, map[string]any{"namespace": s.Namespace, "namespace_uid": s.NamespaceUID, "name": s.Name, "uid": s.UID, "image": s.ExpectedImage, "original_image": s.Original.Spec.Template.Spec.Containers[plan11ContainerIndex(s.Original, s.Name)].Image})
	}
	return out
}

func plan11ProfileRecoveryError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("comparison profile recovery failed: %w", err)
}
