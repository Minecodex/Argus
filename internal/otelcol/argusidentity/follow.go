package argusidentity

import (
	"bytes"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func identityPaths(config Config) map[string]string {
	return map[string]string{
		"client.pem": config.CertificateFile, "client-key.pem": config.PrivateKeyFile,
		"server.pem": config.ServerCertificateFile, "server-key.pem": config.ServerPrivateKeyFile,
		"ca.pem": config.CABundleFile, "trust-bundle.json": config.TrustBundleStateFile,
	}
}

// Pin Kubernetes' ..data directory once. Reading the six top-level symlinks
// separately can combine keys, certificates and bundle state from two updates.
func projectedIdentityDirectory(source string) (string, error) {
	root, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "", err
	}
	projection := filepath.Join(root, "..data")
	if _, err = os.Lstat(projection); errors.Is(err, os.ErrNotExist) {
		return root, nil
	} else if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(projection)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("telemetry identity projection must stay inside its mounted directory")
	}
	return resolved, nil
}

func followBootstrapIdentity(config Config) error {
	if config.BootstrapIdentityDir == "" {
		return errors.New("telemetry identity follower has no bootstrap directory")
	}
	directory, err := projectedIdentityDirectory(config.BootstrapIdentityDir)
	if err != nil {
		return fmt.Errorf("read Collector identity projection: %w", err)
	}
	values := make(map[string][]byte, 6)
	paths := identityPaths(config)
	changed := false
	for name, target := range paths {
		if filepath.Clean(target) == filepath.Join(directory, name) {
			return errors.New("telemetry identity follower requires separate writable state")
		}
		value, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil || len(value) == 0 || len(value) > maxIdentityPayload {
			return fmt.Errorf("Collector identity projection file %s is unavailable or invalid", name)
		}
		values[name] = value
		current, readErr := os.ReadFile(target)
		changed = changed || readErr != nil || !bytes.Equal(current, value)
	}
	// Validate even an unchanged projection; expiry is never hidden by equality.
	state, err := parseTrustBundleState(values["trust-bundle.json"], values["ca.pem"])
	if err != nil {
		return err
	}
	clientKey, err := parseIdentityPrivateKey(values["client-key.pem"])
	if err != nil {
		return err
	}
	serverKey, err := parseIdentityPrivateKey(values["server-key.pem"])
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	material, err := parseTrustBundle(values["ca.pem"], now)
	if err != nil {
		return err
	}
	bundle := bundleSnapshot{Epoch: state.Epoch, Material: material}
	if err = validateIdentityCertificate(string(values["client.pem"]), config.CollectorID, clientKey, bundle, x509.ExtKeyUsageClientAuth, now); err != nil {
		return err
	}
	if err = validateIdentityCertificate(string(values["server.pem"]), config.CollectorID, serverKey, bundle, x509.ExtKeyUsageServerAuth, now); err != nil {
		return err
	}
	if previous, err := loadTrustBundleState(config.TrustBundleStateFile, config.CABundleFile); err == nil {
		if state.Epoch < previous.Epoch || (state.Epoch == previous.Epoch && state.BundleSHA256 != previous.BundleSHA256) {
			return errors.New("telemetry identity projection would roll back or replace its Trust Bundle epoch")
		}
	}
	for _, name := range []string{"client.pem", "server.pem"} {
		incoming, _ := parseCertificate(values[name]) // Validated above.
		if previous, err := loadCertificate(paths[name]); err == nil && previous.NotBefore.After(incoming.NotBefore) {
			return errors.New("telemetry identity projection would roll back its certificate")
		}
	}
	if !changed {
		return nil
	}
	// Keep source read-only. The client certificate is the final commit marker,
	// matching owner writes; consumers preserve their last TLS snapshot on error.
	for _, name := range []string{"client-key.pem", "server-key.pem", "ca.pem", "trust-bundle.json", "server.pem", "client.pem"} {
		target := paths[name]
		if err = os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err = writeAtomic(target, values[name], 0o600); err != nil {
			return err
		}
	}
	return nil
}
