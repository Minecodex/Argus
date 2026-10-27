package collectormanager_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
)

type pipelineCertificates struct {
	pool                     *x509.CertPool
	server                   tls.Certificate
	serial, uri              string
	certificateHash, csrHash [32]byte
	notBefore, notAfter      time.Time
}

func pipelineTLS(t *testing.T, d *pipelineDocker, collector uuid.UUID) pipelineCertificates {
	t.Helper()
	now := time.Now().UTC()
	serial := func() *big.Int { v := uuid.New(); return new(big.Int).SetBytes(v[:]) }
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "Plan8 owned native pipeline CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(2 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, public, private)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("CA pool")
	}
	d.file("ca.pem", caPEM)
	bundleHash, fingerprint := sha256.Sum256(caPEM), sha256.Sum256(caDER)
	state, _ := json.Marshal(map[string]any{"epoch": 1, "bundle_sha256": hex.EncodeToString(bundleHash[:]), "ca_fingerprints": []string{hex.EncodeToString(fingerprint[:])}})
	d.file("trust-bundle.json", state)
	result := pipelineCertificates{pool: pool, notBefore: now.Add(-time.Minute), notAfter: now.Add(time.Hour)}
	for _, server := range []bool{true, false} {
		pub, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		name := "client"
		usage := x509.ExtKeyUsageClientAuth
		cert := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "Plan8 owned Collector"}, NotBefore: result.notBefore, NotAfter: result.notAfter, KeyUsage: x509.KeyUsageDigitalSignature}
		if server {
			name = "server"
			usage = x509.ExtKeyUsageServerAuth
			cert.DNSNames = []string{"host.docker.internal", "localhost", "collector-" + collector.String() + ".argus.telemetry"}
			cert.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		} else {
			result.uri = "spiffe://argus/telemetry/collectors/" + collector.String()
			uri, _ := url.Parse(result.uri)
			cert.URIs = []*url.URL{uri}
			csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: cert.Subject, URIs: cert.URIs}, key)
			if err != nil {
				t.Fatal(err)
			}
			result.csrHash = sha256.Sum256(csr)
		}
		cert.ExtKeyUsage = []x509.ExtKeyUsage{usage}
		der, err := x509.CreateCertificate(rand.Reader, cert, ca, pub, private)
		if err != nil {
			t.Fatal(err)
		}
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		d.file(name+".pem", certPEM)
		d.file(name+"-key.pem", keyPEM)
		if server {
			result.server, err = tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			result.serial = cert.SerialNumber.Text(16)
			result.certificateHash = sha256.Sum256(der)
		}
	}
	return result
}
