package argusdev

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CRI may report an older repoDigest for an identical image config. Only a
// pinned accepted root plus hash-verified node content can establish equality.
func (a *App) plan6PinnedRuntimeAlias(ctx context.Context, env *E2EEnvironment, node string, image plan6Image, configured, actual string) (string, bool, error) {
	_, pin, pinned := strings.Cut(configured, "@")
	if !pinned || pin != image.ImageID || !strings.Contains(actual, "@") {
		return "", false, nil
	}
	_, digest, _ := strings.Cut(actual, "@")
	if !validPlan6Digest(digest) {
		return "", false, nil
	}
	name := kubernetesNameForDev("argus-image-loader-" + env.ReleaseID)
	ds, err := env.Kube.Client.AppsV1().DaemonSets("kube-system").Get(ctx, name, metav1.GetOptions{})
	if err != nil || ds.Labels["argus.io/release-id"] != env.ReleaseID {
		return "", false, fmt.Errorf("node image identity loader ownership unavailable: %v", err)
	}
	pods, err := env.Kube.Client.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + name})
	if err != nil {
		return "", false, err
	}
	loader := ""
	for _, pod := range pods.Items {
		for _, owner := range pod.OwnerReferences {
			if pod.Spec.NodeName == node && owner.UID == ds.UID {
				loader = pod.Name
			}
		}
	}
	if loader == "" {
		return "", false, fmt.Errorf("owned image loader absent from accepted Pod's node")
	}
	manifest, config, err := plan6RegistryIdentity(digest, env.ImagePlatform, func(id string) ([]byte, error) {
		value, err := a.runner.Output(ctx, nil, "kubectl", "--context", env.Options.KubeContext, "-n", "kube-system", "exec", loader, "-c", "loader", "--", "/host/ctr", "--address", "/run/containerd/containerd.sock", "--namespace", "k8s.io", "content", "get", id)
		return []byte(value), err
	})
	if err != nil {
		return "", false, err
	}
	if manifest != image.PlatformManifestID || config != image.ConfigID {
		return "", false, nil
	}
	return "node_verified_equivalent_platform_manifest", true, nil
}
