package argusidentity

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestExistingIdentityKeyFormatsPreserveKeyForRenewal(t *testing.T) {
	for _, format := range []string{"EC PRIVATE KEY", "PRIVATE KEY"} {
		t.Run(format, func(t *testing.T) {
			original, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			var der []byte
			if format == "PRIVATE KEY" {
				der, err = x509.MarshalPKCS8PrivateKey(original)
			} else {
				der, err = x509.MarshalECPrivateKey(original)
			}
			if err != nil {
				t.Fatal(err)
			}
			encoded := pem.EncodeToMemory(&pem.Block{Type: format, Bytes: der})
			path := filepath.Join(t.TempDir(), "client-key.pem")
			if err = os.WriteFile(path, encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			key, returned, err := loadOrCreatePrivateKey(path)
			if err != nil {
				t.Fatal(err)
			}
			if !key.PublicKey.Equal(&original.PublicKey) || !bytes.Equal(returned, encoded) {
				t.Fatal("renewal replaced or re-encoded the installed identity key")
			}
			csr, err := createCSR("collector", key, x509.ExtKeyUsageClientAuth)
			if err != nil {
				t.Fatal(err)
			}
			block, _ := pem.Decode(csr)
			request, err := x509.ParseCertificateRequest(block.Bytes)
			if err != nil || request.CheckSignature() != nil || !request.PublicKey.(*ecdsa.PublicKey).Equal(&original.PublicKey) {
				t.Fatal("renewal CSR is not signed by the installed identity")
			}
		})
	}
}

func TestUnsupportedExistingIdentityKeyIsNotReplaced(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	for _, encoded := range [][]byte{
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("invalid")}),
		[]byte("not a private key"),
	} {
		path := filepath.Join(t.TempDir(), "client-key.pem")
		if err = os.WriteFile(path, encoded, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err = loadOrCreatePrivateKey(path); err == nil {
			t.Fatal("unsupported private key accepted")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, encoded) {
			t.Fatal("unsupported installed key was modified")
		}
	}
}
