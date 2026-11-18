package alerting

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func notificationTestCA(t *testing.T) (string, string, tls.Certificate) {
	t.Helper()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Owned notification CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, public, key)
	if err != nil {
		t.Fatal(err)
	}
	root := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "Owned notification receiver"}, NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(time.Hour), BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, public, key)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	leafPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}))
	certificate, err := tls.X509KeyPair([]byte(leafPEM+root), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}))
	if err != nil {
		t.Fatal(err)
	}
	return root, leafPEM, certificate
}

func TestNotificationCredentialTrustDoesNotEscapeItsConnection(t *testing.T) {
	root, _, certificate := notificationTestCA(t)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"errcode":0}`)) }))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}}
	transport := Transport{Client: client, ValidateEndpoint: func(context.Context, string) error { return nil }}
	credential := NotificationCredential{WebhookURL: server.URL, CAPEM: root}
	if result := transport.Send(t.Context(), ChannelSpec{Provider: "wecom"}, credential, Message{Text: "owned TLS receiver"}); !result.Accepted {
		t.Fatal(result)
	}
	credential.CAPEM = ""
	if result := transport.Send(t.Context(), ChannelSpec{Provider: "wecom"}, credential, Message{Text: "must not inherit another credential"}); result.Accepted {
		t.Fatal("credential CA escaped its send")
	}
	if client.Transport.(*http.Transport).TLSClientConfig.RootCAs != nil {
		t.Fatal("shared client roots mutated")
	}
}

func TestNotificationCARejectsUnboundedOrNonCAMaterial(t *testing.T) {
	root, leaf, _ := notificationTestCA(t)
	for _, value := range []string{"", leaf, "junk\n" + root, root + "private-canary", "-----BEGIN PRIVATE KEY-----\ncanary\n-----END PRIVATE KEY-----", strings.Repeat(root, 17), strings.Repeat("x", 64<<10+1)} {
		if _, err := notificationTLS(nil, value); err == nil || strings.Contains(err.Error(), "canary") {
			t.Fatal("invalid or private material accepted/leaked")
		}
	}
	base := &tls.Config{ServerName: "owned.example", MinVersion: tls.VersionTLS13, RootCAs: x509.NewCertPool()}
	configuration, err := notificationTLS(base, root)
	if err != nil || configuration.ServerName != base.ServerName || configuration.MinVersion != tls.VersionTLS13 || configuration.InsecureSkipVerify || len(base.RootCAs.Subjects()) != 0 {
		t.Fatal("TLS identity or original trust changed", err)
	}
}
