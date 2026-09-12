package argusdev

import "testing"

func TestWindowsHostE2EConfigRejectsInlineCredentialsAndUnpinnedHosts(t *testing.T) {
	valid := windowsHostE2EConfig{SchemaVersion: windowsHostE2ESchema, Targets: []windowsHostE2ETarget{{
		Name: "server-2019-direct", Scenario: "manual_direct", Address: "192.0.2.10", Port: 22, Username: "Administrator",
		HostKeySHA256: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", PasswordEnv: "ARGUS_WINDOWS_2019_PASSWORD", InstallCommandEnv: "ARGUS_WINDOWS_2019_COMMAND", MinimumBuild: 17763,
	}}}
	if err := validateWindowsHostE2EConfig(valid); err != nil {
		t.Fatalf("valid Windows E2E config rejected: %v", err)
	}
	invalid := valid
	invalid.Targets = append([]windowsHostE2ETarget(nil), valid.Targets...)
	invalid.Targets[0].HostKeySHA256 = ""
	if err := validateWindowsHostE2EConfig(invalid); err == nil {
		t.Fatal("unpinned Windows SSH host was accepted")
	}
	invalid = valid
	invalid.Targets = append([]windowsHostE2ETarget(nil), valid.Targets...)
	invalid.Targets[0].PrivateKeyFile = "private-key.pem"
	if err := validateWindowsHostE2EConfig(invalid); err == nil {
		t.Fatal("ambiguous Windows SSH credential sources were accepted")
	}
}
