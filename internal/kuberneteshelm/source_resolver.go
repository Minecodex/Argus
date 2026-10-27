package kuberneteshelm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"helm.sh/helm/v4/pkg/registry"
	repo "helm.sh/helm/v4/pkg/repo/v1"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/errcode"
	"sigs.k8s.io/yaml"
)

func archiveDigest(raw []byte) string {
	h := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(h[:])
}

var sourceSlots = make(chan struct{}, 2)

func (e Engine) resolveChart(ctx context.Context, s ChartSource, credentials *SourceCredentials) ([]byte, *ChartProvenance, error) {
	select {
	case sourceSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ErrSourceUnavailable
	}
	defer func() { <-sourceSlots }()
	if ValidateChartSource(&s) != nil || ValidateSourceCredentials(credentials) != nil {
		return nil, nil, ErrInvalid
	}
	var c SourceCredentials
	if credentials != nil {
		c = *credentials
	}
	defer c.Clear()
	n, err := newSourceSession(s, c, e.SourcePolicy)
	if err != nil {
		return nil, nil, err
	}
	defer n.transport.CloseIdleConnections()
	call, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var raw []byte
	var resolved string
	if s.Kind == "repository" {
		raw, resolved, err = resolveRepository(call, n, s, c)
	} else {
		raw, resolved, err = resolveOCI(call, n, s, c)
	}
	if err != nil {
		clear(raw)
		return nil, nil, sourceError(err)
	}
	ch, err := loadChart(raw)
	if err != nil {
		clear(raw)
		return nil, nil, err
	}
	name := s.Chart
	if name == "" {
		name = s.URL[strings.LastIndex(s.URL, "/")+1:]
	}
	if ch.Metadata.Name != name || ch.Metadata.Version != s.Version || s.Digest != "" && s.Digest != resolved {
		clear(raw)
		return nil, nil, ErrSourceIntegrity
	}
	return raw, &ChartProvenance{Kind: s.Kind, Origin: s.URL, Transport: n.origin.Scheme, AuthURL: s.AuthURL, Chart: ch.Metadata.Name, Version: ch.Metadata.Version, Digest: resolved, ArchiveDigest: archiveDigest(raw)}, nil
}

func sourceGet(ctx context.Context, n *sourceSession, u *url.URL, c SourceCredentials, limit int64) ([]byte, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, ErrInvalid
	}
	if sourceOrigin(u) == sourceOrigin(n.origin) {
		if c.Username != "" {
			r.SetBasicAuth(c.Username, c.Password)
		} else if c.Token != "" {
			r.Header.Set("Authorization", "Bearer "+c.Token)
		}
	}
	response, err := n.client.Do(r)
	if err != nil {
		return nil, sourceError(err)
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return nil, ErrSourceAuth
	}
	if response.StatusCode != 200 {
		return nil, ErrSourceUnavailable
	}
	if response.ContentLength > limit {
		return nil, ErrBudget
	}
	return limitedRead(response.Body, int(limit))
}

func resolveRepository(ctx context.Context, n *sourceSession, s ChartSource, c SourceCredentials) ([]byte, string, error) {
	indexURL := *n.origin
	indexURL.Path = strings.TrimSuffix(indexURL.Path, "/") + "/index.yaml"
	raw, err := sourceGet(ctx, n, &indexURL, c, 2<<20)
	if err != nil {
		return nil, "", err
	}
	var index repo.IndexFile
	if yaml.UnmarshalStrict(raw, &index) != nil || index.APIVersion != "v1" || len(index.Entries) > 10000 {
		return nil, "", ErrInvalid
	}
	// Exact versions only: no floating tags or semantic-version constraints.
	var entry *repo.ChartVersion
	for _, candidate := range index.Entries[s.Chart] {
		if candidate != nil && candidate.Metadata != nil && candidate.Name == s.Chart && candidate.Version == s.Version {
			if entry != nil {
				return nil, "", ErrSourceIntegrity
			}
			entry = candidate
		}
	}
	if entry == nil || len(entry.URLs) == 0 || len(entry.URLs) > 8 {
		return nil, "", ErrInvalid
	}
	reference, err := url.Parse(entry.URLs[0])
	if err != nil {
		return nil, "", ErrInvalid
	}
	chartURL := indexURL.ResolveReference(reference)
	if _, err = sourceURL(chartURL.String(), s.PlainHTTP); err != nil {
		return nil, "", err
	}
	data, err := sourceGet(ctx, n, chartURL, c, MaxArchiveBytes)
	if err != nil {
		return nil, "", err
	}
	digest := archiveDigest(data)
	if entry.Digest != "" && "sha256:"+strings.TrimPrefix(entry.Digest, "sha256:") != digest {
		clear(data)
		return nil, "", ErrSourceIntegrity
	}
	return data, digest, nil
}

