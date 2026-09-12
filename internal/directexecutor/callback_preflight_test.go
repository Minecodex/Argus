package directexecutor

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/installation"
	"golang.org/x/crypto/ssh"
)

func TestSSHConnectionTestDoesNotIgnoreRequestedCallback(t *testing.T) {
	fixture := newCallbackTarget(t)
	plan := fixture.plan(t, `{"control_path":"direct","enrollment_endpoint":"https://unreachable.example.invalid","gateway_endpoint":"grpcs://unreachable.example.invalid:9443","trust_bundle_pem":"","trust_bundle_epoch":1}`)
	executor := &Executor{Timeout: time.Second}
	_, err := executor.probeSSH(context.Background(), plan, fixture.addresses(), "argus", []byte("password"))
	if err == nil {
		t.Fatal("SSH success incorrectly accepted requested but unavailable platform callbacks")
	}
	if !strings.HasPrefix(classifyConnectionError(err), "HOST_ONBOARDING_CALLBACK_") {
		t.Fatalf("callback failure lost its specific error code: %v", err)
	}
}

func TestGenericSSHConnectionTestNeedsNoPlatformCallback(t *testing.T) {
	fixture := newCallbackTarget(t)
	executor := &Executor{Timeout: time.Second}
	result, err := executor.probeSSH(context.Background(), fixture.plan(t, "null"), fixture.addresses(), "argus", []byte("password"))
	if err != nil || result.HostKeyFingerprint == "" || result.Architecture != "amd64" {
		t.Fatalf("generic SSH probe failed: result=%+v err=%v", result, err)
	}
}

