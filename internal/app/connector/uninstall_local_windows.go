//go:build windows

package connector

import (
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/windows"
)

func cleanupLocalPlatform(ctx context.Context, options localUninstallOptions) localCleanupResult {
	programFiles := os.Getenv("ProgramFiles")
	programData := os.Getenv("ProgramData")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	if programData == "" {
		programData = `C:\ProgramData`
	}
	collectorState := filepath.Join(programData, "Argus", "Collector")
	collectorRoot := filepath.Join(programFiles, "Argus", "Collector")
	installState := filepath.Join(programData, "Argus", "Install")
	connectorState := filepath.Join(programData, "Argus", "Connector")
	connectorRoot := filepath.Join(programFiles, "Argus", "Connector")
	deadline := time.Now().Add(30 * time.Second)
	for {
		for _, service := range []string{"ArgusCollector", "ArgusConnectorPrivileged", "ArgusConnector"} {
			_ = exec.CommandContext(ctx, "sc.exe", "stop", service).Run()
			_ = exec.CommandContext(ctx, "sc.exe", "delete", service).Run()
		}
		for _, path := range []string{collectorState, collectorRoot, installState, connectorState, connectorRoot} {
			_ = os.RemoveAll(path)
		}
		if windowsServiceAbsent(ctx, "ArgusConnector") && windowsServiceAbsent(ctx, "ArgusConnectorPrivileged") &&
			windowsServiceAbsent(ctx, "ArgusCollector") && windowsProcessAbsent(ctx, "argus-connector.exe") && windowsProcessAbsent(ctx, "argus-otelcol.exe") &&
			windowsAbsent(collectorState, collectorRoot, installState, connectorState, connectorRoot) {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	return localCleanupResult{
		CollectorServiceAbsent: windowsServiceAbsent(ctx, "ArgusCollector"),
		CollectorProcessAbsent: windowsProcessAbsent(ctx, "argus-otelcol.exe"),
		CollectorFilesAbsent:   windowsAbsent(collectorRoot, collectorState),
		ConnectorServiceAbsent: windowsServiceAbsent(ctx, "ArgusConnector") && windowsServiceAbsent(ctx, "ArgusConnectorPrivileged"),
		ConnectorProcessAbsent: windowsProcessAbsent(ctx, "argus-connector.exe"),
		ConnectorFilesAbsent:   windowsAbsent(connectorRoot, connectorState, installState),
		ConnectorUserAbsent:    true,
		RelayPortsReleased:     windowsPortsReleased(options.RelayHTTPSPort, options.RelayGatewayPort),
	}
}

func windowsProcessAbsent(ctx context.Context, image string) bool {
	output, err := exec.CommandContext(ctx, "tasklist.exe", "/FI", "IMAGENAME eq "+image, "/FO", "CSV", "/NH").CombinedOutput()
	return err == nil && !bytes.Contains(bytes.ToLower(output), bytes.ToLower([]byte(`"`+image+`"`)))
}

func windowsServiceAbsent(ctx context.Context, name string) bool {
	output, err := exec.CommandContext(ctx, "sc.exe", "query", name).CombinedOutput()
	return err != nil && bytes.Contains(output, []byte("1060"))
}

func windowsAbsent(paths ...string) bool {
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func windowsPortsReleased(ports ...int) bool {
	for _, port := range ports {
		if port == 0 {
			continue
		}
		listener, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))
		if err != nil {
			return false
		}
		_ = listener.Close()
	}
	return true
}

func replaceUninstallJournalFile(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
