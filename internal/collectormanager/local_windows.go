//go:build windows

package collectormanager

import (
	"archive/zip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"golang.org/x/sys/windows"
)

const windowsCollectorServiceName = "ArgusCollector"

func (manager Manager) applyWindowsLocal(ctx context.Context, command *connectorv1.CollectorManagementCommand) (Result, error) {
	programData := os.Getenv("ProgramData")
	programFiles := os.Getenv("ProgramFiles")
	if programData == "" || programFiles == "" || !manager.ManageLocalService {
		return Result{}, ErrUnsupportedPlatform
	}
	stateRoot := filepath.Join(programData, "Argus", "Collector")
	installRoot := filepath.Join(programFiles, "Argus", "Collector")
	if command.GetOperation() == "uninstall" {
		if err := uninstallWindowsCollector(ctx, stateRoot, installRoot); err != nil {
			return Result{}, err
		}
		return buildResult(command, "uninstalled"), nil
	}
	collectorRoot := filepath.Join(stateRoot, command.GetCollectorId())
	for _, path := range []string{collectorRoot, installRoot, filepath.Join(stateRoot, "identity")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return Result{}, err
		}
	}
	if output, err := exec.CommandContext(ctx, "icacls.exe", stateRoot, "/inheritance:r", "/grant:r", "SYSTEM:(OI)(CI)(F)", "Administrators:(OI)(CI)(F)").CombinedOutput(); err != nil {
		return Result{}, fmt.Errorf("Collector ACL configuration failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	archivePath := filepath.Join(collectorRoot, "collector.zip")
	if err := manager.writeArtifactAtomic(ctx, archivePath, command.GetArtifact(), 0o600); err != nil {
		return Result{}, err
	}
	_ = exec.CommandContext(ctx, "sc.exe", "stop", windowsCollectorServiceName).Run()
	binaryPath := filepath.Join(installRoot, "argus-otelcol.exe")
	if err := extractWindowsCollector(archivePath, binaryPath); err != nil {
		return Result{}, err
	}
	runtimeConfig, err := configbundle.Extract(command.GetRenderedConfig(), "host")
	if err != nil {
		return Result{}, ErrInvalidCommand
	}
	trust, err := commandTrustBundle(command)
	if err != nil {
		return Result{}, err
	}
	configPath, caPath := filepath.Join(stateRoot, "config.yaml"), filepath.Join(stateRoot, "server-ca.pem")
	if err = writeAtomic(configPath, runtimeConfig, 0o600); err != nil {
		return Result{}, err
	}
	if err = writeAtomic(caPath, trust.PEM, 0o600); err != nil {
		return Result{}, err
	}
	marker := filepath.Join(stateRoot, ".active-collector-id")
	current, _ := os.ReadFile(marker)
	if strings.TrimSpace(string(current)) != command.GetCollectorId() {
		_ = os.RemoveAll(filepath.Join(stateRoot, "identity"))
		if err = os.MkdirAll(filepath.Join(stateRoot, "identity"), 0o700); err != nil {
			return Result{}, err
		}
	}
	if err = writeAtomic(marker, []byte(command.GetCollectorId()), 0o600); err != nil {
		return Result{}, err
	}
	tokenPath := filepath.Join(stateRoot, "enrollment-token")
	if validateWindowsCollectorIdentity(stateRoot, command.GetCollectorId()) != nil {
		if len(command.GetEnrollmentToken()) == 0 {
			return Result{}, ErrInvalidCommand
		}
		if err = writeAtomic(tokenPath, command.GetEnrollmentToken(), 0o600); err != nil {
			return Result{}, err
		}
	} else {
		_ = os.Remove(tokenPath)
	}
	connectorBinary, err := os.Executable()
	if err != nil {
		return Result{}, err
	}
	serviceCommand := windows.ComposeCommandLine([]string{connectorBinary, "collector-service", "--binary", binaryPath, "--config", configPath,
		"--token-file", tokenPath, "--enrollment-endpoint", command.GetEnrollmentEndpoint(), "--ingest-grpc-endpoint", command.GetIngestGrpcEndpoint(),
		"--ingest-http-endpoint", command.GetIngestHttpEndpoint()})
	_ = exec.CommandContext(ctx, "sc.exe", "delete", windowsCollectorServiceName).Run()
	var createOutput []byte
	for attempt := 0; attempt < 20; attempt++ {
		createOutput, err = exec.CommandContext(ctx, "sc.exe", "create", windowsCollectorServiceName, "binPath=", serviceCommand, "start=", "auto").CombinedOutput()
		if err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		return Result{}, fmt.Errorf("create Collector Windows Service: %w: %s", err, strings.TrimSpace(string(createOutput)))
	}
	_, _ = exec.CommandContext(ctx, "sc.exe", "failure", windowsCollectorServiceName, "reset=", "86400", "actions=", "restart/5000/restart/15000/restart/60000").CombinedOutput()
	if output, startErr := exec.CommandContext(ctx, "sc.exe", "start", windowsCollectorServiceName).CombinedOutput(); startErr != nil {
		return Result{}, fmt.Errorf("start Collector Windows Service: %w: %s", startErr, strings.TrimSpace(string(output)))
	}
	if err = waitWindowsCollectorReady(ctx, stateRoot, command.GetCollectorId()); err != nil {
		return Result{}, err
	}
	state := persistedState{SchemaVersion: "argus.collector_state/v1", CollectorID: command.GetCollectorId(), ResourceID: command.GetResourceId(),
		ResourceType: command.GetResourceType(), EffectiveRevision: command.GetDesiredRevision(), ConfigSHA256: strings.ToLower(command.GetConfigSha256()),
		ArtifactSHA256: strings.ToLower(command.GetArtifact().GetSha256()), RouteKind: command.GetRouteKind(), TrustBundleEpoch: command.GetTrustBundleEpoch(),
		TrustBundleSHA256: strings.ToLower(command.GetTrustBundleSha256()), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	encoded, _ := json.Marshal(state)
	if err = writeAtomic(filepath.Join(collectorRoot, "state.json"), encoded, 0o600); err != nil {
		return Result{}, err
	}
	return buildResult(command, "converged"), nil
}

func extractWindowsCollector(archivePath, destination string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return ErrArtifactInvalid
	}
	defer archive.Close()
	var source *zip.File
	for _, entry := range archive.File {
		if !entry.FileInfo().IsDir() && strings.EqualFold(filepath.Base(entry.Name), "argus-otelcol.exe") {
			if source != nil {
				return ErrArtifactInvalid
			}
			source = entry
		}
	}
	if source == nil || source.UncompressedSize64 < 1 || source.UncompressedSize64 > MaxArtifactBytes {
		return ErrArtifactInvalid
	}
	input, err := source.Open()
	if err != nil {
		return err
	}
	defer input.Close()
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".argus-otelcol-*.exe")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err = io.CopyN(temporary, input, int64(source.UncompressedSize64)); err != nil {
		_ = temporary.Close()
		return err
	}
	if err = temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	_ = os.Remove(destination)
	return os.Rename(temporaryPath, destination)
}

