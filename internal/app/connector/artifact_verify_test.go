package connector

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestRunVerifyArtifact(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("signed Windows artifact")
	digest := sha256.Sum256(payload)
	path := filepath.Join(t.TempDir(), "artifact.exe")
	if err = os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--file", path, "--sha256", hex.EncodeToString(digest[:]),
		"--signature", base64.RawStdEncoding.EncodeToString(ed25519.Sign(privateKey, digest[:])),
		"--public-key", base64.RawStdEncoding.EncodeToString(publicKey), "--byte-size", strconv.Itoa(len(payload))}
	if err = runVerifyArtifact(args); err != nil {
		t.Fatalf("verify signed artifact: %v", err)
	}

	if err = os.WriteFile(path, []byte("tampered Windows artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = runVerifyArtifact(args); err == nil {
		t.Fatal("tampered artifact was accepted")
	}
}
