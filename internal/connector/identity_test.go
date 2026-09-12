package connector

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
)

func TestGenerateLocalIdentityUsesConnectorURIAndPrivatePermissions(t *testing.T) {
	connectorID := uuid.MustParse("018f47e2-9a4c-7b31-8acd-02a2475e8d2f")
	directory := filepath.Join(t.TempDir(), "identity")
	identity, err := GenerateLocalIdentity(directory, connectorID)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(identity.CSRPEM))
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(csr.URIs) != 1 || csr.URIs[0].String() != CertificateURI(connectorID) {
		t.Fatalf("unexpected CSR URI SAN: %v", csr.URIs)
	}
	for _, name := range []string{"connector-key.pem", "connector.csr.pem"} {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode is %o", name, info.Mode().Perm())
		}
	}
}

func TestGenerateLocalIdentityRecreatesMissingCSRFromPersistedKey(t *testing.T) {
	connectorID := uuid.MustParse("018f47e2-9a4c-7b31-8acd-02a2475e8d2f")
	directory := filepath.Join(t.TempDir(), "identity")
	first, err := GenerateLocalIdentity(directory, connectorID)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(directory, "connector.csr.pem")); err != nil {
		t.Fatal(err)
	}
	recreated, err := GenerateLocalIdentity(directory, connectorID)
	if err != nil {
		t.Fatal(err)
	}
	if first.PrivateKey.D.Cmp(recreated.PrivateKey.D) != 0 {
		t.Fatal("repair regenerated the persisted Connector private key")
	}
	block, _ := pem.Decode([]byte(recreated.CSRPEM))
	request, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || request.CheckSignature() != nil || len(request.URIs) != 1 || request.URIs[0].String() != CertificateURI(connectorID) {
		t.Fatalf("recreated CSR is invalid: %v", err)
	}
}

func TestGenerateLocalIdentityStagesForeignConnectorWithoutMutatingActiveIdentity(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "identity")
	oldID, nextID := uuid.New(), uuid.New()
	if _, err := GenerateLocalIdentity(directory, oldID); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "identity.json"), []byte(`{"connector_id":"`+oldID.String()+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	oldKey, err := os.ReadFile(filepath.Join(directory, "connector-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	staged, err := GenerateLocalIdentity(directory, nextID)
	if err != nil || !staged.Staged {
		t.Fatalf("foreign Connector identity was not staged: staged=%t err=%v", staged.Staged, err)
	}
	retried, err := GenerateLocalIdentity(directory, nextID)
	if err != nil || !retried.Staged || staged.PrivateKey.D.Cmp(retried.PrivateKey.D) != 0 {
		t.Fatalf("staged identity retry was not stable: staged=%t err=%v", retried.Staged, err)
	}
	currentKey, err := os.ReadFile(filepath.Join(directory, "connector-key.pem"))
	if err != nil || !bytes.Equal(oldKey, currentKey) {
		t.Fatal("active Connector identity changed before enrollment committed")
	}
	stage := filepath.Join(directory, ".enrollment", nextID.String())
	if _, err = os.Stat(filepath.Join(stage, "connector-key.pem")); err != nil {
		t.Fatal("staged private key is missing")
	}
	if err = RemoveStagedLocalIdentity(directory, nextID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(stage); !os.IsNotExist(err) {
		t.Fatal("staged identity was not removed")
	}
}
