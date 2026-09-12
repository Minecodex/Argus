package connector

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const maxVerifiedArtifactBytes int64 = 256 << 20

func runVerifyArtifact(args []string) error {
	flags := flag.NewFlagSet("argus-connector verify-artifact", flag.ContinueOnError)
	path := flags.String("file", "", "Artifact file to verify")
	expectedSHA256 := flags.String("sha256", "", "Expected lowercase SHA-256")
	signatureValue := flags.String("signature", "", "Base64 Ed25519 signature over the SHA-256 digest")
	publicKeyValue := flags.String("public-key", "", "Base64 Ed25519 public key")
	expectedBytes := flags.Int64("byte-size", 0, "Expected artifact size")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *path == "" || *expectedBytes < 1 || *expectedBytes > maxVerifiedArtifactBytes {
		return errors.New("--file, --sha256, --signature, --public-key, and a valid --byte-size are required")
	}
	expectedDigest, err := hex.DecodeString(strings.TrimSpace(*expectedSHA256))
	if err != nil || len(expectedDigest) != sha256.Size {
		return errors.New("artifact SHA-256 is invalid")
	}
	publicKey, err := decodeRawBase64(*publicKeyValue, ed25519.PublicKeySize)
	if err != nil {
		return errors.New("artifact Ed25519 public key is invalid")
	}
	signature, err := decodeRawBase64(*signatureValue, ed25519.SignatureSize)
	if err != nil {
		return errors.New("artifact Ed25519 signature is invalid")
	}
	file, err := os.Open(*path)
	if err != nil {
		return fmt.Errorf("open artifact: %w", err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() != *expectedBytes {
		return errors.New("artifact byte size mismatch")
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, io.LimitReader(file, maxVerifiedArtifactBytes+1)); err != nil {
		return fmt.Errorf("hash artifact: %w", err)
	}
	actualDigest := hash.Sum(nil)
	if !equalBytes(actualDigest, expectedDigest) {
		return errors.New("artifact SHA-256 mismatch")
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), actualDigest, signature) {
		return errors.New("artifact Ed25519 signature verification failed")
	}
	return nil
}

func decodeRawBase64(value string, size int) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	decoded, err := base64.RawStdEncoding.DecodeString(trimmed)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(trimmed)
	}
	if err != nil || len(decoded) != size {
		return nil, errors.New("invalid base64 value")
	}
	return decoded, nil
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var different byte
	for index := range left {
		different |= left[index] ^ right[index]
	}
	return different == 0
}
