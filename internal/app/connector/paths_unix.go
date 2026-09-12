//go:build !windows

package connector

import "os"

func defaultDataDirectory() string {
	if value := os.Getenv("ARGUS_CONNECTOR_DATA_DIR"); value != "" {
		return value
	}
	return "/var/lib/argus-connector"
}

func machineIdentityParts() []string {
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if value, err := os.ReadFile(path); err == nil {
			return []string{string(value)}
		}
	}
	return nil
}

func hardenDirectory(path string) error { return os.Chmod(path, 0o700) }
func hardenFile(path string) error      { return os.Chmod(path, 0o600) }
