//go:build !windows

package connector

import (
	"context"
	"net"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"
)

func cleanupLocalPlatform(ctx context.Context, options localUninstallOptions) localCleanupResult {
	for _, service := range []string{"argus-otelcol.service", "argus-connector-privileged.service", "argus-connector.service"} {
		_ = exec.CommandContext(ctx, "systemctl", "disable", "--now", service).Run()
	}
	for _, path := range []string{"/etc/systemd/system/argus-otelcol.service", "/etc/systemd/system/argus-connector.service", "/etc/systemd/system/argus-connector-privileged.service"} {
		_ = os.Remove(path)
	}
	_ = exec.CommandContext(ctx, "systemctl", "daemon-reload").Run()
	for _, path := range []string{"/var/lib/argus-otelcol", "/etc/argus-otelcol", "/var/lib/argus-connector", "/var/lib/argus-connector-install", "/etc/argus-connector"} {
		_ = os.RemoveAll(path)
	}
	for _, path := range []string{"/usr/local/bin/argus-otelcol", "/usr/local/bin/argus-connector", "/usr/local/bin/argus-connector-privileged"} {
		_ = os.Remove(path)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := user.Lookup("argus-connector"); err == nil {
			_ = exec.CommandContext(ctx, "userdel", "argus-connector").Run()
		}
		if serviceInactive(ctx, "argus-otelcol.service") && serviceInactive(ctx, "argus-connector.service") &&
			serviceInactive(ctx, "argus-connector-privileged.service") && processesAbsent("/usr/local/bin/argus-otelcol", "/usr/local/bin/argus-connector", "/usr/local/bin/argus-connector-privileged") {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	_, accountErr := user.Lookup("argus-connector")
	return localCleanupResult{
		CollectorServiceAbsent: serviceInactive(ctx, "argus-otelcol.service") && absent("/etc/systemd/system/argus-otelcol.service"),
		CollectorProcessAbsent: processesAbsent("/usr/local/bin/argus-otelcol"),
		CollectorFilesAbsent:   absent("/usr/local/bin/argus-otelcol", "/var/lib/argus-otelcol", "/etc/argus-otelcol"),
		ConnectorServiceAbsent: serviceInactive(ctx, "argus-connector.service") && serviceInactive(ctx, "argus-connector-privileged.service") && absent("/etc/systemd/system/argus-connector.service", "/etc/systemd/system/argus-connector-privileged.service"),
		ConnectorProcessAbsent: processesAbsent("/usr/local/bin/argus-connector", "/usr/local/bin/argus-connector-privileged"),
		ConnectorFilesAbsent:   absent("/usr/local/bin/argus-connector", "/usr/local/bin/argus-connector-privileged", "/var/lib/argus-connector", "/var/lib/argus-connector-install", "/etc/argus-connector"),
		ConnectorUserAbsent:    accountErr != nil,
		RelayPortsReleased:     portsReleased(options.RelayHTTPSPort, options.RelayGatewayPort),
	}
}

func processesAbsent(executables ...string) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err = strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		target, readErr := os.Readlink("/proc/" + entry.Name() + "/exe")
		if readErr != nil {
			continue
		}
		target = strings.TrimSuffix(target, " (deleted)")
		for _, executable := range executables {
			if target == executable {
				return false
			}
		}
	}
	return true
}

func serviceInactive(ctx context.Context, name string) bool {
	return exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", name).Run() != nil
}

func absent(paths ...string) bool {
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func portsReleased(ports ...int) bool {
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
	return os.Rename(source, destination)
}