func TestSSHCallbackPreflightUsesTargetDNSAndVerifiesBothOrigins(t *testing.T) {
	fixture := newCallbackTarget(t)
	server, ca := callbackPlatform(t, nil)
	fixture.routes["example.com:443"] = server.Listener.Addr().String()
	fixture.routes["example.com:9443"] = server.Listener.Addr().String()
	plan := fixture.plan(t, "null")
	plan.Onboarding = callbackPlan(ca, "direct")
	result, err := (&Executor{Timeout: time.Second}).probeSSH(context.Background(), plan, fixture.addresses(), "argus", []byte("password"))
	if err != nil || !result.CallbackVerified || result.CallbackControlPath != "direct" {
		t.Fatalf("callback result=%+v err=%v", result, err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if strings.Join(fixture.targets, ",") != "example.com:443,example.com:9443" {
		t.Fatalf("callbacks did not use both target-side public origins: %v", fixture.targets)
	}
}

func TestSSHTunnelPreflightWorksWithoutTargetPublicEgressAndClosesForwards(t *testing.T) {
	fixture := newCallbackTarget(t)
	server, ca := callbackPlatform(t, nil)
	plan := fixture.plan(t, "null")
	plan.Onboarding = callbackPlan(ca, "direct")
	executor := &Executor{Timeout: time.Second, ConnectorEnrollForwardTarget: server.Listener.Addr().String(), ConnectorGatewayForwardTarget: server.Listener.Addr().String()}
	if _, err := executor.probeSSH(context.Background(), plan, fixture.addresses(), "argus", []byte("password")); classifyConnectionError(err) != "HOST_ONBOARDING_CALLBACK_DNS_FAILED" {
		t.Fatalf("fixture should have no public target egress: %v", err)
	}
	plan.Onboarding.ControlPath = "executor_tunnel"
	result, err := executor.probeSSH(context.Background(), plan, fixture.addresses(), "argus", []byte("password"))
	if err != nil || !result.CallbackVerified || result.CallbackControlPath != "executor_tunnel" {
		t.Fatalf("end-to-end temporary tunnel failed: result=%+v err=%v", result, err)
	}
	if plan.Onboarding.EnrollDialAddress != "" || plan.Onboarding.GatewayDialAddress != "" {
		t.Fatal("ephemeral listener ports escaped into the frozen caller plan")
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.forwards) != 0 || fixture.acceptedForwards != 2 || fixture.canceledForwards != 2 {
		t.Fatalf("temporary listeners leaked: active=%d opened=%d closed=%d", len(fixture.forwards), fixture.acceptedForwards, fixture.canceledForwards)
	}
}

func TestSSHCallbackFailureKeepsEvidenceAndNeverMarksVerified(t *testing.T) {
	for _, test := range []struct {
		name, want string
		gateway    bool
		endpoint   string
	}{
		{name: "target DNS", want: "DNS_FAILED"},
		{name: "enrollment refused", endpoint: "refused", want: "CONNECT_FAILED"},
		{name: "enrollment wrong SNI", endpoint: "https://wrong.example", want: "TLS_FAILED"},
		{name: "Gateway refused", endpoint: "refused", gateway: true, want: "CONNECT_FAILED"},
		{name: "Gateway wrong SNI", endpoint: "grpcs://wrong.example:9443", gateway: true, want: "TLS_FAILED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCallbackTarget(t)
			server, ca := callbackPlatform(t, nil)
			plan := fixture.plan(t, "null")
			plan.Onboarding = callbackPlan(ca, "direct")
			if test.name != "target DNS" {
				fixture.routes["example.com:443"] = server.Listener.Addr().String()
				fixture.routes["example.com:9443"] = server.Listener.Addr().String()
			}
			if test.endpoint == "refused" {
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				closed := listener.Addr().String()
				_ = listener.Close()
				if test.gateway {
					fixture.routes["example.com:9443"] = closed
				} else {
					fixture.routes["example.com:443"] = closed
				}
			} else if test.endpoint != "" {
				if test.gateway {
					plan.Onboarding.GatewayEndpoint = test.endpoint
					fixture.routes["wrong.example:9443"] = server.Listener.Addr().String()
				} else {
					plan.Onboarding.EnrollmentEndpoint = test.endpoint
					fixture.routes["wrong.example:443"] = server.Listener.Addr().String()
				}
			}
			result, err := (&Executor{Timeout: time.Second}).probeSSH(context.Background(), plan, fixture.addresses(), "argus", []byte("password"))
			want := "HOST_ONBOARDING_CALLBACK_" + test.want
			if classifyConnectionError(err) != want || result.CallbackVerified || result.CallbackControlPath != "" || result.HostKeyFingerprint == "" {
				t.Fatalf("unsafe callback outcome: result=%+v err=%v, want %s", result, err, want)
			}
			last := result.Checks[len(result.Checks)-1]
			if last["status"] != "failed" || last["detail"] != want {
				t.Fatalf("missing safe failed check: %v", result.Checks)
			}
		})
	}
}

func TestSSHTunnelPreflightCleansUpPartialListenerFailure(t *testing.T) {
	fixture := newCallbackTarget(t)
	fixture.refuseForward = 2
	server, ca := callbackPlatform(t, nil)
	plan := fixture.plan(t, "null")
	plan.Onboarding = callbackPlan(ca, "executor_tunnel")
	executor := &Executor{Timeout: time.Second, ConnectorEnrollForwardTarget: server.Listener.Addr().String(), ConnectorGatewayForwardTarget: server.Listener.Addr().String()}
	if _, err := executor.probeSSH(context.Background(), plan, fixture.addresses(), "argus", []byte("password")); classifyConnectionError(err) != "HOST_ONBOARDING_CALLBACK_CONNECT_FAILED" {
		t.Fatalf("partial tunnel incorrectly accepted: %v", err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.forwards) != 0 || fixture.canceledForwards != 1 {
		t.Fatalf("first listener leaked after second failed: active=%d closed=%d", len(fixture.forwards), fixture.canceledForwards)
	}
}

func TestSSHTunnelPreflightCancellationClosesUpstream(t *testing.T) {
	fixture := newCallbackTarget(t)
	started, closed := make(chan struct{}), make(chan struct{})
	server, ca := callbackPlatform(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(closed) }))
	plan := fixture.plan(t, "null")
	plan.Onboarding = callbackPlan(ca, "executor_tunnel")
	executor := &Executor{Timeout: time.Second, ConnectorEnrollForwardTarget: server.Listener.Addr().String(), ConnectorGatewayForwardTarget: server.Listener.Addr().String()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := executor.probeSSH(ctx, plan, fixture.addresses(), "argus", []byte("password"))
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("callback never reached the upstream")
	}
	cancel()
	select {
	case err := <-done:
		if classifyConnectionError(err) != "HOST_ONBOARDING_CALLBACK_TIMEOUT" {
			t.Fatalf("canceled preflight = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled preflight hung")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("canceled preflight left its HTTPS upstream connected")
	}
	deadline := time.After(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		fixture.mu.Lock()
		active := len(fixture.forwards)
		fixture.mu.Unlock()
		if active == 0 {
			break
		}
		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatal("canceled preflight left target listeners open")
		}
	}
}

