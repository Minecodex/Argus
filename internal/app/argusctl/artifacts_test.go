package argusctl

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredArtifactObjectAcceptsOnlyBundledBucket(t *testing.T) {
	cfg := &InstallConfig{Spec: InstallSpec{Exposure: Exposure{EnterpriseHost: "argus.example.com", ArtifactHost: "artifacts.example.com"}}}
	bucket, key, err := configuredArtifactObject(cfg, "https://artifacts.example.com/argus-collector-artifacts/releases/linux.tar.gz", collectorArtifactBucket)
	if err != nil || bucket != collectorArtifactBucket || key != "releases/linux.tar.gz" {
		t.Fatalf("object location = %q %q %v", bucket, key, err)
	}
	if _, _, err = configuredArtifactObject(cfg, "https://external.example.com/argus-collector-artifacts/releases/linux.tar.gz", collectorArtifactBucket); err == nil {
		t.Fatal("external artifact host was accepted as bundled storage")
	}
}

func TestLoadConnectorInstallArtifactsVerifiesPreSignedBundle(t *testing.T) {
	t.Setenv("ARGUS_OTELCOL_SIGNING_PRIVATE_KEY", "")
	root := t.TempDir()
	directory := filepath.Join(root, "build", "connector", "artifacts")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &InstallConfig{Spec: InstallSpec{Images: Images{Tag: "v0.1.0-beta"}, Telemetry: TelemetryArtifacts{
		SigningKeyID: "release-key", SigningPublicKey: base64.RawStdEncoding.EncodeToString(publicKey),
	}}}
	document := connectorArtifactSignatures{SchemaVersion: connectorSignaturesV1, ImageTag: cfg.Spec.Images.Tag}
	for _, target := range connectorArtifactTargets {
		raw := []byte(target.platform)
		if err := os.WriteFile(filepath.Join(directory, target.filename), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		document.Artifacts = append(document.Artifacts, connectorInstallArtifact{Platform: target.platform,
			SigningKeyID: cfg.Spec.Telemetry.SigningKeyID, ByteSize: int64(len(raw)),
			SHA256: hex.EncodeToString(digest[:]), Signature: base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, digest[:]))})
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, connectorSignaturesFile), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, artifacts, err := loadConnectorInstallArtifacts(root, cfg); err != nil || len(artifacts) != len(connectorArtifactTargets) {
		t.Fatalf("verify installation bundle: %v", err)
	}
	cfg.Spec.Images.Tag = "v0.1.0"
	if _, _, err := loadConnectorInstallArtifacts(root, cfg); err == nil {
		t.Fatal("signatures for a different release were accepted")
	}
	cfg.Spec.Images.Tag = document.ImageTag
	if err := os.WriteFile(filepath.Join(directory, connectorArtifactTargets[0].filename), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadConnectorInstallArtifacts(root, cfg); err == nil {
		t.Fatal("modified Connector file was accepted")
	}
}

func TestVerifySignedArtifactRejectsMetadataDrift(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "artifact")
	raw := []byte("immutable release")
	if err = os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	signature := base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, digest[:]))
	if err = verifySignedArtifact(path, hex.EncodeToString(digest[:]), signature, uint64(len(raw)), publicKey); err != nil {
		t.Fatal(err)
	}
	if err = verifySignedArtifact(path, hex.EncodeToString(digest[:]), signature, uint64(len(raw)+1), publicKey); err == nil {
		t.Fatal("byte-size drift was accepted")
	}
}

func TestSanitizeReleaseVersionIsStable(t *testing.T) {
	if got := sanitizeReleaseVersion("dev build@1"); got != "dev-build-1" {
		t.Fatalf("sanitized version = %q", got)
	}
}
