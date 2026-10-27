package sdkacceptance

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixtureCertificates(t *testing.T, directory, collector string) (*x509.CertPool, tls.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Plan8 isolated SDK fixture"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(72 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(rootPEM)
	write := func(name string, raw []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(directory, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("ca.pem", rootPEM)
	write("server-ca.pem", rootPEM)
	issue := func(serial int64, name string, dns []string, uri string, usage x509.ExtKeyUsage) tls.Certificate {
		t.Helper()
		private, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		cert := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(48 * time.Hour), DNSNames: dns, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
		if uri != "" {
			parsed, _ := url.Parse(uri)
			cert.URIs = []*url.URL{parsed}
		}
		der, e := x509.CreateCertificate(rand.Reader, cert, root, &private.PublicKey, key)
		if e != nil {
			t.Fatal(e)
		}
		privateDER, e := x509.MarshalECPrivateKey(private)
		if e != nil {
			t.Fatal(e)
		}
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateDER})
		if name == "client" || name == "server" {
			write(name+".pem", certPEM)
			write(name+"-key.pem", keyPEM)
		}
		pair, e := tls.X509KeyPair(certPEM, keyPEM)
		if e != nil {
			t.Fatal(e)
		}
		return pair
	}
	issue(2, "client", nil, "spiffe://argus/telemetry/collectors/"+collector, x509.ExtKeyUsageClientAuth)
	issue(3, "server", []string{"collector-" + collector + ".argus.telemetry"}, "spiffe://argus/telemetry/collector-servers/"+collector, x509.ExtKeyUsageServerAuth)
	endpoint := issue(4, "fixture", []string{"host.docker.internal"}, "", x509.ExtKeyUsageServerAuth)
	bundleHash, fingerprint := sha256.Sum256(rootPEM), sha256.Sum256(rootDER)
	state, _ := json.Marshal(map[string]any{"epoch": 1, "bundle_sha256": hex.EncodeToString(bundleHash[:]), "ca_fingerprints": []string{hex.EncodeToString(fingerprint[:])}})
	write("trust-bundle.json", state)
	return pool, endpoint
}
