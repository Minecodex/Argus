package connector

import (
	"context"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/sshtarget"
)

func TestBastionConnectionTestRequiresRequestedPlatformCallback(t *testing.T) {
	host, port, fingerprint := startSSHServer(t, "connector-password")
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	request := &connectorv1.HostConnectionProbe{Address: host, Port: port, Protocol: "ssh", Username: "argus", Platform: "linux", ExpectedHostKeyFingerprint: fingerprint,
		Onboarding: &connectorv1.CallbackProbePlan{ControlPath: "direct", EnrollmentEndpoint: server.URL, GatewayEndpoint: "grpcs://example.com:9443", TrustBundleEpoch: 1,
			TrustBundlePem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})}}
	payload, err := anypb.New(request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := executeHostProbe(context.Background(), payload, []byte("connector-password"))
	if sshtarget.CallbackFailureCode(err) != "CONNECT_FAILED" {
		t.Fatalf("Bastion SSH probe accepted unavailable requested callback: result=%+v err=%v", result, err)
	}
	if result == nil || result.HostKeyFingerprint != fingerprint || result.CallbackVerified {
		t.Fatalf("callback failure must retain SSH evidence without marking verified: %+v", result)
	}
	outcome := (commandExecutor{}).execute(context.Background(), &connectorv1.ConnectorCommand{CommandId: "probe", CommandType: "host_connection_probe", TypedPayload: payload, ExpiresAt: timestamppb.New(time.Now().Add(time.Minute))}, []byte("connector-password"), nil)
	if outcome.code != "HOST_ONBOARDING_CALLBACK_CONNECT_FAILED" || outcome.result == nil {
		t.Fatalf("callback failure lost its typed result or code: %+v", outcome)
	}
	var evidence connectorv1.HostConnectionProbeResult
	if err := outcome.result.UnmarshalTo(&evidence); err != nil || evidence.CallbackVerified || evidence.HostKeyFingerprint != fingerprint {
		t.Fatalf("invalid callback failure wire evidence: %+v err=%v", &evidence, err)
	}
}

func TestBastionRelayPreflightUsesFrozenMemberListenersAndPublicTLSIdentity(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "example.com" || r.TLS.ServerName != "example.com" || r.URL.Path != "/api/v1/setup/status" {
			t.Errorf("relay changed callback origin: host=%q sni=%q path=%q", r.Host, r.TLS.ServerName, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"state":"initialized"}`)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	// Only the persisted member-facing listener addresses work on the target.
	// Resolving/dialing either public origin directly must fail in this fixture.
	host, port, fingerprint := startSSHServerWithCallbacks(t, "connector-password", map[string]string{
		"bastion.member.invalid:8447": server.Listener.Addr().String(),
		"bastion.member.invalid:9447": server.Listener.Addr().String(),
	})
	request := &connectorv1.HostConnectionProbe{Address: host, Port: port, Protocol: "ssh", Username: "argus", Platform: "linux", ExpectedHostKeyFingerprint: fingerprint,
		Onboarding: &connectorv1.CallbackProbePlan{ControlPath: "bastion_relay", EnrollmentEndpoint: "https://example.com", GatewayEndpoint: "grpcs://example.com:9443", TrustBundleEpoch: 1, RelayPortGeneration: 3,
			EnrollDialAddress: "bastion.member.invalid:8447", GatewayDialAddress: "bastion.member.invalid:9447", TrustBundlePem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})}}
	payload, err := anypb.New(request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := executeHostProbe(context.Background(), payload, []byte("connector-password"))
	if err != nil || !result.CallbackVerified || result.CallbackControlPath != "bastion_relay" || result.HostKeyFingerprint != fingerprint {
		t.Fatalf("frozen member relay callback failed: result=%+v err=%v", result, err)
	}
	request.Onboarding.RelayPortGeneration = 0
	payload, _ = anypb.New(request)
	result, err = executeHostProbe(context.Background(), payload, []byte("connector-password"))
	if sshtarget.CallbackFailureCode(err) != "CONFIG_INVALID" || result.CallbackVerified {
		t.Fatalf("unfrozen relay generation accepted: result=%+v err=%v", result, err)
	}
}
