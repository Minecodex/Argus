package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type pausedWorkload struct {
	Namespace, Name, Kind string
	Replicas              int32
	Applied               bool
}

func (a *App) pauseFormalWorkloads(ctx context.Context, env *E2EEnvironment) error {
	namespaces, err := env.Kube.Client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/part-of=argus"})
	if err != nil {
		return err
	}
	for _, namespace := range namespaces.Items {
		owner := namespace.Labels["argus.io/release-id"]
		if owner == "" || owner == env.ReleaseID || namespace.Labels["argus.io/plane"] == "workspace-storage" {
			continue
		}
		deployments, err := env.Kube.Client.AppsV1().Deployments(namespace.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, item := range deployments.Items {
			if item.Spec.Replicas != nil && *item.Spec.Replicas > 0 {
				env.Paused = append(env.Paused, pausedWorkload{Namespace: namespace.Name, Name: item.Name, Kind: "Deployment", Replicas: *item.Spec.Replicas})
			}
		}
		sets, err := env.Kube.Client.AppsV1().StatefulSets(namespace.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, item := range sets.Items {
			if item.Spec.Replicas != nil && *item.Spec.Replicas > 0 {
				env.Paused = append(env.Paused, pausedWorkload{Namespace: namespace.Name, Name: item.Name, Kind: "StatefulSet", Replicas: *item.Spec.Replicas})
			}
		}
	}
	data, _ := json.MarshalIndent(env.Paused, "", "  ")
	if err := writePrivate(filepath.Join(env.Options.Artifacts, "original-replicas.json"), data); err != nil {
		return err
	}
	for index, item := range env.Paused {
		var err error
		if item.Kind == "Deployment" {
			err = env.Kube.ScaleDeployment(ctx, item.Namespace, item.Name, 0)
		} else {
			err = env.Kube.ScaleStatefulSet(ctx, item.Namespace, item.Name, 0)
		}
		if err != nil {
			return err
		}
		env.Paused[index].Applied = true
	}
	return nil
}
func restoreFormalWorkloads(ctx context.Context, env *E2EEnvironment) error {
	var failures []error
	var waiting []pausedWorkload
	for _, item := range env.Paused {
		if !item.Applied {
			continue
		}
		// A concurrent operator/user update must not be overwritten on cleanup.
		patch, _ := json.Marshal([]map[string]any{{"op": "test", "path": "/spec/replicas", "value": 0}, {"op": "replace", "path": "/spec/replicas", "value": item.Replicas}})
		var err error
		if item.Kind == "Deployment" {
			_, err = env.Kube.Client.AppsV1().Deployments(item.Namespace).Patch(ctx, item.Name, types.JSONPatchType, patch, metav1.PatchOptions{})
		} else {
			_, err = env.Kube.Client.AppsV1().StatefulSets(item.Namespace).Patch(ctx, item.Name, types.JSONPatchType, patch, metav1.PatchOptions{})
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("restore %s/%s: %w", item.Namespace, item.Name, err))
			continue
		}
		if item.Kind == "Deployment" {
			waiting = append(waiting, item)
		}
	}
	// Restore all replica counts before waiting: applications need PostgreSQL,
	// Kafka and ClickHouse, which may appear after them in the recorded list.
	for _, item := range waiting {
		if err := env.Kube.WaitDeployment(ctx, item.Namespace, item.Name, 5*time.Minute); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