func resolveOCI(ctx context.Context, n *sourceSession, s ChartSource, c SourceCredentials) ([]byte, string, error) {
	ref := strings.TrimPrefix(s.URL, "oci://")
	r, err := remote.NewRepository(ref)
	if err != nil {
		return nil, "", ErrInvalid
	}
	r.PlainHTTP = s.PlainHTTP
	r.Client = &auth.Client{Client: n.client, Cache: auth.NewCache(), Credential: func(_ context.Context, host string) (auth.Credential, error) {
		if host != n.origin.Host {
			return auth.EmptyCredential, nil
		}
		return auth.Credential{Username: c.Username, Password: c.Password, AccessToken: c.Token}, nil
	}}
	tag := strings.ReplaceAll(s.Version, "+", "_")
	if s.Digest != "" {
		tag = s.Digest
	}
	descriptor, err := r.Resolve(ctx, tag)
	if err != nil {
		return nil, "", err
	}
	if descriptor.MediaType != ocispec.MediaTypeImageManifest || descriptor.Size < 1 || descriptor.Size > 128<<10 {
		return nil, "", ErrInvalid
	}
	manifestBytes, err := ociBytes(ctx, r, descriptor, 128<<10)
	if err != nil {
		return nil, "", err
	}
	var manifest ocispec.Manifest
	if json.Unmarshal(manifestBytes, &manifest) != nil || manifest.SchemaVersion != 2 || manifest.Config.MediaType != registry.ConfigMediaType || len(manifest.Layers) < 1 || len(manifest.Layers) > 3 || len(manifest.Config.URLs) > 0 {
		return nil, "", ErrInvalid
	}
	config, err := ociBytes(ctx, r, manifest.Config, 96<<10)
	if err != nil {
		return nil, "", err
	}
	var metadata struct{ Name, Version string }
	if json.Unmarshal(config, &metadata) != nil || metadata.Version != s.Version {
		return nil, "", ErrSourceIntegrity
	}
	var chart *ocispec.Descriptor
	for _, layer := range manifest.Layers {
		if len(layer.URLs) > 0 {
			return nil, "", ErrDenied
		}
		if layer.MediaType == registry.ChartLayerMediaType {
			if chart != nil {
				return nil, "", ErrSourceIntegrity
			}
			copy := layer
			chart = &copy
		} else if layer.MediaType != registry.ProvLayerMediaType {
			return nil, "", ErrInvalid
		}
	}
	if chart == nil {
		return nil, "", ErrInvalid
	}
	data, err := ociBytes(ctx, r, *chart, MaxArchiveBytes)
	if err != nil {
		return nil, "", err
	}
	loaded, err := loadChart(data)
	if err != nil || loaded.Metadata.Name != metadata.Name || loaded.Metadata.Version != metadata.Version {
		clear(data)
		return nil, "", ErrSourceIntegrity
	}
	return data, descriptor.Digest.String(), nil
}

func ociBytes(ctx context.Context, r *remote.Repository, d ocispec.Descriptor, limit int64) ([]byte, error) {
	if d.Size < 1 || d.Size > limit || d.Digest.Validate() != nil || d.Digest.Algorithm() != digest.SHA256 || len(d.URLs) > 0 {
		return nil, ErrBudget
	}
	reader, err := r.Fetch(ctx, d)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	raw, err := limitedRead(reader, int(limit))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) != d.Size || digest.FromBytes(raw) != d.Digest {
		clear(raw)
		return nil, ErrSourceIntegrity
	}
	return raw, nil
}

func sourceError(err error) error {
	for _, known := range []error{ErrInvalid, ErrDenied, ErrBudget, ErrSourceIntegrity, ErrSourceAuth, ErrSourceUnavailable} {
		if errors.Is(err, known) {
			return known
		}
	}
	var response *errcode.ErrorResponse
	if errors.As(err, &response) && (response.StatusCode == 401 || response.StatusCode == 403) {
		return ErrSourceAuth
	}
	return ErrSourceUnavailable
}

// MaterialDigest binds the original chart and protected values to the sealed
// render. Execution uses the retained bytes and never resolves a source again.
func materialDigest(m Material) string {
	return keyedDigest(m.Key, bytes.Join([][]byte{m.Chart, m.Values}, []byte{0}))
}