func waitWindowsCollectorReady(ctx context.Context, root, collectorID string) error {
	readyCtx, cancel := context.WithTimeout(ctx, localCollectorReadyTimeout)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, tokenErr := os.Stat(filepath.Join(root, "enrollment-token"))
		if validateWindowsCollectorIdentity(root, collectorID) == nil && errors.Is(tokenErr, os.ErrNotExist) && localCollectorHealthReady(readyCtx) {
			return nil
		}
		select {
		case <-readyCtx.Done():
			return readyCtx.Err()
		case <-ticker.C:
		}
	}
}

func validateWindowsCollectorIdentity(root, collectorID string) error {
	directory := filepath.Join(root, "identity")
	certificatePEM, err := os.ReadFile(filepath.Join(directory, "client.pem"))
	if err != nil {
		return err
	}
	privateKeyPEM, err := os.ReadFile(filepath.Join(directory, "client-key.pem"))
	if err != nil {
		return err
	}
	caPEM, err := os.ReadFile(filepath.Join(directory, "ca.pem"))
	if err != nil {
		return err
	}
	pair, err := tls.X509KeyPair(certificatePEM, privateKeyPEM)
	if err != nil || len(pair.Certificate) != 1 {
		return ErrInvalidCommand
	}
	certificate, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || len(certificate.URIs) != 1 || certificate.URIs[0].String() != "spiffe://argus/telemetry/collectors/"+collectorID {
		return ErrInvalidCommand
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return ErrInvalidCommand
	}
	return nil
}

func uninstallWindowsCollector(ctx context.Context, stateRoot, installRoot string) error {
	_ = exec.CommandContext(ctx, "sc.exe", "stop", windowsCollectorServiceName).Run()
	_ = exec.CommandContext(ctx, "sc.exe", "delete", windowsCollectorServiceName).Run()
	for _, path := range []string{installRoot, stateRoot} {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}
