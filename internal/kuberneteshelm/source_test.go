package kuberneteshelm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Only controlled loopback fixtures use this policy. Production Direct and
// Connector paths retain the shared target policy, tested separately below.
type fixtureSourcePolicy struct{ revoked *atomic.Bool }

func (p fixtureSourcePolicy) Resolve(_ context.Context, host string) ([]netip.Addr, error) {
	if p.revoked != nil && p.revoked.Load() {
		return nil, ErrDenied
	}
	address, e := netip.ParseAddr(host)
	if e != nil || !address.IsLoopback() {
		return nil, ErrDenied
	}
	return []netip.Addr{address}, nil
}
func (p fixtureSourcePolicy) Revalidate(ctx context.Context, host string, _ []netip.Addr) error {
	_, e := p.Resolve(ctx, host)
	return e
}
func sourceCA(server *httptest.Server) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
}

func TestHelmSourceValidationAndDefaultNetworkPolicy(t *testing.T) {
	valid := ChartSource{Kind: "repository", URL: "https://charts.example/team", Chart: "sample", Version: "1.0.0"}
	for _, change := range []func(*ChartSource){
		func(s *ChartSource) { s.URL = "https://user:password@charts.example" },
		func(s *ChartSource) { s.URL = "https://charts.example/?token=secret" },
		func(s *ChartSource) { s.URL = "https://charts.example/%2e%2e/private" },
		func(s *ChartSource) { s.Version = ">=1.0" },
		func(s *ChartSource) { s.Digest = "sha256:invalid" },
		func(s *ChartSource) { s.Kind, s.URL = "oci", "oci://registry.example/team/sample:latest" },
	} {
		copy := valid
		change(&copy)
		if ValidateChartSource(&copy) == nil {
			t.Fatal("invalid remote source accepted")
		}
	}
	for _, c := range []SourceCredentials{{Username: "only-user"}, {Username: "u", Password: "p", Token: "t"}, {CAPEM: "-----BEGIN PRIVATE KEY-----"}} {
		if ValidateSourceCredentials(&c) == nil {
			t.Fatal("invalid credential accepted")
		}
	}
	var calls atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	valid.URL = server.URL
	_, _, e := (Engine{}).resolveChart(t.Context(), valid, &SourceCredentials{CAPEM: sourceCA(server)})
	if !errors.Is(e, ErrDenied) || calls.Load() != 0 {
		t.Fatal("default policy allowed a loopback source", e, calls.Load())
	}
}

