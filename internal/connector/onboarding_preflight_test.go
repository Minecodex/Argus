package connector

import (
	"encoding/json"
	"testing"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestHostProbeCommandPreservesAuthoritativeCallbackPlan(t *testing.T) {
	plan := installation.CallbackProbePlan{ControlPath: "bastion_relay", EnrollmentEndpoint: "https://argus.test", GatewayEndpoint: "grpcs://gateway.test:9443", EnrollDialAddress: "10.0.0.2:8445", GatewayDialAddress: "10.0.0.2:9445", TrustBundlePEM: []byte("trusted CA"), TrustBundleEpoch: 7, RelayPortGeneration: 4}
	payload, _ := json.Marshal(map[string]any{"address": "node.test", "port": 22, "platform": "linux", "username": "root", "onboarding": plan})
	command, err := typedCommand(db.ConnectorCommand{CommandType: "host_connection_probe", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	var probe connectorv1.HostConnectionProbe
	if err := command.TypedPayload.UnmarshalTo(&probe); err != nil {
		t.Fatal(err)
	}
	got := probe.GetOnboarding()
	if got == nil || got.ControlPath != plan.ControlPath || got.EnrollmentEndpoint != plan.EnrollmentEndpoint || got.GatewayEndpoint != plan.GatewayEndpoint || got.EnrollDialAddress != plan.EnrollDialAddress || got.GatewayDialAddress != plan.GatewayDialAddress || string(got.TrustBundlePem) != string(plan.TrustBundlePEM) || got.TrustBundleEpoch != 7 || got.RelayPortGeneration != 4 {
		t.Fatalf("authoritative callback plan was dropped or changed: %v", got)
	}
}

func TestCallbackRequestedCannotSucceedWithoutExactTypedEvidence(t *testing.T) {
	raw := []byte(`{"onboarding":{"control_path":"bastion_relay"}}`)
	for _, value := range []*connectorv1.HostConnectionProbeResult{
		{}, {CallbackVerified: true}, {CallbackVerified: true, CallbackControlPath: "direct"},
	} {
		typed, _ := anypb.New(value)
		result, status, code, err := hostProbeOutcome(raw, "succeeded", typed, "")
		if err != nil || status != "failed" || code != "HOST_ONBOARDING_CALLBACK_RESPONSE_INVALID" || result.CallbackVerified {
			t.Fatalf("missing callback evidence accepted: status=%s code=%s result=%+v err=%v", status, code, result, err)
		}
	}
	typed, _ := anypb.New(&connectorv1.HostConnectionProbeResult{CallbackVerified: true, CallbackControlPath: "bastion_relay"})
	if _, status, code, err := hostProbeOutcome(raw, "succeeded", typed, ""); err != nil || status != "succeeded" || code != "" {
		t.Fatalf("valid callback rejected: %s %s %v", status, code, err)
	}
	if _, status, _, err := hostProbeOutcome([]byte(`{}`), "succeeded", mustHostProbeAny(t, &connectorv1.HostConnectionProbeResult{}), ""); err != nil || status != "succeeded" {
		t.Fatalf("SSH-only removal probe rejected: %s %v", status, err)
	}
}

func TestCallbackFailureProducesSafeFailedCheck(t *testing.T) {
	for _, code := range []string{"HOST_ONBOARDING_CALLBACK_DNS_FAILED", "HOST_ONBOARDING_CALLBACK_untrusted remote output"} {
		result, status, safeCode, err := hostProbeOutcome([]byte(`{"onboarding":{"control_path":"direct"}}`), "failed", nil, code)
		if err != nil || status != "failed" || result.CallbackVerified || len(result.Checks) != 1 || result.Checks[0]["status"] != "failed" || result.Checks[0]["detail"] != safeCode {
			t.Fatalf("callback failure projection = %+v %s %s %v", result, status, safeCode, err)
		}
		if code != "HOST_ONBOARDING_CALLBACK_DNS_FAILED" && safeCode != "HOST_ONBOARDING_CALLBACK_RESPONSE_INVALID" {
			t.Fatalf("unsafe code leaked: %s", safeCode)
		}
	}
}

func TestSSHFailureDoesNotInventPassedTargetChecks(t *testing.T) {
	result, status, _, err := hostProbeOutcome([]byte(`{"onboarding":{"control_path":"direct"}}`), "failed", mustHostProbeAny(t, &connectorv1.HostConnectionProbeResult{}), "SSH_AUTH_FAILED")
	if err != nil || status != "failed" || len(result.Checks) != 0 || result.CallbackVerified {
		t.Fatalf("failed SSH authentication invented successful evidence: %+v %s %v", result, status, err)
	}
}

func mustHostProbeAny(t *testing.T, result *connectorv1.HostConnectionProbeResult) *anypb.Any {
	t.Helper()
	value, err := anypb.New(result)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestHostProbeProjectionRequiresExplicitCallbackEvidence(t *testing.T) {
	result := hostProbeConnectionTestResult(&connectorv1.HostConnectionProbeResult{CallbackVerified: true, CallbackControlPath: "bastion_relay"})
	if !result.CallbackVerified || result.CallbackControlPath != "bastion_relay" {
		t.Fatalf("callback evidence lost: %+v", result)
	}
	for _, check := range result.Checks {
		if check["name"] == "onboarding_callback" && check["status"] == "passed" {
			return
		}
	}
	t.Fatal("callback success was not projected into checks")
}
