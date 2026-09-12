package action

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/installinstruction"
)

func TestValidOneTimeResultPayloadRequiresCommandForLinux(t *testing.T) {
	set := validInstructionSet(installinstruction.ScopeLinuxSystem)
	set.BootstrapTLSMode = string(installinstruction.DownloadTLSInsecureFirstFetch)
	if !validOneTimeResultPayload([]installinstruction.Set{set}) {
		t.Fatal("valid Linux one-line instruction was rejected")
	}
	set.Command = ""
	if validOneTimeResultPayload([]installinstruction.Set{set}) {
		t.Fatal("Linux instruction without its one-line command was accepted")
	}
}

func TestValidOneTimeResultPayloadRequiresOneKubernetesCommand(t *testing.T) {
	set := validInstructionSet(installinstruction.ScopeKubernetes)
	if !validOneTimeResultPayload([]installinstruction.Set{set}) {
		t.Fatal("valid Kubernetes one-command instruction was rejected")
	}
	set.Command = ""
	if validOneTimeResultPayload([]installinstruction.Set{set}) {
		t.Fatal("Kubernetes instruction without its command was accepted")
	}
}

func TestValidOneTimeResultPayloadAcceptsWindowsSystem(t *testing.T) {
	set := validInstructionSet(installinstruction.ScopeWindowsSystem)
	set.Platform = "windows_amd64"
	set.Shell = "powershell"
	if !validOneTimeResultPayload([]installinstruction.Set{set}) {
		t.Fatal("valid Windows system instruction was rejected")
	}
}

func TestValidOneTimeResultKindsAreExplicit(t *testing.T) {
	if !validOneTimeResultKind(oneTimeResultKindConnectorInstall) || !validOneTimeResultKind(oneTimeResultKindHostRemoval) {
		t.Fatal("supported one-time command kind was rejected")
	}
	if validOneTimeResultKind("secret_export") {
		t.Fatal("unknown one-time result kind was accepted")
	}
}

func TestOneTimeResultPayloadPreservesScopeAcrossJSON(t *testing.T) {
	want := validInstructionSet(installinstruction.ScopeLinuxSystem)
	encoded, err := json.Marshal(oneTimeResultPayload{InstructionSets: []installinstruction.Set{want}})
	if err != nil {
		t.Fatal(err)
	}
	var decoded oneTimeResultPayload
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !validOneTimeResultPayload(decoded.InstructionSets) {
		t.Fatalf("instruction became invalid after JSON round trip: %#v", decoded.InstructionSets)
	}
}

func validInstructionSet(scope installinstruction.Scope) installinstruction.Set {
	set := installinstruction.Set{
		Scope: scope, Command: "install",
		ExpiresAt: time.Now().UTC().Add(time.Hour), TrustBundleEpoch: 1,
		TrustBundleSHA256: strings.Repeat("b", 64), BootstrapSHA256: strings.Repeat("c", 64), InstallerSHA256: strings.Repeat("a", 64), CapabilityWarnings: []string{},
	}
	if scope != installinstruction.ScopeKubernetes {
		set.BootstrapTLSMode = string(installinstruction.DownloadTLSStrict)
	}
	return set
}
