package connector

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConnectorReleaseSelectsSignedArchitecture(t *testing.T) {
	manifest, err := json.Marshal(connectorReleaseManifest{SchemaVersion: "argus.connector_release/v3",
		SigningKeyID: "release-key", SigningPublicKey: strings.Repeat("a", 43), Installers: testConnectorInstallers(), Artifacts: []connectorReleaseArtifact{
			{Platform: "linux_amd64", URI: "https://artifacts.invalid/amd64", SHA256: strings.Repeat("a", 64),
				Signature: strings.Repeat("b", 86), SigningKeyID: "release-key", ByteSize: 100},
			{Platform: "linux_arm64", URI: "https://artifacts.invalid/arm64", SHA256: strings.Repeat("c", 64),
				Signature: strings.Repeat("d", 86), SigningKeyID: "release-key", ByteSize: 101},
			{Platform: "windows_amd64", URI: "https://artifacts.invalid/windows", SHA256: strings.Repeat("e", 64), Signature: strings.Repeat("f", 86), SigningKeyID: "release-key", ByteSize: 102},
		}})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := connectorArtifactForArchitecture(manifest, "arm64")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Platform != "linux_arm64" || artifact.SigningKeyID != "release-key" {
		t.Fatalf("artifact = %#v", artifact)
	}
	var invalid connectorReleaseManifest
	if json.Unmarshal(manifest, &invalid) != nil {
		t.Fatal("decode manifest")
	}
	invalid.Artifacts[0].SigningKeyID = "other-key"
	broken, _ := json.Marshal(invalid)
	if _, err = connectorArtifactForArchitecture(broken, "amd64"); err == nil {
		t.Fatal("artifact signed by a key outside the release trust record must be rejected")
	}
}

func TestModeAManifestRequiresBothArchitecturesAndInstallerURLs(t *testing.T) {
	manifest := connectorReleaseManifest{SchemaVersion: "argus.connector_release/v3", ManifestURI: "https://artifacts.invalid/manifest.json",
		Installers: testConnectorInstallers(), SigningKeyID: "release-key", SigningPublicKey: strings.Repeat("a", 43),
		Artifacts: []connectorReleaseArtifact{
			{Platform: "linux_amd64", URI: "https://artifacts.invalid/amd64", SHA256: strings.Repeat("a", 64), Signature: strings.Repeat("b", 86), SigningKeyID: "release-key", ByteSize: 100},
			{Platform: "linux_arm64", URI: "https://artifacts.invalid/arm64", SHA256: strings.Repeat("c", 64), Signature: strings.Repeat("d", 86), SigningKeyID: "release-key", ByteSize: 101},
			{Platform: "windows_amd64", URI: "https://artifacts.invalid/windows", SHA256: strings.Repeat("e", 64), Signature: strings.Repeat("f", 86), SigningKeyID: "release-key", ByteSize: 102},
		}}
	raw, _ := json.Marshal(manifest)
	if _, err := connectorManualInstallRelease(raw, "amd64"); err != nil {
		t.Fatalf("complete Mode A manifest rejected: %v", err)
	}
	manifest.Artifacts = manifest.Artifacts[:1]
	raw, _ = json.Marshal(manifest)
	if _, err := connectorManualInstallRelease(raw, "amd64"); err == nil {
		t.Fatal("Mode A manifest without arm64 was accepted")
	}
}

func testConnectorInstallers() []connectorReleaseInstaller {
	return []connectorReleaseInstaller{
		{Platform: "linux", Shell: "posix_sh", URI: "https://artifacts.invalid/install.sh", SHA256: strings.Repeat("1", 64)},
		{Platform: "windows", Shell: "powershell", URI: "https://artifacts.invalid/install.ps1", SHA256: strings.Repeat("2", 64)},
	}
}

func TestConnectorBootstrapScriptURLUsesEnrollmentOrigin(t *testing.T) {
	if got := connectorBootstrapScriptURL("https://argus.example.com/"); got != "https://argus.example.com/api/v1/connectors/bootstrap-script" {
		t.Fatalf("connectorBootstrapScriptURL() = %q", got)
	}
}
