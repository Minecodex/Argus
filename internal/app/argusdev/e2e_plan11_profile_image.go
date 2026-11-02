package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const plan11LegacyBaseCommit = "37f5fb4516de52d98ad7a39fef8e2b8fc869d1b3"

type plan11LegacyImage struct {
	ID     string `json:"Id"`
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
}

func (a *App) inspectPlan11LegacyImage(ctx context.Context, image string) (plan11LegacyImage, error) {
	var accepted plan11LegacyImage
	if !validPlan6Digest(image) {
		return accepted, fmt.Errorf("%w: baseline image must be a cached immutable sha256 image ID", errUsage)
	}
	raw, err := a.runner.Output(ctx, nil, "docker", "image", "inspect", image)
	if err != nil {
		return accepted, errors.New("comparison baseline image is not cached")
	}
	var rows []plan11LegacyImage
	if json.Unmarshal([]byte(raw), &rows) != nil || len(rows) != 1 || rows[0].ID != image || rows[0].Config.Labels["io.argus.plan11.profile"] != "legacy-discovery-common-reliability" || rows[0].Config.Labels["org.opencontainers.image.revision"] != plan11LegacyBaseCommit {
		return accepted, errors.New("comparison baseline immutable identity or declared profile mismatch")
	}
	return rows[0], nil
}

func (a *App) loadPlan11LegacyImage(ctx context.Context, env *E2EEnvironment) (string, error) {
	accepted, err := a.inspectPlan11LegacyImage(ctx, env.Options.Plan11BaselineImage)
	if err != nil {
		return "", err
	}
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return "", errors.New("comparison image import namespace ownership mismatch")
	}
	registry, err := e2eRegistry(env.ConfigPath)
	if err != nil {
		return "", err
	}
	_, port, ok := strings.Cut(registry, ":")
	if !ok {
		return "", errors.New("comparison requires the owned local registry")
	}
	local := "localhost:" + port + "/argus/argus-plan11-legacy:" + env.ImageTag
	if err = a.runner.Run(ctx, nil, "docker", "tag", accepted.ID, local); err != nil {
		return "", err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// Only the task-created registry tag is removed; the immutable cached
		// baseline ID still has its original user-visible baseline tag.
		_ = a.runner.Run(cleanup, nil, "docker", "image", "rm", local)
	}()
	if err = a.runner.Run(ctx, nil, "docker", "push", local); err != nil {
		return "", err
	}
	raw, err := a.runner.Output(ctx, nil, "docker", "buildx", "imagetools", "inspect", strings.Replace(local, "localhost:", "127.0.0.1:", 1), "--format", "{{json .Manifest}}")
	if err != nil {
		return "", err
	}
	var descriptor struct{ Digest string }
	if json.Unmarshal([]byte(raw), &descriptor) != nil || !validPlan6Digest(descriptor.Digest) {
		return "", errors.New("comparison baseline manifest invalid")
	}
	image := "host.docker.internal:" + port + "/argus/argus-plan11-legacy:" + env.ImageTag + "@" + descriptor.Digest
	loader := kubernetesNameForDev("argus-image-loader-" + env.ReleaseID)
	ds, err := env.Kube.Client.AppsV1().DaemonSets("kube-system").Get(ctx, loader, metav1.GetOptions{})
	if err != nil || ds.Labels["argus.io/release-id"] != env.ReleaseID {
		return "", errors.New("comparison image loader ownership mismatch")
	}
	pods, err := env.Kube.Client.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + loader})
	if err != nil || len(pods.Items) == 0 {
		return "", errors.New("comparison image loader unavailable")
	}
	for _, pod := range pods.Items {
		owned := false
		for _, owner := range pod.OwnerReferences {
			owned = owned || owner.UID == ds.UID
		}
		if !owned {
			return "", errors.New("comparison image loader pod identity mismatch")
		}
		base := []string{"--context", env.Options.KubeContext, "-n", "kube-system", "exec", pod.Name, "--", "/host/ctr", "--address", "/run/containerd/containerd.sock", "--namespace", "k8s.io", "images"}
		if err = a.runner.Run(ctx, nil, "kubectl", append(append([]string{}, base...), "pull", "--plain-http", image)...); err != nil {
			return "", err
		}
		repository := "host.docker.internal:" + port + "/argus/argus-plan11-legacy"
		if err = a.runner.Run(ctx, nil, "kubectl", append(append([]string{}, base...), "tag", "--force", image, repository+"@"+descriptor.Digest)...); err != nil {
			return "", err
		}
	}
	proof, _ := json.MarshalIndent(map[string]any{"source_commit": plan11LegacyBaseCommit, "profile": "legacy-discovery-common-reliability", "docker_image_id": accepted.ID, "runtime_image": image, "shared_components": []string{"current Query Engine/schema6", "current Collector", "current Workspace and offline evidence helper"}, "common_backports": []string{"pre-HTTP TLS retry", "exhaustion termination", "schema6 admission"}}, "", "  ")
	return image, writePrivate(filepath.Join(env.Options.Artifacts, "plan11-baseline-image.json"), proof)
}
