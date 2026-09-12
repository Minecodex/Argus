package artifacthttp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInternalRoutePreservesTLSAuthorityAndRejectsForeignOrigins(t *testing.T) {
	now := time.Now()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, _ := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	ca, _ = x509.ParseCertificate(der)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"artifacts.argus.test"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, _ := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, key)
	path := filepath.Join(t.TempDir(), "ca.pem")
	_ = os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600)
	requests := 0
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Host != "artifacts.argus.test" || r.TLS.ServerName != "artifacts.argus.test" || r.URL.Path != "/bucket/immutable" {
			t.Errorf("authority/path changed: %s %s %s", r.Host, r.TLS.ServerName, r.URL.Path)
		}
		_, _ = io.WriteString(w, "signed binary")
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER}, PrivateKey: key}}}
	server.StartTLS()
	defer server.Close()
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	client, err := New(path, Route{Origin: "https://artifacts.argus.test", DialAddress: strings.TrimPrefix(server.URL, "https://")})
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"HEAD", "GET"} {
		req, _ := http.NewRequestWithContext(context.Background(), method, "https://artifacts.argus.test/bucket/immutable", nil)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}
	for _, address := range []string{"http://artifacts.argus.test/bucket/immutable", "https://other.argus.test/bucket/immutable"} {
		if res, err := client.Get(address); err == nil {
			res.Body.Close()
			t.Fatal("accepted forbidden origin")
		}
	}
	if requests != 2 {
		t.Fatalf("unexpected forwarded requests: %d", requests)
	}
	wrong, err := New(path, Route{Origin: "https://wrong.argus.test", DialAddress: strings.TrimPrefix(server.URL, "https://")})
	if err != nil {
		t.Fatal(err)
	}
	if res, err := wrong.Get("https://wrong.argus.test/bucket/immutable"); err == nil {
		res.Body.Close()
		t.Fatal("accepted wrong SNI")
	}
}
