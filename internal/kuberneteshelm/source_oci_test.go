package kuberneteshelm

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"helm.sh/helm/v4/pkg/registry"
)

type sourceOCIArtifact struct {
	manifest []byte
	digest   string
	blobs    map[string][]byte
}

func ociFixtureArtifact(t *testing.T) sourceOCIArtifact {
	chart := packagedChart(t, "1.0.0", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\n")
	config := []byte(`{"name":"sample","version":"1.0.0","apiVersion":"v2"}`)
	descriptor := func(data []byte, media string) ocispec.Descriptor {
		return ocispec.Descriptor{MediaType: media, Size: int64(len(data)), Digest: digest.FromBytes(data)}
	}
	m := ocispec.Manifest{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageManifest, Config: descriptor(config, registry.ConfigMediaType), Layers: []ocispec.Descriptor{descriptor(chart, registry.ChartLayerMediaType)}}
	raw, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	return sourceOCIArtifact{manifest: raw, digest: digest.FromBytes(raw).String(), blobs: map[string][]byte{digest.FromBytes(chart).String(): chart, digest.FromBytes(config).String(): config}}
}

func (a sourceOCIArtifact) serve(w http.ResponseWriter, r *http.Request) {
	var data []byte
	if strings.Contains(r.URL.Path, "/manifests/") {
		ref := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if ref != "1.0.0" && ref != a.digest {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", ocispec.MediaTypeImageManifest)
		w.Header().Set("Docker-Content-Digest", a.digest)
		data = a.manifest
	} else if strings.Contains(r.URL.Path, "/blobs/") {
		data = a.blobs[r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]]
	} else {
		w.WriteHeader(404)
		return
	}
	if data == nil {
		w.WriteHeader(404)
		return
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	if r.Method != http.MethodHead {
		w.Write(data)
	}
}

func TestHelmOCIUsesScopedAuthenticationAndVerifiesManifestAndLayerDigests(t *testing.T) {
	artifact := ociFixtureArtifact(t)
	var corrupt atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		if u != "monitor" || p != "oci-private-canary" {
			w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			w.WriteHeader(401)
			return
		}
		if corrupt.Load() && strings.Contains(r.URL.Path, "/blobs/") {
			w.Write([]byte("forged-layer"))
			return
		}
		artifact.serve(w, r)
	}))
	defer server.Close()
	s := ChartSource{Kind: "oci", URL: "oci://" + strings.TrimPrefix(server.URL, "https://") + "/charts/sample", Version: "1.0.0"}
	c := SourceCredentials{Username: "monitor", Password: "oci-private-canary", CAPEM: sourceCA(server)}
	engine := Engine{SourcePolicy: fixtureSourcePolicy{}}
	raw, receipt, e := engine.resolveChart(t.Context(), s, &c)
	if e != nil || receipt.Digest != artifact.digest || receipt.ArchiveDigest != archiveDigest(raw) || receipt.Kind != "oci" {
		t.Fatal("OCI resolution", receipt, e)
	}
	s.Digest = artifact.digest
	if _, _, e = engine.resolveChart(t.Context(), s, &c); e != nil {
		t.Fatal("digest reference", e)
	}
	c.Password = "wrong-password"
	if _, _, e = engine.resolveChart(t.Context(), s, &c); !errors.Is(e, ErrSourceAuth) {
		t.Fatal("bad OCI credentials", e)
	}
	c.Password = "oci-private-canary"
	corrupt.Store(true)
	if _, _, e = engine.resolveChart(t.Context(), s, &c); e == nil {
		t.Fatal("forged OCI layer accepted")
	}
}

func TestHelmOCICrossOriginTokenRealmRequiresExplicitTrust(t *testing.T) {
	artifact := ociFixtureArtifact(t)
	var tokenCalls atomic.Int64
	token := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		u, p, _ := r.BasicAuth()
		if u != "monitor" || p != "realm-private-canary" {
			w.WriteHeader(403)
			return
		}
		if r.URL.Query().Get("scope") != "repository:charts/sample:pull" {
			t.Error("incorrect OCI token scope")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"token":"ephemeral-pull-token"}`))
	}))
	defer token.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ephemeral-pull-token" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="source-fixture"`, token.URL))
			w.WriteHeader(401)
			return
		}
		artifact.serve(w, r)
	}))
	defer server.Close()
	s := ChartSource{Kind: "oci", URL: "oci://" + strings.TrimPrefix(server.URL, "https://") + "/charts/sample", Version: "1.0.0"}
	c := SourceCredentials{Username: "monitor", Password: "realm-private-canary", CAPEM: sourceCA(server) + sourceCA(token)}
	engine := Engine{SourcePolicy: fixtureSourcePolicy{}}
	if _, _, e := engine.resolveChart(t.Context(), s, &c); !errors.Is(e, ErrDenied) || tokenCalls.Load() != 0 {
		t.Fatal("untrusted token realm received credentials", e, tokenCalls.Load())
	}
	s.AuthURL = token.URL + "/token"
	if _, _, e := engine.resolveChart(t.Context(), s, &c); e != nil || tokenCalls.Load() == 0 {
		t.Fatal("explicit trusted token realm failed", e)
	}
}
