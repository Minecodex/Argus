package argusidentity

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

const followerCollectorID = "018f08d2-7d43-7a54-a8fb-f2f3f2f0d111"

func TestAgentsFollowOneIdentityWithoutEnrollmentOrRotation(t *testing.T) {
	ca, caKey, caPEM := testCertificateAuthority(t)
	source := filepath.Join(t.TempDir(), "projection")
	initial := followerTestMaterial(t, ca, caKey, caPEM, followerCollectorID, 10, 1, time.Now().Add(-2*time.Minute), time.Now().Add(time.Hour))
	writeFollowerProjection(t, source, initial)
	var agents []*identityExtension
	for range 3 {
		config := followerTestConfig(source, t.TempDir())
		extension := newIdentityExtension(config, zap.NewNop())
		// Less than rotate_before remains. An independent renewal would call
		// the unreachable HTTPS endpoint and fail, rather than following.
		if err := extension.Start(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		agents = append(agents, extension)
	}
	next := followerTestMaterial(t, ca, caKey, caPEM, followerCollectorID, 20, 1, time.Now().Add(-time.Minute), time.Now().Add(24*time.Hour))
	writeFollowerProjection(t, source, next)
	for _, agent := range agents {
		if err := agent.ensureIdentity(t.Context()); err != nil {
			t.Fatal(err)
		}
		assertFollowerFiles(t, agent.config, next)
		if err := agent.Shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
		// A restarted Agent must adopt the current projection as well.
		restarted := newIdentityExtension(agent.config, zap.NewNop())
		if err := restarted.Start(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		if err := restarted.Shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
		assertFollowerFiles(t, restarted.config, next)
	}
}

func TestFollowerRejectsInvalidUpdatesWithoutChangingLastValidFiles(t *testing.T) {
	ca, caKey, caPEM := testCertificateAuthority(t)
	now := time.Now()
	baseline := followerTestMaterial(t, ca, caKey, caPEM, followerCollectorID, 30, 2, now.Add(-time.Minute), now.Add(time.Hour))
	for _, scenario := range []string{"missing", "key-mismatch", "wrong-collector", "expired", "wrong-eku", "wrong-dns", "wrong-chain", "bundle-hash", "epoch-rollback", "epoch-replacement", "certificate-rollback"} {
		t.Run(scenario, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "projection")
			writeFollowerProjection(t, source, baseline)
			config := followerTestConfig(source, t.TempDir())
			if err := followBootstrapIdentity(config); err != nil {
				t.Fatal(err)
			}
			next := followerTestMaterial(t, ca, caKey, caPEM, followerCollectorID, 40, 2, now.Add(-time.Minute), now.Add(time.Hour))
			switch scenario {
			case "missing":
				delete(next, "server.pem")
			case "key-mismatch":
				next["client-key.pem"] = baseline["client-key.pem"]
			case "wrong-collector":
				next = followerTestMaterial(t, ca, caKey, caPEM, "another-collector", 40, 2, now.Add(-time.Minute), now.Add(time.Hour))
			case "expired":
				next = followerTestMaterial(t, ca, caKey, caPEM, followerCollectorID, 40, 2, now.Add(-2*time.Hour), now.Add(-time.Minute))
			case "wrong-eku":
				next["client.pem"] = next["server.pem"]
			case "wrong-dns":
				key, _ := parseIdentityPrivateKey(next["server-key.pem"])
				next["server.pem"] = followerTestLeaf(t, ca, caKey, key, followerCollectorID, 42, now.Add(-time.Minute), now.Add(time.Hour), x509.ExtKeyUsageServerAuth, "unexpected.argus.telemetry")
			case "wrong-chain":
				otherCA, otherKey, _ := testCertificateAuthority(t)
				key, _ := parseIdentityPrivateKey(next["client-key.pem"])
				next["client.pem"] = followerTestLeaf(t, otherCA, otherKey, key, followerCollectorID, 42, now.Add(-time.Minute), now.Add(time.Hour), x509.ExtKeyUsageClientAuth, "")
			case "bundle-hash":
				next["trust-bundle.json"] = []byte(`{"epoch":2,"bundle_sha256":"invalid"}`)
			case "epoch-rollback":
				next = followerTestMaterial(t, ca, caKey, caPEM, followerCollectorID, 40, 1, now.Add(-time.Minute), now.Add(time.Hour))
			case "epoch-replacement":
				otherCA, otherKey, otherPEM := testCertificateAuthority(t)
				next = followerTestMaterial(t, otherCA, otherKey, otherPEM, followerCollectorID, 40, 2, now.Add(-time.Minute), now.Add(time.Hour))
			case "certificate-rollback":
				next = followerTestMaterial(t, ca, caKey, caPEM, followerCollectorID, 40, 2, now.Add(-time.Hour), now.Add(time.Hour))
			}
			// Use a new directory so missing files really are absent.
			config.BootstrapIdentityDir = filepath.Join(t.TempDir(), "next")
			writeFollowerProjection(t, config.BootstrapIdentityDir, next)
			if err := followBootstrapIdentity(config); err == nil {
				t.Fatal("invalid identity update accepted")
			}
			assertFollowerFiles(t, config, baseline)
			writeFollowerProjection(t, config.BootstrapIdentityDir, baseline)
			if err := followBootstrapIdentity(config); err != nil {
				t.Fatalf("valid retry failed: %v", err)
			}
		})
	}
}

