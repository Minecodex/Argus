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
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const windowsCollectorServiceName = "ArgusCollector"

func (manager Manager) applyWindowsLocal(ctx context.Context, command *connectorv1.CollectorManagementCommand) (result Result, resultErr error) {
	programData, programFiles := os.Getenv("ProgramData"), os.Getenv("ProgramFiles")
	if programData == "" || programFiles == "" || !manager.ManageLocalService {
		return result, ErrUnsupportedPlatform
	}
	stateRoot, installRoot := filepath.Join(programData, "Argus", "Collector"), filepath.Join(programFiles, "Argus", "Collector")
	unlock, err := lockRuntime(ctx, filepath.Join(programData, "Argus", "CollectorControl"))
	if err != nil {
		return result, err
	}
	defer unlock()
	marker := filepath.Join(stateRoot, ".active-collector-id")
	current, err := os.ReadFile(marker)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	if len(current) > 0 && strings.TrimSpace(string(current)) != command.GetCollectorId() {
		return result, ErrInvalidCommand
	}
	if command.GetOperation() == "uninstall" {
		if err := uninstallWindowsCollector(ctx, stateRoot, installRoot); err != nil {
			return result, err
		}
		return buildResult(command, "uninstalled"), nil
	}
	collectorRoot := filepath.Join(stateRoot, command.GetCollectorId())
	for _, path := range []string{collectorRoot, installRoot, filepath.Join(stateRoot, "identity")} {
		if err = os.MkdirAll(path, 0700); err != nil {
			return result, err
		}
	}
	if output, err := exec.CommandContext(ctx, "icacls.exe", stateRoot, "/inheritance:r", "/grant:r", "SYSTEM:(OI)(CI)(F)", "Administrators:(OI)(CI)(F)").CombinedOutput(); err != nil {
		return result, fmt.Errorf("Collector ACL configuration failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if old, readErr := os.ReadFile(filepath.Join(collectorRoot, "state.json")); readErr == nil {
		var previous persistedState
		if json.Unmarshal(old, &previous) != nil || previous.CollectorID != command.GetCollectorId() || previous.EffectiveRevision > command.GetDesiredRevision() || (previous.EffectiveRevision == command.GetDesiredRevision() && previous.ConfigSHA256 != strings.ToLower(command.GetConfigSha256())) {
			return result, ErrInvalidCommand
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return result, readErr
	}
	archivePath := filepath.Join(collectorRoot, "collector.zip")
	if err = manager.writeArtifactAtomic(ctx, archivePath, command.GetArtifact(), 0600); err != nil {
		return result, err
	}
	candidate := filepath.Join(collectorRoot, "candidate.exe")
	if err = extractWindowsCollector(archivePath, candidate); err != nil {
		return result, err
	}
	defer os.Remove(candidate)
	public, err := configbundle.Extract(command.GetRenderedConfig(), "host")
	if err != nil {
		return result, ErrInvalidCommand
	}
	private, err := monitoringPrivateConfig(command)
	if err != nil {
		return result, err
	}
	defer clear(private)
	publicCandidate, privateCandidate := filepath.Join(collectorRoot, "candidate.json"), filepath.Join(collectorRoot, "candidate-private.json")
	if err = writeAtomic(publicCandidate, public, 0600); err != nil {
		return result, err
	}
	defer os.Remove(publicCandidate)
	if err = writeAtomic(privateCandidate, private, 0600); err != nil {
		return result, err
	}
	defer os.Remove(privateCandidate)
	if err = exec.CommandContext(ctx, candidate, "validate", "--config="+publicCandidate, "--config="+privateCandidate).Run(); err != nil {
		return result, ErrConfigInvalid
	}
	trust, err := commandTrustBundle(command)
	if err != nil {
		return result, err
	}
	configPath, privatePath, caPath, tokenPath := filepath.Join(stateRoot, "config.yaml"), filepath.Join(stateRoot, "monitoring-private.json"), filepath.Join(stateRoot, "server-ca.pem"), filepath.Join(stateRoot, "enrollment-token")
	saved, err := saveRuntimeFiles(configPath, privatePath, caPath, tokenPath, marker)
	if err != nil {
		return result, err
	}
	defer clearRuntimeFiles(saved)
	scm, err := mgr.Connect()
	if err != nil {
		return result, err
	}
	defer scm.Disconnect()
	service, err := scm.OpenService(windowsCollectorServiceName)
	if err != nil && !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return result, err
	}
	existed := err == nil
	created := false
	wasRunning := false
	oldCommand := ""
	defer func() {
		if service != nil {
			service.Close()
		}
	}()
	if existed {
		if len(current) == 0 {
			return result, ErrInvalidCommand
		}
		config, e := service.Config()
		if e != nil {
			return result, e
		}
		oldCommand = config.BinaryPathName
		state, e := service.Query()
		if e != nil {
			return result, e
		}
		wasRunning = state.State != svc.Stopped
	}
	binaryPath, backup := filepath.Join(installRoot, "argus-otelcol.exe"), filepath.Join(collectorRoot, "previous.exe")
	hadBinary := false
	if _, e := os.Stat(binaryPath); e == nil {
		if e = copyRuntimeFile(binaryPath, backup, 0700); e != nil {
			return result, e
		}
		hadBinary = true
	} else if !errors.Is(e, os.ErrNotExist) {
		return result, e
	}
	defer os.Remove(backup)
	defer func() {
		if resultErr == nil {
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.Background(), localCollectorReadyTimeout)
		defer cancel()
		rollbackErr := stopWindowsRuntime(rollbackCtx, service)
		rollbackErr = errors.Join(rollbackErr, restoreRuntimeFiles(saved))
		if hadBinary {
			rollbackErr = errors.Join(rollbackErr, copyRuntimeFile(backup, binaryPath, 0700))
		} else {
			if e := os.Remove(binaryPath); e != nil && !errors.Is(e, os.ErrNotExist) {
				rollbackErr = errors.Join(rollbackErr, e)
			}
		}
		if existed {
			rollbackErr = errors.Join(rollbackErr, changeWindowsRuntimeBinary(service, oldCommand))
			if wasRunning {
				rollbackErr = errors.Join(rollbackErr, service.Start())
				if rollbackErr == nil {
					rollbackErr = waitWindowsCollectorReady(rollbackCtx, stateRoot, command.GetCollectorId())
				}
			}
		} else if created {
			rollbackErr = errors.Join(rollbackErr, service.Delete())
		}
		if rollbackErr != nil {
			resultErr = errors.Join(resultErr, ErrRollbackFailed)
		}
	}()
	if err = stopWindowsRuntime(ctx, service); err != nil {
		return result, err
	}
	if err = copyRuntimeFile(candidate, binaryPath, 0700); err != nil {
		return result, err
	}
	for path, value := range map[string][]byte{configPath: public, privatePath: private, caPath: trust.PEM, marker: []byte(command.GetCollectorId())} {
		if err = writeAtomic(path, value, 0600); err != nil {
			return result, err
		}
	}
	if validateWindowsCollectorIdentity(stateRoot, command.GetCollectorId()) != nil {
		if len(command.GetEnrollmentToken()) == 0 {
			return result, ErrInvalidCommand
		}
		if err = writeAtomic(tokenPath, command.GetEnrollmentToken(), 0600); err != nil {
			return result, err
		}
	} else {
		_ = os.Remove(tokenPath)
	}
	connectorBinary, err := os.Executable()
	if err != nil {
		return result, err
	}
	args := []string{"collector-service", "--binary", binaryPath, "--config", configPath, "--private-config", privatePath, "--token-file", tokenPath, "--enrollment-endpoint", command.GetEnrollmentEndpoint(), "--ingest-grpc-endpoint", command.GetIngestGrpcEndpoint(), "--ingest-http-endpoint", command.GetIngestHttpEndpoint()}
	if existed {
		err = changeWindowsRuntimeBinary(service, windows.ComposeCommandLine(append([]string{connectorBinary}, args...)))
	} else {
		service, err = scm.CreateService(windowsCollectorServiceName, connectorBinary, mgr.Config{DisplayName: "Argus OpenTelemetry Collector", StartType: mgr.StartAutomatic}, args...)
		created = err == nil
		if err == nil {
			err = service.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 5 * time.Second}, {Type: mgr.ServiceRestart, Delay: 15 * time.Second}, {Type: mgr.ServiceRestart, Delay: time.Minute}}, 86400)
		}
	}
	if err != nil {
		return result, err
	}
	if err = service.Start(); err != nil {
		return result, err
	}
	if err = waitWindowsCollectorReady(ctx, stateRoot, command.GetCollectorId()); err != nil {
		return result, err
	}
	state := persistedState{SchemaVersion: "argus.collector_state/v1", CollectorID: command.GetCollectorId(), ResourceID: command.GetResourceId(), ResourceType: command.GetResourceType(), EffectiveRevision: command.GetDesiredRevision(), DesiredRevision: command.GetDesiredRevision(), ApplyStatus: "effective", ConfigSHA256: strings.ToLower(command.GetConfigSha256()), ArtifactSHA256: strings.ToLower(command.GetArtifact().GetSha256()), RouteKind: command.GetRouteKind(), TrustBundleEpoch: command.GetTrustBundleEpoch(), TrustBundleSHA256: strings.ToLower(command.GetTrustBundleSha256()), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	encoded, _ := json.Marshal(state)
	if err = writeAtomic(filepath.Join(collectorRoot, "state.json"), encoded, 0600); err != nil {
		return result, err
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
