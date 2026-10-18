package modelprovider

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelClientNegotiatesHTTP1WithVerifiedTLSOnHTTP2Server(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 1 {
			t.Error("model stream still negotiated HTTP/2")
		}
		w.Write([]byte("verified"))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client := (PublicEndpointPolicy{}).ModelClient()
	transport := client.Transport.(*http.Transport)
	if transport.Protocols == nil || !transport.Protocols.HTTP1() || transport.Protocols.HTTP2() {
		t.Fatal("model protocol policy not explicit")
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	if transport.ForceAttemptHTTP2 || len(transport.TLSNextProto) != 0 || transport.TLSClientConfig == nil || len(transport.TLSClientConfig.NextProtos) != 1 || transport.TLSClientConfig.NextProtos[0] != "http/1.1" {
		t.Fatal("cloned h2 negotiation leaked into the model transport")
	}
	// Preserve the production ALPN configuration: replacing the whole TLS
	// config here would hide an inherited-h2 regression.
	transport.TLSClientConfig.RootCAs = roots
	transport.TLSClientConfig.MinVersion = tls.VersionTLS12
	// The test owns this destination. Production still uses the unchanged
	// validated-public-address dialer; only the test transport is redirected.
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	response, err := client.Get("https://example.com/model-fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil || string(content) != "verified" || response.ProtoMajor != 1 {
		t.Fatal("verified HTTP/1 response missing", err)
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("TLS verification disabled")
	}
}

func TestGeneralClientRetainsDefaultProtocolNegotiation(t *testing.T) {
	client := (PublicEndpointPolicy{}).Client()
	transport := client.Transport.(*http.Transport)
	if transport.Protocols != nil || !transport.ForceAttemptHTTP2 {
		t.Fatal("general and customer MCP transport negotiation was changed")
	}
}
