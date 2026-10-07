package argusdev

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

func TestMinimalSuitesGetIsolatedConnectorArtifactSigning(t *testing.T) {
	app := &App{root: t.TempDir()}
	first := &E2EEnvironment{Options: E2EOptions{Suite: "m2", RunID: "first"}}
	second := &E2EEnvironment{Options: E2EOptions{Suite: "m2", RunID: "second"}}
	for _, env := range []*E2EEnvironment{first, second} {
		if err := app.prepareE2EArtifactServer(env); err != nil {
			t.Fatal(err)
		}
		if env.ArtifactSigning == nil || len(env.ArtifactSigning.PrivateKey) != ed25519.PrivateKeySize {
			t.Fatal("Connector installation has no signing root")
		}
		message := []byte("test connector release")
		if !ed25519.Verify(env.ArtifactSigning.PublicKey, message, ed25519.Sign(env.ArtifactSigning.PrivateKey, message)) {
			t.Fatal("signing identity mismatched")
		}
		if env.ArtifactTLS.Certificate == "" {
			t.Fatal("installation artifact fixture lacks TLS")
		}
	}
	if bytes.Equal(first.ArtifactSigning.PublicKey, second.ArtifactSigning.PublicKey) {
		t.Fatal("independent suites share signing authority")
	}
}
