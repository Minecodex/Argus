package connector

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"testing"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
)

func TestBastionRelaySelectsPortsOnceAndKeepsThemAcrossRestart(t *testing.T) {
	conflict, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conflict.Close()
	httpsStart := conflict.Addr().(*net.TCPAddr).Port
	gatewayProbe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gatewayStart := gatewayProbe.Addr().(*net.TCPAddr).Port
	_ = gatewayProbe.Close()
	if gatewayStart == httpsStart || gatewayStart+defaultRelayPortAttempts > 65535 {
		gatewayStart = httpsStart - 100
	}
	store := localStore{directory: t.TempDir()}
	identity := identityState{Role: "bastion", EnrollmentEndpoint: "https://argus.example.test", GatewayEndpoint: "grpcs://connector.argus.example.test:9443"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	relay, err := startBastionRelayWithOptions(ctx, store, identity, logger, bastionRelayOptions{bindAddress: "127.0.0.1",
		httpsStartPort: httpsStart, gatewayStartPort: gatewayStart, portAttempts: defaultRelayPortAttempts, advertiseAddress: "10.0.0.8"})
	if err != nil {
		t.Fatal(err)
	}
	first := relay.snapshot()
	if first.GetStatus() != "ready" || first.GetGeneration() != 1 || first.GetAdvertiseAddress() != "10.0.0.8" {
		t.Fatalf("unexpected first relay status: %#v", first)
	}
	httpsPort, gatewayPort := relayPort(first, "https"), relayPort(first, "gateway")
	if httpsPort == uint32(httpsStart) || httpsPort < uint32(httpsStart) || gatewayPort < uint32(gatewayStart) {
		t.Fatalf("relay did not advance only the conflicting port: https=%d gateway=%d", httpsPort, gatewayPort)
	}
	relay.Close()

	restartConflict, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(httpsPort))))
	if err != nil {
		t.Fatal(err)
	}
	defer restartConflict.Close()
	restarted, err := startBastionRelayWithOptions(ctx, store, identity, logger, bastionRelayOptions{bindAddress: "127.0.0.1",
		httpsStartPort: httpsStart, gatewayStartPort: gatewayStart, portAttempts: defaultRelayPortAttempts, advertiseAddress: "10.0.0.9"})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	second := restarted.snapshot()
	if second.GetStatus() != "degraded" || second.GetGeneration() != 1 || second.GetAdvertiseAddress() != "10.0.0.8" ||
		relayPort(second, "https") != httpsPort || relayPort(second, "gateway") != gatewayPort || second.GetErrorCode() != "RELAY_PORT_IN_USE" {
		t.Fatalf("restart drifted from persisted relay state: %#v", second)
	}
}

func relayPort(status *connectorv1.BastionRelayStatus, kind string) uint32 {
	for _, listener := range status.GetListeners() {
		if listener.GetKind() == kind {
			return listener.GetPort()
		}
	}
	return 0
}

func TestReadTLSClientHelloExtractsPinnedSNIAndALPN(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	done := make(chan error, 1)
	go func() {
		configuration := &tls.Config{ServerName: "argus.example.com", NextProtos: []string{"h2", "http/1.1"}, InsecureSkipVerify: true} // test peer has no TLS server
		done <- tls.Client(client, configuration).Handshake()
	}()
	initial, sni, alpn, err := readTLSClientHello(server)
	if err != nil {
		t.Fatal(err)
	}
	if len(initial) < 5 || sni != "argus.example.com" || !containsAllowedALPN(alpn, map[string]bool{"h2": true}) {
		t.Fatalf("unexpected ClientHello evidence: sni=%q alpn=%v bytes=%d", sni, alpn, len(initial))
	}
	_ = server.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("TLS test client did not stop")
	}
}

func TestRelaySourceLimiterAppliesCIDRAndRate(t *testing.T) {
	limiter := &sourceRateLimiter{window: time.Now(), limit: 2, counts: map[netip.Addr]int{}, allowed: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}}
	allowed := netip.MustParseAddr("10.1.2.3")
	if !limiter.accept(allowed, time.Now()) || !limiter.accept(allowed, time.Now()) || limiter.accept(allowed, time.Now()) {
		t.Fatal("relay per-source rate limit was not enforced")
	}
	if limiter.accept(netip.MustParseAddr("203.0.113.10"), time.Now()) {
		t.Fatal("relay accepted a source outside the configured CIDRs")
	}
}

func TestDefaultArtifactEndpointMatchesDeploymentHostConvention(t *testing.T) {
	for input, expected := range map[string]string{
		"https://argus.dev":                  "https://artifacts.argus.dev",
		"https://enterprise.argus.localhost": "https://artifacts.argus.localhost",
	} {
		if actual := defaultArtifactEndpoint(input); actual != expected {
			t.Fatalf("defaultArtifactEndpoint(%q) = %q, want %q", input, actual, expected)
		}
	}
}
