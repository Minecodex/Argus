package trustbundle

import (
	"crypto/x509"
	"testing"
)

func TestNormalizedDNSNamesUsesEmptyArrayForCertificateWithoutDNSNames(t *testing.T) {
	names := normalizedDNSNames(&x509.Certificate{})
	if names == nil || len(names) != 0 {
		t.Fatalf("normalized DNS names = %#v, want non-nil empty array", names)
	}
}

func TestNormalizedDNSNamesReturnsIndependentCopy(t *testing.T) {
	certificate := &x509.Certificate{DNSNames: []string{"connector.example.test"}}
	names := normalizedDNSNames(certificate)
	names[0] = "changed.example.test"
	if certificate.DNSNames[0] != "connector.example.test" {
		t.Fatal("normalized DNS names alias the certificate")
	}
}
