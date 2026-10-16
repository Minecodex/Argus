package argusdev

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type plan6Manifest struct {
	Config struct {
		Digest string `json:"digest"`
	} `json:"config"`
	Manifests []struct {
		Digest   string `json:"digest"`
		Platform struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
		} `json:"platform"`
	} `json:"manifests"`
}

func validPlan6Digest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

// Resolve the exact root -> platform manifest -> config relationship. A CRI
// config ID is accepted only with an explicitly pinned root and this proof.
func plan6RegistryIdentity(root, platform string, fetch func(string) ([]byte, error)) (string, string, error) {
	parts := strings.Split(platform, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("unsupported accepted platform %q", platform)
	}
	get := func(digest string) (plan6Manifest, error) {
		var manifest plan6Manifest
		if !validPlan6Digest(digest) {
			return manifest, fmt.Errorf("invalid registry digest %q", digest)
		}
		raw, err := fetch(digest)
		if err != nil {
			return manifest, err
		}
		hash := sha256.Sum256(raw)
		if "sha256:"+hex.EncodeToString(hash[:]) != digest {
			return manifest, fmt.Errorf("registry content differs from immutable digest %s", digest)
		}
		return manifest, json.Unmarshal(raw, &manifest)
	}
	manifest, err := get(root)
	if err != nil {
		return "", "", err
	}
	selected := root
	if len(manifest.Manifests) > 0 {
		selected = ""
		for _, member := range manifest.Manifests {
			if member.Platform.OS == parts[0] && member.Platform.Architecture == parts[1] {
				if selected != "" {
					return "", "", fmt.Errorf("ambiguous accepted platform %s", platform)
				}
				selected = member.Digest
			}
		}
		if selected == "" {
			return "", "", fmt.Errorf("accepted platform %s missing", platform)
		}
		manifest, err = get(selected)
		if err != nil {
			return "", "", err
		}
	}
	if !validPlan6Digest(manifest.Config.Digest) {
		return "", "", fmt.Errorf("accepted image config digest missing")
	}
	return selected, manifest.Config.Digest, nil
}

func (a *App) plan6ImageIdentity(ctx context.Context, port, repository, root, platform string) (string, string, error) {
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	return plan6RegistryIdentity(root, platform, func(digest string) ([]byte, error) {
		endpoint := "http://127.0.0.1:" + port + "/v2/" + escapeRepository(repository) + "/manifests/" + url.PathEscape(digest)
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept", registryManifestAccept)
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("immutable image manifest returned %s", response.Status)
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
		if len(raw) > 4<<20 {
			return nil, fmt.Errorf("image manifest exceeds limit")
		}
		return raw, err
	})
}

func plan6RuntimeImageIdentity(image plan6Image, configured, actual string) (string, bool) {
	_, pinned, hasPin := strings.Cut(configured, "@")
	if hasPin && pinned != image.ImageID {
		return "", false
	}
	id := actual
	if index := strings.LastIndex(id, "@"); index >= 0 {
		id = id[index+1:]
	}
	if id == image.ImageID && validPlan6Digest(id) {
		return "registry_root", true
	}
	if !hasPin || !validPlan6Digest(id) {
		return "", false
	}
	if id == image.PlatformManifestID {
		return "platform_manifest", true
	}
	if id == image.ConfigID {
		return "image_config", true
	}
	return "", false
}

func plan6CandidateManifestDigest(raw []byte) (string, error) {
	var metadata struct {
		Digest     string `json:"containerimage.digest"`
		Descriptor struct {
			Digest string `json:"digest"`
		} `json:"containerimage.descriptor"`
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return "", err
	}
	digest := metadata.Descriptor.Digest
	if digest == "" {
		digest = metadata.Digest
	} else if metadata.Digest != "" && metadata.Digest != digest {
		return "", fmt.Errorf("candidate metadata has conflicting manifest digests")
	}
	if !validPlan6Digest(digest) {
		return "", fmt.Errorf("candidate metadata omitted its immutable manifest digest")
	}
	return digest, nil
}