func TestSSHTunnelPreflightRejectsDisconnectedGatewayUpstream(t *testing.T) {
	fixture := newCallbackTarget(t)
	server, ca := callbackPlatform(t, nil)
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
	plan := fixture.plan(t, "null")
	plan.Onboarding = callbackPlan(ca, "executor_tunnel")
	executor := &Executor{Timeout: time.Second, ConnectorEnrollForwardTarget: server.Listener.Addr().String(), ConnectorGatewayForwardTarget: listener.Addr().String()}
	result, err := executor.probeSSH(context.Background(), plan, fixture.addresses(), "argus", []byte("password"))
	if classifyConnectionError(err) != "HOST_ONBOARDING_CALLBACK_CONNECT_FAILED" || result.CallbackVerified {
		t.Fatalf("Gateway upstream EOF accepted through remote TCP listener: result=%+v err=%v", result, err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.forwards) != 0 || fixture.canceledForwards != 2 {
		t.Fatalf("failed Gateway preflight leaked listeners: active=%d closed=%d", len(fixture.forwards), fixture.canceledForwards)
	}
}

func callbackPlan(ca []byte, path string) *installation.CallbackProbePlan {
	return &installation.CallbackProbePlan{ControlPath: path, EnrollmentEndpoint: "https://example.com", GatewayEndpoint: "grpcs://example.com:9443", TrustBundlePEM: ca, TrustBundleEpoch: 1}
}

func callbackPlatform(t *testing.T, handler http.Handler) (*httptest.Server, []byte) {
	t.Helper()
	if handler == nil {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Host != "example.com" || r.TLS.ServerName != "example.com" || r.URL.Path != "/api/v1/setup/status" {
				t.Errorf("callback changed identity/route: host=%q sni=%q path=%q", r.Host, r.TLS.ServerName, r.URL.Path)
			}
			_, _ = io.WriteString(w, `{"state":"initialized"}`)
		})
	}
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	return server, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
}

// The fixture accepts real SSH session and forwarding channels. Public callback
// names are resolved only here, allowing tests to distinguish target-side DNS
// and egress from the Executor's own network.
type callbackTarget struct {
	listener         net.Listener
	host             string
	port             int
	mu               sync.Mutex
	forwards         map[string]net.Listener
	targets          []string
	routes           map[string]string
	acceptedForwards int
	canceledForwards int
	refuseForward    int
}

func newCallbackTarget(t *testing.T) *callbackTarget {
	t.Helper()
	interfaces, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	host := ""
	for _, address := range interfaces {
		prefix, err := netip.ParsePrefix(address.String())
		if err == nil && prefix.Addr().Is4() && prefix.Addr().IsGlobalUnicast() && !prefix.Addr().IsLoopback() {
			host = prefix.Addr().String()
			break
		}
	}
	if host == "" {
		t.Fatal("SSH preflight fixture requires a local non-loopback IPv4 interface")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Fatal(err)
	}
	fixture := &callbackTarget{listener: listener, host: host, port: listener.Addr().(*net.TCPAddr).Port, forwards: map[string]net.Listener{}, routes: map[string]string{}}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if meta.User() != "argus" || string(password) != "password" {
			return nil, errors.New("authentication failed")
		}
		return nil, nil
	}}
	config.AddHostKey(signer)
	t.Cleanup(func() {
		_ = listener.Close()
		fixture.mu.Lock()
		defer fixture.mu.Unlock()
		for _, forward := range fixture.forwards {
			_ = forward.Close()
		}
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go fixture.serve(conn, config)
		}
	}()
	return fixture
}

func (fixture *callbackTarget) addresses() []netip.Addr {
	return []netip.Addr{netip.MustParseAddr(fixture.host)}
}

func (fixture *callbackTarget) plan(t *testing.T, onboarding string) connectionPlan {
	t.Helper()
	raw := `{"address":"` + fixture.host + `","port":` + strconv.Itoa(fixture.port) + `,"platform":"linux","onboarding":` + onboarding + `}`
	var plan connectionPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		t.Fatal(err)
	}
	return plan
}

