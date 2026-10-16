package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type plan6ProductionWorkload struct {
	Namespace    string `json:"namespace"`
	NamespaceUID string `json:"namespace_uid"`
	Name         string `json:"name"`
	UID          string `json:"uid"`
}

func validatePlan6ProductionCoverage(env *E2EEnvironment, changed []plan6ProductionWorkload) error {
	covered := map[string]bool{}
	for _, workload := range changed {
		covered[workload.Namespace+"/"+workload.Name] = true
	}
	for _, group := range []struct {
		namespace string
		names     []string
	}{
		{env.SystemNS, []string{"argus-server", "argus-worker", "argus-connector-gateway", "argus-direct-executor", "argus-pki-controller"}},
		{env.ObservNS, []string{"argus-telemetry-ingest", "argus-telemetry-writer", "argus-telemetry-query"}},
	} {
		for _, name := range group.names {
			if !covered[group.namespace+"/"+name] {
				return fmt.Errorf("production switch did not cover %s/%s", group.namespace, name)
			}
		}
	}
	return nil
}

// Legacy prerequisites require the Replay test build. All Plan6 business
// acceptance and release candidates use the normal production backend.
func (a *App) usePlan6ProductionBackend(ctx context.Context, env *E2EEnvironment) error {
	hostReady := env.Options.Suite == "plan6" && env.Options.Plan6Phase == "host" && env.State.Values["host_prerequisites_ready"] == "true"
	if env.State.Values["p6_selfmonitor_ready"] != "true" && !hostReady {
		return fmt.Errorf("production switch requires the completed self-monitoring prerequisite")
	}
	prior, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "image-provenance.json"))
	if err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "image-provenance-prerequisites.json"), prior); err != nil {
		return err
	}
	var prerequisite struct {
		Images []plan6Image `json:"core_images"`
	}
	if err = json.Unmarshal(prior, &prerequisite); err != nil {
		return err
	}
	collectorImage := ""
	egressImage := ""
	for _, accepted := range prerequisite.Images {
		if accepted.Name == "otelcol" && validPlan6Digest(accepted.ImageID) {
			collectorImage = env.State.FixtureImages["otelcol"] + "@" + accepted.ImageID
		}
		if accepted.Name == "workspace-egress" && validPlan6Digest(accepted.ImageID) {
			registry, registryErr := e2eRegistry(env.ConfigPath)
			if registryErr != nil {
				return registryErr
			}
			egressImage = registry + "/argus/argus-workspace-egress:" + env.ImageTag + "@" + accepted.ImageID
		}
	}
	if collectorImage == "" || egressImage == "" {
		return fmt.Errorf("production switch lacks the accepted Collector manifest")
	}
	protocolTrust, err := a.preparePlan6ProtocolTrust(ctx, env)
	if err != nil {
		return err
	}
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("production switch namespace ownership is not established: %v", err)
	}
	registry, err := e2eRegistry(env.ConfigPath)
	if err != nil {
		return err
	}
	_, port, ok := strings.Cut(registry, ":")
	if !ok {
		return fmt.Errorf("production switch has no owned registry port")
	}
	local := "localhost:" + port + "/argus/argus-backend:" + env.ImageTag
	transport := strings.Replace(local, "localhost:", "127.0.0.1:", 1)
	args := []string{"buildx", "build", "--platform", env.ImagePlatform, "--file", "deploy/docker/backend.Dockerfile",
		"--tag", local, "--label", "io.argus.e2e.run=" + env.ImageTag, "--label", "io.argus.plan6.build=production",
		"--build-arg", "GO_BUILD_TAGS=", "--push", "."}
	if err = a.retryE2EImageBuild(ctx, "Plan6 production backend", func(buildCtx context.Context) error {
		return a.runner.Run(buildCtx, nil, "docker", args...)
	}); err != nil {
		return err
	}
	raw, err := a.runner.Output(ctx, nil, "docker", "buildx", "imagetools", "inspect", transport, "--format", "{{json .Manifest}}")
	if err != nil {
		return err
	}
	var descriptor struct {
		Digest string `json:"digest"`
	}
	if json.Unmarshal([]byte(raw), &descriptor) != nil || !strings.HasPrefix(descriptor.Digest, "sha256:") || len(descriptor.Digest) != 71 {
		return fmt.Errorf("production backend manifest is invalid")
	}
	image := env.State.FixtureImages["backend"] + "@" + descriptor.Digest
	loader := kubernetesNameForDev("argus-image-loader-" + env.ReleaseID)
	ds, err := env.Kube.Client.AppsV1().DaemonSets("kube-system").Get(ctx, loader, metav1.GetOptions{})
	if err != nil || ds.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("production image loader ownership is not established: %v", err)
	}
	pods, err := env.Kube.Client.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + loader})
	if err != nil || len(pods.Items) == 0 {
		return fmt.Errorf("production image loader has no pods: %v", err)
	}
	for _, pod := range pods.Items {
		owned := false
		for _, owner := range pod.OwnerReferences {
			owned = owned || owner.UID == ds.UID
		}
		if !owned {
			return fmt.Errorf("production image loader pod ownership mismatch")
		}
		for _, acceptedImage := range []string{image, collectorImage} {
			if err = a.runner.Run(ctx, nil, "kubectl", "--context", env.Options.KubeContext, "--namespace", "kube-system", "exec", pod.Name, "--",
				"/host/ctr", "--address", "/run/containerd/containerd.sock", "--namespace", "k8s.io", "images", "pull", "--plain-http", acceptedImage); err != nil {
				return err
			}
			// CRI canonicalizes tag@digest to repository@digest.
			base, digest, _ := strings.Cut(acceptedImage, "@")
			repository := strings.TrimSuffix(base, ":"+env.ImageTag)
			if err = a.runner.Run(ctx, nil, "kubectl", "--context", env.Options.KubeContext, "--namespace", "kube-system", "exec", pod.Name, "--",
				"/host/ctr", "--address", "/run/containerd/containerd.sock", "--namespace", "k8s.io", "images", "tag", "--force", acceptedImage, repository+"@"+digest); err != nil {
				return err
			}
		}
	}
	runtimeConfig, err := env.Kube.Client.CoreV1().ConfigMaps(env.SystemNS).Get(ctx, "argus-runtime-config", metav1.GetOptions{})
	if err != nil || runtimeConfig.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("production runtime ownership is not established: %v", err)
	}
	runtimeConfig.Data["ARGUS_WORKSPACE_IO_IMAGE"] = image
	runtimeConfig.Data["ARGUS_WORKSPACE_EGRESS_IMAGE"] = egressImage
	runtimeConfig.Data["ARGUS_CONNECTOR_KUBERNETES_IMAGE"] = image
	runtimeConfig.Data["ARGUS_TOOL_IMPLEMENTATION_REVISION"] = image
	if _, err = env.Kube.Client.CoreV1().ConfigMaps(env.SystemNS).Update(ctx, runtimeConfig, metav1.UpdateOptions{}); err != nil {
		return err
	}
	changed := []plan6ProductionWorkload{}
	visited := map[string]bool{}
	for _, namespace := range []string{env.SystemNS, env.SandboxNS, env.ObservNS} {
		if visited[namespace] {
			continue
		}
		visited[namespace] = true
		ownedNS, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil || ownedNS.Labels["argus.io/release-id"] != env.ReleaseID {
			return fmt.Errorf("production namespace %s ownership is not established: %v", namespace, err)
		}
		deployments, err := env.Kube.Client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{LabelSelector: "argus.io/release-id=" + env.ReleaseID})
		if err != nil {
			return err
		}
		for _, deployment := range deployments.Items {
			containers := []any{}
			trustBackend := deployment.Name == "argus-server" || deployment.Name == "argus-worker"
			for _, container := range deployment.Spec.Template.Spec.Containers {
				switch strings.SplitN(container.Image, "@", 2)[0] {
				case env.State.FixtureImages["backend"]:
					item := map[string]any{"name": container.Name, "image": image,
						"env": []any{map[string]any{"name": "ARGUS_E2E_QUERY_CHECKPOINTS", "value": "0"}}}
					if trustBackend {
						existing := ""
						for _, value := range container.Env {
							if value.Name == "SSL_CERT_DIR" {
								if value.ValueFrom != nil {
									return fmt.Errorf("protocol trust cannot replace an indirect SSL_CERT_DIR")
								}
								existing = value.Value
							}
						}
						item["env"] = append(item["env"].([]any), map[string]any{"name": "SSL_CERT_DIR", "value": plan6ProtocolCertDirectories(existing)})
						item["volumeMounts"] = []any{map[string]any{"name": "plan6-protocol-ca", "mountPath": plan6ProtocolTrustDirectory, "readOnly": true}}
					}
					containers = append(containers, item)
				case env.State.FixtureImages["otelcol"]:
					containers = append(containers, map[string]any{"name": container.Name, "image": collectorImage})
				}
			}
			if len(containers) == 0 {
				continue
			}
			if deployment.Name == "argus-server" {
				// The OTLP generator belongs to the test fixture, never to the
				// production binary. It shares the self-monitoring Collector's
				// network namespace without changing the real ingestion path.
				containers = append(containers, map[string]any{"name": "plan6-telemetry-fixture", "image": env.State.FixtureImages["systemd"],
					"imagePullPolicy": "Never", "command": []string{"/bin/sleep", "365d"},
					"securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": 65532, "runAsGroup": 65532}})
			}
			podSpec := map[string]any{"containers": containers}
			if trustBackend {
				podSpec["volumes"] = []any{map[string]any{"name": "plan6-protocol-ca", "configMap": map[string]any{"name": protocolTrust}}}
			}
			patch := map[string]any{"metadata": map[string]any{"uid": deployment.UID, "resourceVersion": deployment.ResourceVersion},
				"spec": map[string]any{"template": map[string]any{"spec": podSpec}}}
			if err = env.Kube.PatchDeployment(ctx, namespace, deployment.Name, patch); err != nil {
				return err
			}
			changed = append(changed, plan6ProductionWorkload{Namespace: namespace, NamespaceUID: string(ownedNS.UID), Name: deployment.Name, UID: string(deployment.UID)})
		}
	}
	if err = validatePlan6ProductionCoverage(env, changed); err != nil {
		return err
	}
	for _, workload := range changed {
		if err = env.Kube.WaitDeployment(ctx, workload.Namespace, workload.Name, 5*time.Minute); err != nil {
			return err
		}
	}
	if err = a.refreshPlan6ConnectorForward(ctx, env); err != nil {
		return err
	}
	env.State.Values["p6_backend_build"] = "production"
	report, _ := json.MarshalIndent(map[string]any{"run_id": env.Options.RunID, "namespace_uid": ns.UID, "runtime_config_uid": runtimeConfig.UID,
		"go_build_tags": "", "image": image, "digest": descriptor.Digest, "deployments": changed, "passed": true}, "", "  ")
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "plan6-production-backend.json"), report); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(a.stdout, "Plan6 normal production backend active; Replay prerequisites completed")
	return a.plan6Images(ctx, env, "")
}

// The identity scenario changes only its owned Host Collector through the real
// HTTP Preview/host confirmation path. Replay tool coverage belongs to M7,
// before the production switch; real alert model coverage remains mandatory.
func plan6HTTPCollectorFixture(env *E2EEnvironment, resourceType, resourceID, action string, prefixes []string) (bool, error) {
	if env.State.Values["p6_backend_build"] != "production" {
		return false, nil
	}
	if env.Options.Suite != "plan6" || env.State.Values["p6_selfmonitor_ready"] != "true" || resourceType != "host" ||
		resourceID != env.State.Values["p6_identity_host_id"] || resourceID == "" || len(prefixes) != 1 || prefixes[0] != "p6-identity" ||
		(action != "install" && action != "uninstall") {
		return false, fmt.Errorf("normal-backend Collector fixture requires the owned Plan6 identity Host")
	}
	return true, nil
}
