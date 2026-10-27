package argusdev

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Importing an image archive can wrap the immutable platform manifest in a
// new OCI index. Verify the descriptor bytes rather than accepting its name.
func (a *App) plan8RawfileNodeImageIdentity(ctx context.Context, env *E2EEnvironment, node, actual string) (bool, error) {
	_, digest, hasDigest := strings.Cut(actual, "@")
	if !hasDigest || !validPlan6Digest(digest) || env.ImagePlatform != "linux/amd64" {
		return false, nil
	}
	cluster := strings.TrimPrefix(env.Options.KubeContext, "kind-")
	if !strings.HasPrefix(env.Options.KubeContext, "kind-plan8-") || !strings.HasPrefix(node, cluster+"-") {
		return false, errors.New("image proof requires the accepted Kind node")
	}
	if _, err := a.localE2EDockerNode(ctx, env, node); err != nil {
		return false, err
	}
	return plan8RawfileDescriptorIdentity(digest, env.ImagePlatform, func(blob string) ([]byte, error) {
		if !validPlan6Digest(blob) {
			return nil, errors.New("invalid node image content digest")
		}
		value, e := a.runner.Output(ctx, nil, "docker", "exec", node, "ctr", "--namespace", "k8s.io", "content", "get", blob)
		if len(value) > 4<<20 {
			return nil, errors.New("node image descriptor exceeds verification budget")
		}
		return []byte(value), e
	})
}

func plan8RawfileDescriptorIdentity(actual, platform string, fetch func(string) ([]byte, error)) (bool, error) {
	if platform != "linux/amd64" {
		return false, errors.New("RawFile runtime platform is not verified by this acceptance profile")
	}
	manifest, config, err := plan6RegistryIdentity(actual, platform, fetch)
	if err != nil {
		return false, err
	}
	// The selected manifest is itself frozen, even if the OCI index around it
	// was rewritten by a local archive import and includes an attestation.
	if manifest != plan8RawfileAMD64Digest {
		return false, nil
	}
	expected, expectedConfig, err := plan6RegistryIdentity(plan8RawfileIndexDigest, platform, fetch)
	if err != nil {
		return false, err
	}
	if expected != plan8RawfileAMD64Digest || expectedConfig != config {
		return false, fmt.Errorf("RawFile platform/config descriptor differs from the frozen upstream image")
	}
	return true, nil
}
