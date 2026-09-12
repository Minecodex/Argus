package resource

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestSSHInstallationRequiresConnectionTestForItsControlPath(t *testing.T) {
	input := HostInput{Address: "node.test", Port: 22, Platform: "linux", Username: "root", SSHPath: "direct_executor", InstallMethod: "ssh", ControlPath: "direct"}
	for _, tc := range []struct {
		name, onboarding string
		want             bool
	}{
		{"SSH only cannot install", "", false},
		{"matching direct callback", `,"onboarding":{"control_path":"direct"}`, true},
		{"tunnel callback cannot prove direct", `,"onboarding":{"control_path":"executor_tunnel"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var plan connectionPlan
			if err := json.Unmarshal([]byte(`{"target_type":"host","address":"node.test","port":22,"platform":"linux","username":"root","ssh_path":"direct_executor"`+tc.onboarding+`}`), &plan); err != nil {
				t.Fatal(err)
			}
			if got := hostConnectionPlanMatches(plan, input); got != tc.want {
				t.Fatalf("connection test purpose match = %v, want %v", got, tc.want)
			}
		})
	}
}

type onboardingProbePlannerStub struct {
	current *installation.CallbackProbePlan
}

func (planner onboardingProbePlannerStub) PlanHostOnboardingProbe(context.Context, *db.Queries, uuid.UUID, string, string, uuid.NullUUID) (*installation.CallbackProbePlan, error) {
	return planner.current, nil
}

func TestOnboardingProofRequiresCurrentOriginTrustAndRelay(t *testing.T) {
	frozen := installation.CallbackProbePlan{ControlPath: "bastion_relay", EnrollmentEndpoint: "https://argus.test", GatewayEndpoint: "grpcs://gateway.test:9443", EnrollDialAddress: "10.0.0.2:8445", GatewayDialAddress: "10.0.0.2:9445", TrustBundlePEM: []byte("CA"), TrustBundleEpoch: 7, RelayPortGeneration: 4}
	input := HostInput{InstallMethod: "ssh", SSHPath: "bastion_connector", ControlPath: "bastion_relay"}
	evidence := ConnectionTestResult{CallbackVerified: true, CallbackControlPath: "bastion_relay"}
	for _, tc := range []struct {
		name   string
		mutate func(*installation.CallbackProbePlan)
	}{
		{"enrollment origin", func(p *installation.CallbackProbePlan) { p.EnrollmentEndpoint = "https://other.test" }},
		{"gateway origin", func(p *installation.CallbackProbePlan) { p.GatewayEndpoint = "grpcs://other.test:9443" }},
		{"CA material", func(p *installation.CallbackProbePlan) { p.TrustBundlePEM = []byte("new CA") }},
		{"CA epoch", func(p *installation.CallbackProbePlan) { p.TrustBundleEpoch++ }},
		{"relay generation", func(p *installation.CallbackProbePlan) { p.RelayPortGeneration++ }},
		{"relay enrollment address", func(p *installation.CallbackProbePlan) { p.EnrollDialAddress = "10.0.0.3:8445" }},
		{"relay gateway port", func(p *installation.CallbackProbePlan) { p.GatewayDialAddress = "10.0.0.2:9446" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := frozen
			tc.mutate(&current)
			service := Service{OnboardingProbes: onboardingProbePlannerStub{current: &current}}
			if err := service.validateOnboardingProbe(context.Background(), nil, uuid.New(), input, &frozen, evidence); err == nil {
				t.Fatal("stale callback test authorized installation")
			}
		})
	}
	service := Service{OnboardingProbes: onboardingProbePlannerStub{current: &frozen}}
	if err := service.validateOnboardingProbe(context.Background(), nil, uuid.New(), input, &frozen, evidence); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []ConnectionTestResult{{}, {CallbackVerified: true}, {CallbackVerified: true, CallbackControlPath: "direct"}, {CallbackControlPath: "bastion_relay"}} {
		if err := service.validateOnboardingProbe(context.Background(), nil, uuid.New(), input, &frozen, invalid); err == nil {
			t.Fatalf("incomplete evidence authorized installation: %+v", invalid)
		}
	}
}

func TestSSHOnlyConnectionTestStillMatchesRemoval(t *testing.T) {
	input := HostInput{Address: "node.test", Port: 22, Platform: "linux", Username: "root", SSHPath: "direct_executor"}
	plan := connectionPlan{TargetType: "host", Address: input.Address, Port: input.Port, Platform: input.Platform, Username: input.Username, SSHPath: input.SSHPath}
	if !hostConnectionPlanMatches(plan, input) {
		t.Fatal("SSH-only removal evidence was rejected")
	}
}
