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
	NamespaceUID, UID     types.UID
	ReleaseID             string
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
				env.Paused = append(env.Paused, pausedWorkload{Namespace: namespace.Name, NamespaceUID: namespace.UID, ReleaseID: owner, UID: item.UID, Name: item.Name, Kind: "Deployment", Replicas: *item.Spec.Replicas})
			}
		}
		sets, err := env.Kube.Client.AppsV1().StatefulSets(namespace.Name).List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}
		for _, item := range sets.Items {
			if item.Spec.Replicas != nil && *item.Spec.Replicas > 0 {
				env.Paused = append(env.Paused, pausedWorkload{Namespace: namespace.Name, NamespaceUID: namespace.UID, ReleaseID: owner, UID: item.UID, Name: item.Name, Kind: "StatefulSet", Replicas: *item.Spec.Replicas})
			}
		}
	}
	data, _ := json.MarshalIndent(env.Paused, "", "  ")
	if err := writePrivate(filepath.Join(env.Options.Artifacts, "original-replicas.json"), data); err != nil {
		return err
	}
	for index, item := range env.Paused {
		if err := pausedNamespaceCurrent(ctx, env, item); err != nil {
			return err
		}
		patch := pausedReplicaPatch(item, item.Replicas, 0)
		var err error
		if item.Kind == "Deployment" {
			_, err = env.Kube.Client.AppsV1().Deployments(item.Namespace).Patch(ctx, item.Name, types.JSONPatchType, patch, metav1.PatchOptions{})
		} else {
			_, err = env.Kube.Client.AppsV1().StatefulSets(item.Namespace).Patch(ctx, item.Name, types.JSONPatchType, patch, metav1.PatchOptions{})
		}
		if err != nil {
			return err
		}
		env.Paused[index].Applied = true
		data, _ := json.MarshalIndent(env.Paused, "", "  ")
		if err = writePrivate(filepath.Join(env.Options.Artifacts, "original-replicas.json"), data); err != nil {
			return err
		}
	}
	return nil
}
func restoreFormalWorkloads(ctx context.Context, env *E2EEnvironment) error {
	var failures []error
	var waiting []pausedWorkload
	for index, item := range env.Paused {
		if !item.Applied {
			continue
		}
		if err := pausedNamespaceCurrent(ctx, env, item); err != nil {
			failures = append(failures, err)
			continue
		}
		// A concurrent operator/user update must not be overwritten on cleanup.
		patch := pausedReplicaPatch(item, 0, item.Replicas)
		alreadyRestored, err := pausedWorkloadRestored(ctx, env, item)
		if err == nil && !alreadyRestored {
			if item.Kind == "Deployment" {
				_, err = env.Kube.Client.AppsV1().Deployments(item.Namespace).Patch(ctx, item.Name, types.JSONPatchType, patch, metav1.PatchOptions{})
			} else {
				_, err = env.Kube.Client.AppsV1().StatefulSets(item.Namespace).Patch(ctx, item.Name, types.JSONPatchType, patch, metav1.PatchOptions{})
			}
			if err != nil {
				// An operator can restore the exact desired state between Get and
				// Patch. Accept only that same identity and replica count.
				if restored, checkErr := pausedWorkloadRestored(ctx, env, item); checkErr == nil && restored {
					err = nil
				}
			}
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("restore %s/%s: %w", item.Namespace, item.Name, err))
			continue
		}
		env.Paused[index].Applied = false
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

func pausedWorkloadRestored(ctx context.Context, env *E2EEnvironment, item pausedWorkload) (bool, error) {
	if err := pausedNamespaceCurrent(ctx, env, item); err != nil {
		return false, err
	}
	var uid types.UID
	var replicas *int32
	if item.Kind == "Deployment" {
		current, err := env.Kube.Client.AppsV1().Deployments(item.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		uid, replicas = current.UID, current.Spec.Replicas
	} else {
		current, err := env.Kube.Client.AppsV1().StatefulSets(item.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		uid, replicas = current.UID, current.Spec.Replicas
	}
	if uid != item.UID || replicas == nil {
		return false, fmt.Errorf("paused workload identity changed: %s/%s", item.Namespace, item.Name)
	}
	return *replicas == item.Replicas, nil
}

func pausedReplicaPatch(item pausedWorkload, from, to int32) []byte {
	patch, _ := json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": item.UID}, {"op": "test", "path": "/spec/replicas", "value": from}, {"op": "replace", "path": "/spec/replicas", "value": to}})
	return patch
}

func pausedNamespaceCurrent(ctx context.Context, env *E2EEnvironment, item pausedWorkload) error {
	current, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, item.Namespace, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if item.UID == "" || item.NamespaceUID == "" || current.UID != item.NamespaceUID || item.ReleaseID == "" || current.Labels["argus.io/release-id"] != item.ReleaseID {
		return fmt.Errorf("paused workload ownership changed: %s/%s", item.Namespace, item.Name)
	}
	return nil
}
