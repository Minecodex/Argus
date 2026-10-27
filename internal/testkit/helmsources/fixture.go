// Package helmsources supplies owned TLS repository/OCI fixtures for component
// acceptance. It is imported only by tests and never supplies product data.
package helmsources

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/crypto/bcrypt"
	"helm.sh/helm/v4/pkg/registry"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

type Fixture struct {
	Archive                                                                 []byte
	ManifestDigest, RepositoryURL, OCIURL, CAPEM, Username, Password, Owner string
	Requests                                                                atomic.Int64
	t                                                                       *testing.T
	repository                                                              *httptest.Server
	registry                                                                string
	closed                                                                  bool
}

// LoopbackPolicy applies only to controlled published ports in local tests.
// Product paths use DirectTargetValidator, which rejects these addresses.
type LoopbackPolicy struct{}

func (LoopbackPolicy) Resolve(_ context.Context, host string) ([]netip.Addr, error) {
	ip, e := netip.ParseAddr(host)
	if e != nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("fixture address rejected")
	}
	return []netip.Addr{ip}, nil
}
func (p LoopbackPolicy) Revalidate(ctx context.Context, host string, _ []netip.Addr) error {
	_, e := p.Resolve(ctx, host)
	return e
}

func Start(t *testing.T) *Fixture {
	t.Helper()
	if os.Getenv("ARGUS_HELM_SOURCE_REGISTRY") != "1" {
		t.Skip("requires explicitly owned Helm repository and OCI registry")
	}
	f := &Fixture{t: t, Owner: "plan8-helm-sources-" + uuid.NewString()[:10], Username: "reader", Password: "source-private-" + uuid.NewString()}
	t.Cleanup(f.Close)
	f.Archive = chart(t)
	directory := t.TempDir()
	certificate, key, ca := fixtureTLS(t)
	f.CAPEM = string(ca)
	write := func(name string, data []byte) string {
		p := filepath.Join(directory, name)
		if e := os.WriteFile(p, data, 0600); e != nil {
			t.Fatal(e)
		}
		return p
	}
	certPath, keyPath := write("tls.pem", certificate), write("tls.key", key)
	hashed, e := bcrypt.GenerateFromPassword([]byte(f.Password), bcrypt.MinCost)
	if e != nil {
		t.Fatal(e)
	}
	passwordPath := write("htpasswd", []byte(f.Username+":"+string(hashed)+"\n"))
	pair, e := tls.X509KeyPair(certificate, key)
	if e != nil {
		t.Fatal(e)
	}
	repo := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.Requests.Add(1)
		u, p, _ := r.BasicAuth()
		if u != f.Username || p != f.Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="owned-chart-repository"`)
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/index.yaml" {
			fmt.Fprintf(w, "apiVersion: v1\nentries:\n  sample:\n    - name: sample\n      apiVersion: v2\n      version: 1.0.0\n      digest: %s\n      urls: [sample.tgz]\n", digest.FromBytes(f.Archive))
			return
		}
		if r.URL.Path != "/sample.tgz" {
			w.WriteHeader(404)
			return
		}
		w.Write(f.Archive)
	}))
	repo.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	repo.StartTLS()
	f.repository, f.RepositoryURL = repo, repo.URL
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	f.registry = f.Owner + "-registry"
	// Cleanup is registered before dispatch, including unknown docker-run outcomes.
	f.must("run", "-d", "--name", f.registry, "--label", "argus.owner="+f.Owner, "--memory=192m", "--cpus=.5", "--read-only", "--tmpfs", "/var/lib/registry:rw,size=67108864", "--tmpfs", "/tmp:rw,size=16777216", "-p", fmt.Sprintf("127.0.0.1:%d:5000", port), "-e", "REGISTRY_HTTP_TLS_CERTIFICATE=/tls.pem", "-e", "REGISTRY_HTTP_TLS_KEY=/tls.key", "-e", "REGISTRY_AUTH=htpasswd", "-e", "REGISTRY_AUTH_HTPASSWD_REALM=Plan8", "-e", "REGISTRY_AUTH_HTPASSWD_PATH=/htpasswd", "--mount", "type=bind,source="+certPath+",target=/tls.pem,readonly", "--mount", "type=bind,source="+keyPath+",target=/tls.key,readonly", "--mount", "type=bind,source="+passwordPath+",target=/htpasswd,readonly", "registry:3.0.0")
	host := fmt.Sprintf("127.0.0.1:%d", port)
	f.OCIURL = "oci://" + host + "/plan8/sample"
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca)
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	deadline := time.Now().Add(25 * time.Second)
	for {
		request, _ := http.NewRequestWithContext(t.Context(), "GET", "https://"+host+"/v2/", nil)
		request.SetBasicAuth(f.Username, f.Password)
		response, e := client.Do(request)
		if e == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("owned OCI registry not ready")
		}
		time.Sleep(200 * time.Millisecond)
	}
	r, e := remote.NewRepository(host + "/plan8/sample")
	if e != nil {
		t.Fatal(e)
	}
	r.Client = &auth.Client{Client: client, Cache: auth.NewCache(), Credential: auth.StaticCredential(host, auth.Credential{Username: f.Username, Password: f.Password})}
	store := memory.New()
	push := func(data []byte, media string) ocispec.Descriptor {
		d := ocispec.Descriptor{MediaType: media, Size: int64(len(data)), Digest: digest.FromBytes(data)}
		if e := store.Push(t.Context(), d, bytes.NewReader(data)); e != nil {
			t.Fatal(e)
		}
		return d
	}
	config := push([]byte(`{"name":"sample","version":"1.0.0","apiVersion":"v2"}`), registry.ConfigMediaType)
	layer := push(f.Archive, registry.ChartLayerMediaType)
	manifest, _ := json.Marshal(ocispec.Manifest{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageManifest, Config: config, Layers: []ocispec.Descriptor{layer}})
	d := push(manifest, ocispec.MediaTypeImageManifest)
	if e := store.Tag(t.Context(), d, "1.0.0"); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	if _, e := oras.Copy(ctx, store, "1.0.0", r, "1.0.0", oras.DefaultCopyOptions); e != nil {
		t.Fatal("owned OCI artifact publish failed")
	}
	f.ManifestDigest = d.Digest.String()
	return f
}

func (f *Fixture) Close() {
	if f.closed {
		return
	}
	f.closed = true
	if f.repository != nil {
		f.repository.Close()
	}
	if f.registry != "" {
		owner, e := f.command("inspect", "--format", `{{index .Config.Labels "argus.owner"}}`, f.registry)
		if e == nil && strings.TrimSpace(owner) == f.Owner {
			if _, e = f.command("rm", "-f", "-v", f.registry); e != nil {
				f.t.Error("owned registry cleanup failed")
			}
		}
	}
}
func (f *Fixture) command(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	raw, e := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	return strings.ReplaceAll(strings.TrimSpace(string(raw)), f.Password, "[REDACTED]"), e
}
func (f *Fixture) must(args ...string) string {
	f.t.Helper()
	out, e := f.command(args...)
	if e != nil {
		f.t.Fatalf("owned registry Docker operation: %v %s", e, out)
	}
	return out
}
func chart(t *testing.T) []byte {
	var raw bytes.Buffer
	z := gzip.NewWriter(&raw)
	w := tar.NewWriter(z)
	for _, file := range [][2]string{{"sample/Chart.yaml", "apiVersion: v2\nname: sample\nversion: 1.0.0\n"}, {"sample/templates/settings.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Release.Name }}-settings\ndata:\n  message: {{ .Values.message | quote }}\n  fixed: {{ randAlphaNum 12 | quote }}\n"}} {
		data := []byte(file[1])
		if e := w.WriteHeader(&tar.Header{Name: file[0], Mode: 0600, Size: int64(len(data))}); e != nil {
			t.Fatal(e)
		}
		if _, e := w.Write(data); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return raw.Bytes()
}
func fixtureTLS(t *testing.T) ([]byte, []byte, []byte) {
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Plan8 source fixture CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	rootDER, e := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "Plan8 source fixture"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, e := x509.CreateCertificate(rand.Reader, leaf, root, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	private, e := x509.MarshalECPrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: private}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})
}
