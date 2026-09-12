package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestHostConnectionTestOnboardingPurposeContract(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "api/openapi/generated/argus.bundle.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	var request struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type string   `json:"type"`
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(document.Components.Schemas["HostConnectionTestCreate"], &request); err != nil {
		t.Fatal(err)
	}
	purpose, found := request.Properties["onboarding_control_path"]
	if !found || purpose.Type != "string" || !reflect.DeepEqual(purpose.Enum, []string{"direct", "bastion_relay", "executor_tunnel"}) {
		t.Fatalf("onboarding purpose enum changed: %+v", purpose)
	}
	if slices.Contains(request.Required, "onboarding_control_path") {
		t.Fatal("SSH-only removal tests must be able to omit onboarding purpose")
	}
	for _, private := range []string{"enrollment_endpoint", "gateway_endpoint", "trust_bundle_pem", "enroll_dial_address", "gateway_dial_address"} {
		if _, found := request.Properties[private]; found {
			t.Fatalf("browser can override authoritative callback field %s", private)
		}
	}
}

func TestCallbackProbeWireContractCarriesTypedProof(t *testing.T) {
	request := (&connectorv1.HostConnectionProbe{}).ProtoReflect().Descriptor().Fields().ByName("onboarding")
	if request == nil || request.Kind() != protoreflect.MessageKind || request.Message().FullName() != "argus.connector.v1.CallbackProbePlan" {
		t.Fatal("typed callback probe plan is missing")
	}
	fields := request.Message().Fields()
	for _, name := range []protoreflect.Name{"control_path", "enrollment_endpoint", "gateway_endpoint", "enroll_dial_address", "gateway_dial_address", "trust_bundle_pem", "trust_bundle_epoch", "relay_port_generation"} {
		if fields.ByName(name) == nil {
			t.Fatalf("callback plan omitted frozen field %s", name)
		}
	}
	if fields.Len() != 8 {
		t.Fatal("callback probe wire plan unexpectedly exposes additional routing fields")
	}
	result := (&connectorv1.HostConnectionProbeResult{}).ProtoReflect().Descriptor().Fields()
	if result.ByName("callback_verified").Kind() != protoreflect.BoolKind || result.ByName("callback_control_path").Kind() != protoreflect.StringKind {
		t.Fatal("callback outcome must carry explicit verified flag and control path")
	}
}
