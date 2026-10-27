package argusdev

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) plan6Source(ctx context.Context, env *E2EEnvironment, stage string) (string, error) {
	listed, err := a.runner.Output(ctx, nil, "git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return "", err
	}
	files, locks := map[string]string{}, map[string]string{}
	lockNames := map[string]bool{"go.mod": true, "go.sum": true, "pnpm-lock.yaml": true, "pnpm-workspace.yaml": true, "package.json": true}
	for _, name := range strings.Split(listed, "\x00") {
		if name == "" {
			continue
		}
		include := lockNames[name]
		for _, prefix := range []string{"api/", "cmd/", "internal/", "migrations/", "scripts/", "web/", "deploy/", "tests/"} {
			include = include || strings.HasPrefix(name, prefix)
		}
		if !include {
			continue
		}
		data, err := os.ReadFile(filepath.Join(env.Root, filepath.FromSlash(name)))
		if os.IsNotExist(err) {
			files[name] = "deleted"
			continue
		}
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256(data)
		files[name] = hex.EncodeToString(digest[:])
		if lockNames[name] {
			locks[name] = files[name]
		}
	}
	if len(files) == 0 || len(locks) < 4 {
		return "", fmt.Errorf("Plan6 source/dependency snapshot is incomplete")
	}
	canonical, _ := json.Marshal(files)
	digest := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(digest[:])
	head, err := a.runner.Output(ctx, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	status, err := a.runner.Output(ctx, nil, "git", "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return "", err
	}
	report, _ := json.MarshalIndent(map[string]any{"schema_version": "argus.plan6_source/v1", "run_id": env.Options.RunID, "stage": stage, "captured_at": time.Now().UTC(), "head": strings.TrimSpace(head), "fingerprint": fingerprint, "uncommitted_status": strings.Split(strings.TrimSpace(status), "\n"), "dependency_locks": locks, "source_files": files}, "", "  ")
	return fingerprint, writePrivate(filepath.Join(env.Options.Artifacts, "source-"+stage+".json"), report)
}

type plan6Image struct {
	Name               string `json:"name"`
	Source             string `json:"source"`
	ImageID            string `json:"image_id"`
	PlatformManifestID string `json:"platform_manifest_digest"`
	ConfigID           string `json:"config_digest"`
	Candidate          string `json:"candidate,omitempty"`
}

func plan6PodRequiresImageVerification(pod corev1.Pod) bool {
	return pod.DeletionTimestamp == nil && pod.Status.Phase == corev1.PodRunning
}

func (a *App) plan6Images(ctx context.Context, env *E2EEnvironment, candidateRegistry string) error {
	if candidateRegistry != "" && env.State.Values["p6_backend_build"] != "production" {
		return fmt.Errorf("Plan6 release candidates require the accepted production backend")
	}
	registry, err := e2eRegistry(env.ConfigPath)
	if err != nil {
		return err
	}
	_, port, ok := strings.Cut(registry, ":")
	if !ok {
		return fmt.Errorf("missing owned registry port")
	}
	refs := make(map[string]string, len(env.State.FixtureImages)+2)
	for name, ref := range env.State.FixtureImages {
		refs[name] = ref
	}
	// Workspace and egress are built and loaded with the same install group.
	// Preserve them too so formal sessions do not retain the old image group.
	refs["workspace"] = registry + "/argus/argus-workspace:" + env.ImageTag
	refs["workspace-egress"] = registry + "/argus/argus-workspace-egress:" + env.ImageTag
	refs["install-tools"] = registry + "/argus/argus-install-tools:" + env.ImageTag
	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	sort.Strings(names)
	images := []plan6Image{}
	for _, name := range names {
		if name != "backend" && name != "web" && name != "minio" && name != "workspace" && name != "workspace-egress" && name != "otelcol" && name != "remote-runtime" && name != "install-tools" {
			continue
		}
		ref := refs[name]
		parts := strings.Split(ref, "/")
		local := "localhost:" + port + "/argus/" + parts[len(parts)-1]
		transport := strings.Replace(local, "localhost:", "127.0.0.1:", 1)
		output, e := a.runner.Output(ctx, nil, "docker", "buildx", "imagetools", "inspect", transport, "--format", "{{json .Manifest}}")
		if e != nil {
			return e
		}
		var descriptor struct {
			Digest string `json:"digest"`
		}
		if e = json.Unmarshal([]byte(output), &descriptor); e != nil {
			return e
		}
		if !strings.HasPrefix(descriptor.Digest, "sha256:") || len(descriptor.Digest) != 71 {
			return fmt.Errorf("immutable registry manifest missing for %s", name)
		}
		if _, e = hex.DecodeString(strings.TrimPrefix(descriptor.Digest, "sha256:")); e != nil {
			return e
		}
		repository := local[:strings.LastIndex(local, ":")]
		source := repository + "@" + descriptor.Digest
		image := plan6Image{Name: name, Source: source, ImageID: descriptor.Digest}
		image.PlatformManifestID, image.ConfigID, e = a.plan6ImageIdentity(ctx, port, "argus/"+strings.SplitN(parts[len(parts)-1], ":", 2)[0], image.ImageID, env.ImagePlatform)
		if e != nil {
			return e
		}
		if candidateRegistry != "" {
			image.Candidate = strings.TrimRight(candidateRegistry, "/") + "/" + parts[len(parts)-1]
			meta := filepath.Join(env.Options.Artifacts, "candidate-"+name+".json")
			// Docker Desktop's published local registry is reachable on IPv4;
			// localhost can select an IPv6 listener that resets this request.
			// Retain the accepted source reference and compare the same digest.
			copySource := strings.Replace(source, "localhost:", "127.0.0.1:", 1)
			if e = a.retryE2EImageBuild(ctx, "Plan6 immutable candidate "+name, func(copyCtx context.Context) error {
				return a.runner.Run(copyCtx, nil, "docker", "buildx", "imagetools", "create", "--prefer-index=false", "--tag", image.Candidate, "--metadata-file", meta, copySource)
			}); e != nil {
				return e
			}
			data, e := os.ReadFile(meta)
			if e != nil {
				return e
			}
			candidateDigest, e := plan6CandidateManifestDigest(data)
			if e != nil {
				return e
			}
			_, digest, _ := strings.Cut(source, "@")
			if candidateDigest != digest {
				return fmt.Errorf("candidate %s changed the accepted manifest digest", name)
			}
			// Independently verify the actual target bytes and platform/config
			// descriptors, rather than trusting metadata alone.
			targetRegistry, targetPath, _ := strings.Cut(image.Candidate, "/")
			_, targetPort, _ := strings.Cut(targetRegistry, ":")
			targetRepository := targetPath[:strings.LastIndex(targetPath, ":")]
			manifestID, configID, e := a.plan6ImageIdentity(ctx, targetPort, targetRepository, digest, env.ImagePlatform)
			if e != nil || manifestID != image.PlatformManifestID || configID != image.ConfigID {
				return fmt.Errorf("candidate %s target differs from accepted descriptor chain: %v", name, e)
			}
			image.Candidate += "@" + digest
		}
		images = append(images, image)
	}
	if err = validateE2ECoreImageSet(images, env.Options.Suite == "plan7" || env.Options.Suite == "plan8" || env.State.FixtureImages["remote-runtime"] != ""); err != nil {
		return err
	}
	pods := []map[string]any{}
	for _, namespace := range []string{env.SystemNS, env.SandboxNS, env.ObservNS} {
		ns, e := env.Kube.Client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if e != nil {
			return e
		}
		list, e := env.Kube.Client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if e != nil {
			return e
		}
		for _, pod := range list.Items {
			containers := []map[string]any{}
			for _, c := range pod.Status.ContainerStatuses {
				configured := c.Image
				for _, spec := range pod.Spec.Containers {
					if spec.Name == c.Name {
						configured = spec.Image
					}
				}
				runtimeIdentity := "dependency"
				for _, image := range images {
					matches := strings.SplitN(configured, "@", 2)[0] == refs[image.Name]
					identity, accepted := plan6RuntimeImageIdentity(image, configured, c.ImageID)
					if plan6PodRequiresImageVerification(pod) && matches && !accepted {
						identity, accepted, e = a.plan6PinnedRuntimeAlias(ctx, env, pod.Spec.NodeName, image, configured, c.ImageID)
						if e != nil {
							return e
						}
					}
					if matches {
						runtimeIdentity = identity
					}
					if plan6PodRequiresImageVerification(pod) && matches && !accepted {
						return fmt.Errorf("accepted pod %s/%s uid=%s image_id=%s differs from registry manifest %s for %s", namespace, pod.Name, pod.UID, c.ImageID, image.ImageID, image.Name)
					}
				}
				containers = append(containers, map[string]any{"name": c.Name, "image": c.Image, "configured_image": configured, "image_id": c.ImageID, "runtime_identity": runtimeIdentity})
			}
			pods = append(pods, map[string]any{"namespace": namespace, "namespace_uid": ns.UID, "pod": pod.Name, "pod_uid": pod.UID, "phase": pod.Status.Phase, "deletion_timestamp": pod.DeletionTimestamp, "containers": containers})
		}
	}
	report, _ := json.MarshalIndent(map[string]any{"run_id": env.Options.RunID, "source_fingerprint": env.State.Values["p6_source_fingerprint"], "core_images": images, "deployed_dependencies": pods, "candidate_registry": candidateRegistry}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "image-provenance.json"), report)
}