func TestFollowerPinsKubernetesProjectionAndRejectsEscapingDirectory(t *testing.T) {
	ca, key, caPEM := testCertificateAuthority(t)
	root := t.TempDir()
	first := filepath.Join(root, "..first")
	second := filepath.Join(root, "..second")
	now := time.Now()
	before := followerTestMaterial(t, ca, key, caPEM, followerCollectorID, 50, 1, now.Add(-time.Minute), now.Add(time.Hour))
	after := followerTestMaterial(t, ca, key, caPEM, followerCollectorID, 60, 1, now.Add(-time.Minute), now.Add(time.Hour))
	writeFollowerProjection(t, first, before)
	writeFollowerProjection(t, second, after)
	link := filepath.Join(root, "..data")
	if err := os.Symlink(first, link); err != nil {
		t.Skipf("directory symlink is unavailable: %v", err)
	}
	pinned, err := projectedIdentityDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	for name, expected := range before {
		actual, err := os.ReadFile(filepath.Join(pinned, name))
		if err != nil || !bytes.Equal(actual, expected) {
			t.Fatal("pinned projection mixed versions")
		}
	}
	config := followerTestConfig(root, t.TempDir())
	if err := followBootstrapIdentity(config); err != nil {
		t.Fatal(err)
	}
	assertFollowerFiles(t, config, after)
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if _, err := projectedIdentityDirectory(root); err == nil {
		t.Fatal("escaping Kubernetes projection accepted")
	}
}

func TestFollowerCannotAlsoOwnRenewalMirror(t *testing.T) {
	t.Setenv(kubernetesMirrorEnv, "1")
	config := followerTestConfig(t.TempDir(), t.TempDir())
	if err := newIdentityExtension(config, zap.NewNop()).Start(t.Context(), nil); err == nil {
		t.Fatal("follower also owns identity mirror")
	}
	t.Setenv(kubernetesMirrorEnv, "")
	config.BootstrapIdentityDir = ""
	if config.Validate() == nil {
		t.Fatal("follower without projection accepted")
	}
	config.BootstrapIdentityDir = t.TempDir()
	config.CheckInterval = 5 * time.Minute
	if config.Validate() == nil {
		t.Fatal("follower cannot refresh within the overlap budget")
	}
}

func followerTestConfig(source, target string) Config {
	return Config{CollectorID: followerCollectorID, EnrollmentEndpoint: "https://127.0.0.1:1/enroll", RotationEndpoint: "https://127.0.0.1:1/rotate",
		TrustBundleEndpoint: "https://127.0.0.1:1/trust-bundle", FollowBootstrapIdentity: true, BootstrapIdentityDir: source,
		CertificateFile: filepath.Join(target, "client.pem"), PrivateKeyFile: filepath.Join(target, "client-key.pem"),
		ServerCertificateFile: filepath.Join(target, "server.pem"), ServerPrivateKeyFile: filepath.Join(target, "server-key.pem"),
		CABundleFile: filepath.Join(target, "ca.pem"), TrustBundleStateFile: filepath.Join(target, "trust-bundle.json"),
		EnrollmentTokenFile: filepath.Join(target, "enrollment-token"), RotateBefore: 8 * time.Hour, CheckInterval: time.Minute}
}

func followerTestMaterial(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, caPEM, collector string, serial, epoch int64, before, after time.Time) map[string][]byte {
	t.Helper()
	values := map[string][]byte{"ca.pem": []byte(caPEM)}
	for _, role := range []string{"client", "server"} {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		values[role+"-key.pem"] = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})
		usage, dns := x509.ExtKeyUsageClientAuth, ""
		if role == "server" {
			usage, dns = x509.ExtKeyUsageServerAuth, collectorDNSName(collector)
			serial++
		}
		values[role+".pem"] = followerTestLeaf(t, ca, caKey, key, collector, serial, before, after, usage, dns)
	}
	material, err := parseTrustBundle([]byte(caPEM), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	values["trust-bundle.json"], err = json.Marshal(trustBundleState{Epoch: epoch, BundleSHA256: material.SHA256, CAFingerprints: material.Fingerprints})
	if err != nil {
		t.Fatal(err)
	}
	return values
}

func followerTestLeaf(t *testing.T, ca *x509.Certificate, caKey, key *ecdsa.PrivateKey, collector string, serial int64, before, after time.Time, usage x509.ExtKeyUsage, dns string) []byte {
	t.Helper()
	uri := spiffePrefix + collector
	if usage == x509.ExtKeyUsageServerAuth {
		uri = "spiffe://argus/telemetry/collector-servers/" + collector
	}
	identity, _ := url.Parse(uri)
	template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "collector"}, URIs: []*url.URL{identity},
		NotBefore: before, NotAfter: after, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
	if dns != "" {
		template.DNSNames = []string{dns}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func writeFollowerProjection(t *testing.T, path string, values map[string][]byte) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, value := range values {
		if err := os.WriteFile(filepath.Join(path, name), value, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func assertFollowerFiles(t *testing.T, config Config, values map[string][]byte) {
	t.Helper()
	for name, path := range identityPaths(config) {
		actual, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(actual, values[name]) {
			t.Fatalf("runtime identity file %s changed or was not synchronized", name)
		}
		if err := verifyPrivateFile(path); err != nil {
			t.Fatalf("runtime identity file %s has unsafe permissions", name)
		}
	}
}
