package argusdev

import (
	"strings"
	"testing"
)

func TestPlan6ProtocolTrustPreservesRootsAndRequiresHostname(t *testing.T) {
	fixture, err := generateFixtureCertificate("receiver", []string{"receiver.owned.svc"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyPlan6ProtocolCertificate([]byte(fixture.CA), []byte(fixture.Certificate), "receiver.owned.svc"); err != nil {
		t.Fatal(err)
	}
	if err := verifyPlan6ProtocolCertificate([]byte(fixture.CA), []byte(fixture.Certificate), "unrelated.svc"); err == nil {
		t.Fatal("wrong receiver hostname accepted")
	}
	other, err := generateFixtureCertificate("unrelated", []string{"receiver.owned.svc"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyPlan6ProtocolCertificate([]byte(other.CA), []byte(fixture.Certificate), "receiver.owned.svc"); err == nil {
		t.Fatal("unrelated trust anchor accepted")
	}
	dirs := strings.Split(plan6ProtocolCertDirectories("/existing/private-ca:/etc/ssl/certs"), ":")
	for _, required := range []string{"/etc/ssl/certs", "/etc/pki/tls/certs", "/existing/private-ca", plan6ProtocolTrustDirectory} {
		found := 0
		for _, dir := range dirs {
			if dir == required {
				found++
			}
		}
		if found != 1 {
			t.Fatalf("trust directory %s appears %d times", required, found)
		}
	}
}
