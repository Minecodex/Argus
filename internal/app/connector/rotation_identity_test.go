package connector

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorcore "github.com/kakj-go/Argus/internal/connector"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCertificateRotationPersistsIdentityUsableForSubsequentRepair(t *testing.T) {
	id := uuid.New()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	issue := func(key *ecdsa.PrivateKey, serial int64) []byte {
		t.Helper()
		uri, _ := url.Parse(connectorcore.CertificateURI(id))
		leaf := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), URIs: []*url.URL{uri}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		der, issueErr := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	store := localStore{directory: t.TempDir()}
	initial, err := connectorcore.GenerateLocalIdentity(store.directory, id)
	if err != nil {
		t.Fatal(err)
	}
	initialPEM, err := connectorcore.MarshalPrivateKey(initial.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	identity := identityState{ConnectorID: id.String(), Role: "host", InstanceID: "rotation-repair", Name: "rotation-repair", GatewayEndpoint: "grpcs://gateway.example.test:9443", EnrollmentEndpoint: "https://control.example.test", CertificateExpiresAt: now.Add(time.Hour)}
	if err = store.saveEnrollmentIdentity(identity, initialPEM, []byte(initial.CSRPEM), issue(initial.PrivateKey, 2), caPEM); err != nil {
		t.Fatal(err)
	}
	nextKey, _, err := connectorcore.GenerateCSR(id)
	if err != nil {
		t.Fatal(err)
	}
	nextPEM, err := connectorcore.MarshalPrivateKey(nextKey)
	if err != nil {
		t.Fatal(err)
	}
	grant := &connectorv1.CertificateRotationGrant{ConnectionEpoch: 42, CertificatePem: issue(nextKey, 3), NotAfter: timestamppb.New(now.Add(time.Hour))}
	if err = (connectorClient{store: store}).acceptRotation(identity, grant, nextPEM); err != nil {
		t.Fatal(err)
	}
	// This is the real entry used by enroll/repair after restarting the process.
	repaired, err := connectorcore.GenerateLocalIdentity(store.directory, id)
	if err != nil {
		t.Fatalf("rotation left identity unusable for formal repair: %v", err)
	}
	if !repaired.PrivateKey.Equal(nextKey) || repaired.ConnectorID != id {
		t.Fatal("repair changed the rotated private key or Connector identity")
	}
	certPEM, keyPEM, bundlePEM, err := store.identityMaterial()
	if err != nil || validateIssuedIdentity(id, keyPEM, certPEM, bundlePEM) != nil {
		t.Fatal("rotated identity material did not remain internally valid")
	}
}
