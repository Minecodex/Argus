package installinstruction

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/installation"
)

func TestDynamicBootstrapRequiresExplicitTLSMode(t *testing.T) {
	for _, mode := range []DownloadTLSMode{"", "fallback-on-error"} {
		_, err := BuildPOSIX(POSIXOptions{Scope: ScopeLinuxSystem, Platform: installation.LinuxAMD64,
			InstallerURL: "https://example.test/install.sh", BootstrapScriptURL: "https://example.test/bootstrap", DownloadTLSMode: mode,
			InstallerSHA256: strings.Repeat("a", 64), TrustBundlePEM: testBundle(t), TrustBundleEpoch: 1,
			Token: "test-token", ExpiresAt: time.Now().Add(time.Hour)})
		if err == nil || !strings.Contains(err.Error(), "TLS mode") {
			t.Fatalf("missing or invalid TLS policy was not rejected: %v", err)
		}
	}
}

// Execute the unmodified user command against real HTTPS servers. The first
// request and the downloaded bootstrap must enforce different trust policies.
func TestBootstrapTLSBoundary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute the POSIX bootstrap suite in the Linux test container")
	}
	for _, name := range []string{"sh", "curl", "sha256sum", "base64"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Fatalf("bootstrap test requires %s", name)
		}
	}
	for _, tc := range []struct {
		name                                                     string
		mode                                                     DownloadTLSMode
		trusted, wrongCA, wrongName, expired, badDigest, success bool
	}{
		{name: "managed-first-fetch", mode: DownloadTLSInsecureFirstFetch, success: true},
		{name: "strict-untrusted", mode: DownloadTLSStrict},
		{name: "strict-trusted", mode: DownloadTLSStrict, trusted: true, success: true},
		{name: "later-wrong-ca", mode: DownloadTLSInsecureFirstFetch, wrongCA: true},
		{name: "later-wrong-hostname", mode: DownloadTLSInsecureFirstFetch, wrongName: true},
		{name: "later-expired-certificate", mode: DownloadTLSInsecureFirstFetch, expired: true},
		{name: "installer-tampered", mode: DownloadTLSInsecureFirstFetch, badDigest: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundle, certificate := bootstrapTestCertificate(t, tc.wrongName, tc.expired)
			var script string
			var fetched atomic.Int32
			installer := "printf 'verified-installer-executed'\n"
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/bootstrap":
					if r.Header.Get("X-Argus-Enrollment-Token") != "bootstrap-test-token" || r.URL.Query().Get("scope") != "linux-system" {
						http.Error(w, "invalid request", http.StatusUnauthorized)
						return
					}
					fetched.Add(1)
					_, _ = w.Write([]byte(script + "\n"))
				case "/install.sh":
					_, _ = w.Write([]byte(installer))
				default:
					http.NotFound(w, r)
				}
			}))
			server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
			server.StartTLS()
			defer server.Close()
			if tc.wrongCA {
				bundle = testBundle(t)
			}
			digest := sha256.Sum256([]byte(installer))
			if tc.badDigest {
				digest[0] ^= 1
			}
			set, err := BuildPOSIX(POSIXOptions{Scope: ScopeLinuxSystem, Platform: installation.LinuxAMD64, InstallerURL: server.URL + "/install.sh",
				BootstrapScriptURL: server.URL + "/bootstrap", DownloadTLSMode: tc.mode,
				InstallerSHA256: hex.EncodeToString(digest[:]), TrustBundlePEM: bundle, TrustBundleEpoch: 1,
				Token: "bootstrap-test-token", ExpiresAt: time.Now().Add(time.Hour)})
			if err != nil {
				t.Fatal(err)
			}
			script = set.BootstrapScript
			process := exec.CommandContext(t.Context(), "sh", "-c", set.Command)
			process.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "NO_PROXY=*"}
			if tc.trusted {
				path := filepath.Join(t.TempDir(), "trusted.pem")
				if err := os.WriteFile(path, bundle, 0600); err != nil {
					t.Fatal(err)
				}
				process.Env = append(process.Env, "CURL_CA_BUNDLE="+path)
			}
			output, err := process.CombinedOutput()
			if tc.success {
				if err != nil || string(output) != "verified-installer-executed" {
					t.Fatalf("verified installation failed: %v: %s", err, output)
				}
			} else if err == nil || strings.Contains(string(output), "verified-installer-executed") {
				t.Fatalf("untrusted or tampered installer executed: %v: %s", err, output)
			}
			if tc.name == "strict-untrusted" && fetched.Load() != 0 {
				t.Fatal("strict TLS failure reached the authenticated bootstrap endpoint")
			}
			if tc.mode == DownloadTLSInsecureFirstFetch && fetched.Load() != 1 {
				t.Fatal("first-fetch exception did not stop at the first request")
			}
		})
	}
}

func bootstrapTestCertificate(t *testing.T, wrongName, expired bool) ([]byte, tls.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Bootstrap test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	root, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leafNotBefore, leafNotAfter := ca.NotBefore, ca.NotAfter
	if expired {
		leafNotBefore, leafNotAfter = time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: leafNotBefore, NotAfter: leafNotAfter,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	if wrongName {
		leaf.DNSNames = []string{"wrong.example.test"}
	} else {
		leaf.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root}), tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
