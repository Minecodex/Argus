package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) plan7TargetNamespace(ctx context.Context, env *E2EEnvironment) (string, error) {
	if name := env.State.Values["p7_target_namespace"]; name != "" {
		current, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil || string(current.UID) != env.State.Values["p7_target_namespace_uid"] || current.Labels["argus.io/release-id"] != env.ReleaseID || current.Labels["pod-security.kubernetes.io/enforce"] != "restricted" {
			return "", fmt.Errorf("Plan7 target namespace ownership changed")
		}
		return name, nil
	}
	name := kubernetesNameForDev(env.ReleaseID + "-target")
	ns, err := env.Kube.Client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{"argus.io/release-id": env.ReleaseID, "app.kubernetes.io/part-of": "argus-e2e", "pod-security.kubernetes.io/enforce": "restricted", "pod-security.kubernetes.io/enforce-version": "latest"}}}, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	env.State.Values["p7_target_namespace"], env.State.Values["p7_target_namespace_uid"] = name, string(ns.UID)
	env.ManagedNamespaces = append(env.ManagedNamespaces, name)
	raw, err := json.MarshalIndent(map[string]any{"run_id": env.Options.RunID, "namespace": name, "uid": ns.UID, "release_id": env.ReleaseID, "pod_security": "restricted", "separate_from_platform_namespace": true}, "", "  ")
	if err != nil {
		return "", err
	}
	return name, writePrivate(filepath.Join(env.Options.Artifacts, "plan7-kubernetes-ownership.json"), raw)
}