func TestHelmRepositoryResolutionPinsIdentityAndDoesNotUseAmbientCredentials(t *testing.T) {
	archive := packagedChart(t, "1.0.0", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: example\n")
	var invalid atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		if user != "monitor" || pass != "source-private-canary" {
			w.Header().Set("WWW-Authenticate", `Basic realm="charts"`)
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/index.yaml" {
			digest := archiveDigest(archive)
			if invalid.Load() {
				digest = "sha256:" + strings.Repeat("1", 64)
			}
			fmt.Fprintf(w, "apiVersion: v1\nentries:\n  sample:\n    - name: sample\n      apiVersion: v2\n      version: 1.0.0\n      digest: %s\n      urls: [sample.tgz]\n", digest)
			return
		}
		w.Write(archive)
	}))
	defer server.Close()
	source := ChartSource{Kind: "repository", URL: server.URL, Chart: "sample", Version: "1.0.0"}
	c := SourceCredentials{Username: "monitor", Password: "source-private-canary", CAPEM: sourceCA(server)}
	engine := Engine{SourcePolicy: fixtureSourcePolicy{}}
	raw, receipt, e := engine.resolveChart(t.Context(), source, &c)
	if e != nil || !bytes.Equal(raw, archive) || receipt.ArchiveDigest != archiveDigest(archive) || receipt.Digest != receipt.ArchiveDigest || receipt.Chart != "sample" || receipt.Version != "1.0.0" || receipt.Transport != "https" {
		t.Fatal("actual repository resolution", receipt, e)
	}
	// No implicit credentials from HELM_*, Docker configuration or the OS.
	t.Setenv("HELM_CONFIG_HOME", t.TempDir())
	dockerConfig := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerConfig)
	ambient, _ := json.Marshal(map[string]any{"auths": map[string]any{strings.TrimPrefix(server.URL, "https://"): map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Password))}}})
	if e := os.WriteFile(filepath.Join(dockerConfig, "config.json"), ambient, 0600); e != nil {
		t.Fatal(e)
	}
	_, _, e = engine.resolveChart(t.Context(), source, &SourceCredentials{CAPEM: c.CAPEM})
	if !errors.Is(e, ErrSourceAuth) || strings.Contains(e.Error(), c.Password) {
		t.Fatal("authentication failure did not remain a sanitized source error", e)
	}
	_, _, e = engine.resolveChart(t.Context(), source, &SourceCredentials{Username: c.Username, Password: c.Password})
	if !errors.Is(e, ErrSourceUnavailable) {
		t.Fatal("untrusted TLS source fell back to insecure transport", e)
	}
	invalid.Store(true)
	_, _, e = engine.resolveChart(t.Context(), source, &c)
	if !errors.Is(e, ErrSourceIntegrity) {
		t.Fatal("index digest mismatch accepted", e)
	}
	invalid.Store(false)
	source.Version = "2.0.0"
	if _, _, e = engine.resolveChart(t.Context(), source, &c); e == nil {
		t.Fatal("missing exact version silently selected a different chart")
	}
}

func TestHelmSourceRedirectsStripCredentialsAndEnforceBudgetsAndRevalidation(t *testing.T) {
	archive := packagedChart(t, "1.0.0", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\n")
	var leaked, large atomic.Bool
	cdn := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			leaked.Store(true)
		}
		if large.Load() {
			w.Header().Set("Content-Length", fmt.Sprint(MaxArchiveBytes+1))
			return
		}
		w.Write(archive)
	}))
	defer cdn.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.yaml" {
			fmt.Fprintf(w, "apiVersion: v1\nentries:\n  sample:\n    - name: sample\n      version: 1.0.0\n      digest: %s\n      urls: [sample.tgz]\n", archiveDigest(archive))
			return
		}
		http.Redirect(w, r, cdn.URL+"/sample.tgz?temporary_signature=fixture", 302)
	}))
	defer server.Close()
	c := SourceCredentials{Username: "monitor", Password: "private-redirect-canary", CAPEM: sourceCA(server) + sourceCA(cdn)}
	source := ChartSource{Kind: "repository", URL: server.URL, Chart: "sample", Version: "1.0.0"}
	var revoked atomic.Bool
	engine := Engine{SourcePolicy: fixtureSourcePolicy{revoked: &revoked}}
	if _, _, e := engine.resolveChart(t.Context(), source, &c); e != nil || leaked.Load() {
		t.Fatal("cross-origin TLS CDN transfer leaked credentials or failed", e)
	}
	large.Store(true)
	if _, _, e := engine.resolveChart(t.Context(), source, &c); !errors.Is(e, ErrBudget) {
		t.Fatal("oversized archive accepted", e)
	}
	large.Store(false)
	n, e := newSourceSession(source, c, engine.SourcePolicy)
	if e != nil {
		t.Fatal(e)
	}
	defer n.transport.CloseIdleConnections()
	u := *n.origin
	u.Path = "/index.yaml"
	if _, e = sourceGet(t.Context(), n, &u, c, 2<<20); e != nil {
		t.Fatal(e)
	}
	revoked.Store(true)
	if _, e = sourceGet(t.Context(), n, &u, c, 2<<20); !errors.Is(e, ErrDenied) {
		t.Fatal("cached connections bypassed target revalidation", e)
	}
	request, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:1", nil)
	if _, e := n.RoundTrip(request); !errors.Is(e, ErrDenied) {
		t.Fatal("TLS downgrade accepted", e)
	}
}
