package sshtarget

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// An accepted remote-forward socket does not prove its upstream is reachable.
func TestEnrollmentProbeRejectsTunnelWithDisconnectedUpstream(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			_ = connection.Close()
		}
	}()
	client := callbackSSHClient(t)
	if err = ProbeCallbacks(context.Background(), client, []string{listener.Addr().String()}); err != nil {
		t.Fatalf("fixture must reproduce successful TCP-only preflight: %v", err)
	}
	server, ca := callbackHTTPServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	_ = server
	err = ProbeEnrollment(context.Background(), client, "https://example.com", listener.Addr().String(), ca)
	if err == nil || CallbackFailureCode(err) != "CONNECT_FAILED" {
		t.Fatalf("disconnected tunnel accepted or not classified: %v", err)
	}
}

func TestEnrollmentProbePreservesTLSIdentityAndChecksPlatform(t *testing.T) {
	client := callbackSSHClient(t)
	server, ca := callbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "example.com" || r.TLS.ServerName != "example.com" || r.URL.Path != "/api/v1/setup/status" {
			t.Errorf("probe changed origin, SNI or route: host=%q sni=%q path=%q", r.Host, r.TLS.ServerName, r.URL.Path)
		}
		if r.Header.Get("X-Argus-Enrollment-Token") != "" {
			t.Error("probe must not enroll")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"state":"initialized","platform_name":"test"}`)
	}))
	// example.com is resolved only by the target dial override. A proxy must not
	// cause an enrollment probe to escape the frozen SSH path.
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	if err := ProbeEnrollment(context.Background(), client, "https://example.com", server.Listener.Addr().String(), ca); err != nil {
		t.Fatal(err)
	}
}

func TestEnrollmentProbeRejectsTLSRedirectAndWrongApplication(t *testing.T) {
	for _, test := range []struct {
		name, endpoint, body, location, want string
		status                               int
		untrusted                            bool
	}{
		{name: "wrong certificate name", endpoint: "https://wrong.example", status: 200, body: `{"state":"initialized"}`, want: "TLS_FAILED"},
		{name: "untrusted CA", status: 200, body: `{"state":"initialized"}`, untrusted: true, want: "TLS_FAILED"},
		{name: "redirect", status: 302, location: "https://other.example/private", want: "HTTP_FAILED"},
		{name: "unavailable", status: 503, want: "HTTP_FAILED"},
		{name: "HTML frontend", status: 200, body: "<html>index</html>", want: "RESPONSE_INVALID"},
		{name: "not initialized", status: 200, body: `{"state":"uninitialized"}`, want: "RESPONSE_INVALID"},
		{name: "trailing JSON", status: 200, body: `{"state":"initialized"}{}`, want: "RESPONSE_INVALID"},
		{name: "oversized", status: 200, body: strings.Repeat("x", 4097), want: "RESPONSE_INVALID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := callbackSSHClient(t)
			server, ca := callbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if test.location != "" {
					w.Header().Set("Location", test.location)
				}
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			if test.untrusted {
				ca = unrelatedCallbackCA(t)
			}
			endpoint := test.endpoint
			if endpoint == "" {
				endpoint = "https://example.com"
			}
			err := ProbeEnrollment(context.Background(), client, endpoint, server.Listener.Addr().String(), ca)
			if err == nil || CallbackFailureCode(err) != test.want {
				t.Fatalf("got %v (%s), want %s", err, CallbackFailureCode(err), test.want)
			}
		})
	}
}

func TestEnrollmentProbeRejectsUnsafeConfigurationWithoutExposingSecrets(t *testing.T) {
	client := callbackSSHClient(t)
	_, ca := callbackHTTPServer(t, http.NotFoundHandler())
	for _, test := range []struct{ endpoint, address string }{
		{"http://example.com", "127.0.0.1:8443"},
		{"https://secret:password@example.com", "127.0.0.1:8443"},
		{"https://example.com?token=secret", "127.0.0.1:8443"},
		{"https://example.com", "missing-port"},
	} {
		err := ProbeEnrollment(context.Background(), client, test.endpoint, test.address, ca)
		if CallbackFailureCode(err) != "CONFIG_INVALID" || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe configuration was accepted or exposed: %v", err)
		}
	}
}

func unrelatedCallbackCA(t *testing.T) []byte {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, public, private)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestEnrollmentProbeHonorsCancellation(t *testing.T) {
	client := callbackSSHClient(t)
	server, ca := callbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := ProbeEnrollment(ctx, client, "https://example.com", server.Listener.Addr().String(), ca)
	if CallbackFailureCode(err) != "TIMEOUT" {
		t.Fatalf("timeout not classified: %v", err)
	}
}

func TestGatewayProbeVerifiesServerTLSOverTargetSSH(t *testing.T) {
	for _, test := range []struct {
		name, endpoint, want string
		untrusted, mtls      bool
	}{
		{name: "trusted server", endpoint: "grpcs://example.com:9443"},
		{name: "client certificate is not issued yet", endpoint: "grpcs://example.com:9443", mtls: true},
		{name: "wrong SNI", endpoint: "grpcs://wrong.example:9443", want: "TLS_FAILED"},
		{name: "wrong CA", endpoint: "grpcs://example.com:9443", untrusted: true, want: "TLS_FAILED"},
		{name: "unsafe scheme", endpoint: "https://example.com:9443", want: "CONFIG_INVALID"},
		{name: "unsafe user info", endpoint: "grpcs://secret:password@example.com:9443", want: "CONFIG_INVALID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewUnstartedServer(http.NotFoundHandler())
			server.EnableHTTP2 = true
			server.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
			if test.mtls {
				server.TLS.ClientAuth = tls.RequireAndVerifyClientCert
			}
			server.StartTLS()
			defer server.Close()
			ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
			if test.untrusted {
				ca = unrelatedCallbackCA(t)
			}
			err := ProbeGateway(context.Background(), callbackSSHClient(t), test.endpoint, server.Listener.Addr().String(), ca)
			if CallbackFailureCode(err) != test.want {
				t.Fatalf("got %v, want %q", err, test.want)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatalf("unsafe origin leaked: %v", err)
			}
		})
	}
}

func TestGatewayProbeRejectsTCPAcceptanceWithoutTLS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		connection, err := listener.Accept()
		if err == nil {
			_ = connection.Close()
		}
	}()
	_, ca := callbackHTTPServer(t, http.NotFoundHandler())
	err = ProbeGateway(context.Background(), callbackSSHClient(t), "grpcs://example.com:9443", listener.Addr().String(), ca)
	if CallbackFailureCode(err) != "CONNECT_FAILED" {
		t.Fatalf("TCP-only Gateway accepted: %v", err)
	}
}

func TestGatewayProbeRejectsNonGRPCTLSProtocol(t *testing.T) {
	server, ca := callbackHTTPServer(t, http.NotFoundHandler())
	err := ProbeGateway(context.Background(), callbackSSHClient(t), "grpcs://example.com:9443", server.Listener.Addr().String(), ca)
	if CallbackFailureCode(err) != "TLS_FAILED" {
		t.Fatalf("Gateway without h2 accepted: %v", err)
	}
}

func callbackHTTPServer(t *testing.T, handler http.Handler) (*httptest.Server, []byte) {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	server.StartTLS()
	t.Cleanup(server.Close)
	certificate, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return server, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
}

func callbackSSHClient(t *testing.T) *ssh.Client {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			_ = conn.Close()
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		for incoming := range channels {
			var target struct {
				Host       string
				Port       uint32
				Origin     string
				OriginPort uint32
			}
			if incoming.ChannelType() != "direct-tcpip" || ssh.Unmarshal(incoming.ExtraData(), &target) != nil || target.Host != "127.0.0.1" {
				_ = incoming.Reject(ssh.Prohibited, "unsupported test target")
				continue
			}
			upstream, err := net.Dial("tcp", net.JoinHostPort(target.Host, strconv.Itoa(int(target.Port))))
			if err != nil {
				_ = incoming.Reject(ssh.ConnectionFailed, "upstream unavailable")
				continue
			}
			channel, reqs, err := incoming.Accept()
			if err != nil {
				_ = upstream.Close()
				continue
			}
			go ssh.DiscardRequests(reqs)
			go func() {
				defer channel.Close()
				defer upstream.Close()
				go func() { _, _ = io.Copy(upstream, channel); _ = upstream.Close() }()
				_, _ = io.Copy(channel, upstream)
			}()
		}
	}()
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{User: "probe", HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}