func (fixture *callbackTarget) serve(conn net.Conn, config *ssh.ServerConfig) {
	server, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer server.Close()
	go fixture.serveRequests(server, requests)
	for incoming := range channels {
		switch incoming.ChannelType() {
		case "session":
			channel, requests, err := incoming.Accept()
			if err != nil {
				continue
			}
			go func() {
				defer channel.Close()
				for request := range requests {
					if request.Type != "exec" {
						_ = request.Reply(false, nil)
						continue
					}
					_ = request.Reply(true, nil)
					_, _ = io.WriteString(channel, "architecture=x86_64\ndistribution_version=ubuntu:24.04\nservice_manager=systemd\nprivileged=true\nfree_disk_bytes=1073741824\n")
					_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
					return
				}
			}()
		case "direct-tcpip":
			go fixture.serveDirect(incoming)
		default:
			_ = incoming.Reject(ssh.Prohibited, "unsupported channel")
		}
	}
}

func (fixture *callbackTarget) serveDirect(incoming ssh.NewChannel) {
	var target struct {
		Host       string
		Port       uint32
		Origin     string
		OriginPort uint32
	}
	if ssh.Unmarshal(incoming.ExtraData(), &target) != nil {
		_ = incoming.Reject(ssh.Prohibited, "invalid target")
		return
	}
	address := net.JoinHostPort(target.Host, strconv.Itoa(int(target.Port)))
	fixture.mu.Lock()
	fixture.targets = append(fixture.targets, address)
	upstreamAddress, mapped := fixture.routes[address]
	_, forwarded := fixture.forwards[address]
	fixture.mu.Unlock()
	if !mapped && !forwarded {
		_ = incoming.Reject(ssh.ConnectionFailed, "dial tcp: lookup target: no such host")
		return
	}
	if forwarded {
		upstreamAddress = address
	}
	upstream, err := net.Dial("tcp", upstreamAddress)
	if err != nil {
		_ = incoming.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	channel, requests, err := incoming.Accept()
	if err != nil {
		_ = upstream.Close()
		return
	}
	go ssh.DiscardRequests(requests)
	copyCallbackConnection(channel, upstream)
}

func (fixture *callbackTarget) serveRequests(server *ssh.ServerConn, requests <-chan *ssh.Request) {
	owned := map[string]net.Listener{}
	defer func() {
		fixture.mu.Lock()
		defer fixture.mu.Unlock()
		for address, listener := range owned {
			_ = listener.Close()
			if fixture.forwards[address] == listener {
				delete(fixture.forwards, address)
			}
		}
	}()
	for request := range requests {
		var bind struct {
			Host string
			Port uint32
		}
		if ssh.Unmarshal(request.Payload, &bind) != nil || bind.Host != "127.0.0.1" {
			_ = request.Reply(false, nil)
			continue
		}
		address := net.JoinHostPort(bind.Host, strconv.Itoa(int(bind.Port)))
		switch request.Type {
		case "tcpip-forward":
			fixture.mu.Lock()
			refuse := fixture.refuseForward > 0 && fixture.acceptedForwards+1 == fixture.refuseForward
			fixture.mu.Unlock()
			if refuse {
				_ = request.Reply(false, nil)
				continue
			}
			listener, err := net.Listen("tcp", address)
			if err != nil {
				_ = request.Reply(false, nil)
				continue
			}
			fixture.mu.Lock()
			fixture.forwards[listener.Addr().String()] = listener
			owned[listener.Addr().String()] = listener
			fixture.acceptedForwards++
			fixture.mu.Unlock()
			port := uint32(listener.Addr().(*net.TCPAddr).Port)
			_ = request.Reply(true, ssh.Marshal(struct{ Port uint32 }{port}))
			go func() {
				for {
					connection, err := listener.Accept()
					if err != nil {
						return
					}
					go func() {
						channel, reqs, err := server.OpenChannel("forwarded-tcpip", ssh.Marshal(struct {
							Host       string
							Port       uint32
							Origin     string
							OriginPort uint32
						}{"127.0.0.1", port, "127.0.0.1", 12345}))
						if err != nil {
							_ = connection.Close()
							return
						}
						go ssh.DiscardRequests(reqs)
						copyCallbackConnection(channel, connection)
					}()
				}
			}()
		case "cancel-tcpip-forward":
			fixture.mu.Lock()
			listener := fixture.forwards[address]
			delete(fixture.forwards, address)
			delete(owned, address)
			fixture.canceledForwards++
			fixture.mu.Unlock()
			if listener != nil {
				_ = listener.Close()
			}
			_ = request.Reply(true, nil)
		default:
			_ = request.Reply(false, nil)
		}
	}
}

func copyCallbackConnection(channel ssh.Channel, connection net.Conn) {
	defer channel.Close()
	defer connection.Close()
	go func() { _, _ = io.Copy(connection, channel); _ = connection.Close() }()
	_, _ = io.Copy(channel, connection)
}
