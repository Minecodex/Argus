package resource

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestHostConnectionPlanBindsEverySSHInput(t *testing.T) {
	credentialID := uuid.MustParse("018f47e2-9a4c-7b31-8acd-02a2475e8d2f")
	scopeID := uuid.MustParse("018f47e2-9a4c-7b31-8acd-02a2475e8d30")
	input := HostInput{Address: "host.example", Port: 22, Platform: "linux", Username: "argus", SSHPath: "bastion_connector",
		BastionScopeID: uuid.NullUUID{UUID: scopeID, Valid: true}, CredentialID: uuid.NullUUID{UUID: credentialID, Valid: true}}
	plan := connectionPlan{TargetType: "host", Address: input.Address, Port: input.Port, Platform: input.Platform, Username: input.Username,
		SSHPath: input.SSHPath, BastionScopeID: input.BastionScopeID, CredentialID: input.CredentialID}
	if !hostConnectionPlanMatches(plan, input) {
		t.Fatal("matching Host connection plan was rejected")
	}
	mutations := []HostInput{input, input, input, input, input, input, input}
	mutations[0].Address = "other.example"
	mutations[1].Port = 2222
	mutations[2].Platform = "windows"
	mutations[3].Username = "root"
	mutations[4].SSHPath = "direct_executor"
	mutations[5].BastionScopeID.UUID = uuid.New()
	mutations[6].CredentialID.UUID = uuid.New()
	for index, mutation := range mutations {
		if hostConnectionPlanMatches(plan, mutation) {
			t.Fatalf("mutation %d reused a Connection Test for different Host input", index)
		}
	}
}

func TestBastionConnectionPlanHashSurvivesJSONRoundTrip(t *testing.T) {
	plan := connectionPlan{TargetType: "host", Address: "198.51.100.25", Port: 22, Platform: "linux", Username: "root",
		SSHPath: "bastion_connector", BastionScopeID: uuid.NullUUID{UUID: uuid.New(), Valid: true},
		ConnectorID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, CredentialID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, CredentialVersion: 1}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(encoded)
	var decoded connectionPlan
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	roundTrip, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	got := sha256.Sum256(roundTrip)
	if !bytes.Equal(got[:], want[:]) {
		t.Fatalf("connection plan changed across JSON round trip: %s != %s", roundTrip, encoded)
	}
}

func TestHostPendingPlanBindsConnectionTestEvidence(t *testing.T) {
	plan := hostActionPlan{PinnedKey: "SHA256:expected", Arch: "arm64", DistributionVersion: "ubuntu:24.04", ServiceManager: "systemd", FreeDiskBytes: 1 << 30}
	valid := ConnectionTestResult{HostKeyFingerprint: "SHA256:expected", Architecture: "arm64", DistributionVersion: "ubuntu:24.04", ServiceManager: "systemd", FreeDiskBytes: 1 << 30, Privileged: true}
	if !hostConnectionEvidenceMatches(plan, valid) {
		t.Fatal("matching Host key and architecture evidence was rejected")
	}
	for _, result := range []ConnectionTestResult{
		{HostKeyFingerprint: "", Architecture: "arm64"},
		{HostKeyFingerprint: "SHA256:changed", Architecture: "arm64"},
		{HostKeyFingerprint: "SHA256:expected", Architecture: "amd64"},
		{HostKeyFingerprint: "SHA256:expected", Architecture: "arm64", DistributionVersion: "ubuntu:22.04", ServiceManager: "systemd", FreeDiskBytes: 1 << 30, Privileged: true},
	} {
		if hostConnectionEvidenceMatches(plan, result) {
			t.Fatalf("changed connection evidence was accepted: %+v", result)
		}
	}
}

func TestManualHostOnboardingRejectsSSHInputs(t *testing.T) {
	valid := HostInput{Role: "managed_host", Platform: "linux", Architecture: "amd64", ControlPath: "direct", InstallMethod: "manual", SSHPath: "none"}
	if err := validateHostOnboardingInput(valid); err != nil {
		t.Fatalf("valid manual onboarding rejected: %v", err)
	}
	invalid := valid
	invalid.Address = "10.0.0.10"
	if err := validateHostOnboardingInput(invalid); err == nil {
		t.Fatal("manual onboarding accepted SSH-only address input")
	}
}

func TestKubernetesNetworkPathChangeDetection(t *testing.T) {
	credentialID := uuid.New()
	current := db.KubernetesCluster{ApiServer: "https://api.example:6443", ConnectionMode: "direct",
		CredentialID: uuid.NullUUID{UUID: credentialID, Valid: true}}
	if kubernetesNetworkPathChanged(current, KubernetesInput{Name: "renamed"}) {
		t.Fatal("metadata-only Kubernetes update was treated as a path change")
	}
	if !kubernetesNetworkPathChanged(current, KubernetesInput{APIServer: "https://other.example:6443"}) {
		t.Fatal("API server change was not detected")
	}
}
