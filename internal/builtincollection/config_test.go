package builtincollection

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func publicTestCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Plan8 public trust test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestComponentTrustIsPublicTypedAndScopeBound(t *testing.T) {
	ca := publicTestCA(t)
	raw, _ := json.Marshal(Config{Interval: "5s", ComponentTLS: &ComponentTLS{CAPEM: ca, ServerName: "kube-scheduler"}})
	parsed, err := ParseFor("kubernetes_scheduler_metrics", raw)
	if err != nil || parsed.ComponentTLS == nil || parsed.ComponentTLS.CAPEM != ca || parsed.ComponentTLS.ServerName != "kube-scheduler" {
		t.Fatalf("public trust: %+v %v", parsed, err)
	}
	for _, definition := range All() {
		_, err := ParseFor(definition.Key, raw)
		if (err == nil) != definition.ComponentTLS {
			t.Fatalf("TLS authority for %s: %v", definition.Key, err)
		}
	}
	for _, config := range []string{
		`{"component_tls":{"server_name":"${env:PRIVATE_TOKEN}"}}`,
		`{"component_tls":{"server_name":"https://foreign.example/metrics"}}`,
		`{"component_tls":{"server_name":"*.example.test"}}`,
		`{"component_tls":{"insecure_skip_verify":true}}`,
		`{"component_tls":{"token":"private"}}`,
		`{"component_tls":{"ca_pem":"-----BEGIN PRIVATE KEY-----"}}`,
	} {
		if _, err = ParseFor("kubernetes_scheduler_metrics", []byte(config)); err == nil {
			t.Fatalf("invalid trust accepted: %s", config)
		}
	}
	for _, value := range []string{"garbage\n" + ca, ca + "\n-----BEGIN PRIVATE KEY-----", "-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----"} {
		raw, _ := json.Marshal(Config{ComponentTLS: &ComponentTLS{CAPEM: value}})
		if _, err = ParseFor("kubernetes_controller_metrics", raw); err == nil {
			t.Fatal("mixed or invalid PEM accepted")
		}
	}
	parsed, err = ParseFor("kubernetes_scheduler_metrics", []byte(`{"component_tls":{}}`))
	if err != nil || parsed.ComponentTLS != nil {
		t.Fatal("empty trust settings did not restore defaults")
	}
}
