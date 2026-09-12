//go:build windows

package connector

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func defaultDataDirectory() string {
	if value := os.Getenv("ARGUS_CONNECTOR_DATA_DIR"); value != "" {
		return value
	}
	root := os.Getenv("ProgramData")
	if root == "" {
		root = `C:\ProgramData`
	}
	return filepath.Join(root, "Argus", "Connector")
}

func machineIdentityParts() []string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer key.Close()
	value, _, err := key.GetStringValue("MachineGuid")
	if err != nil || strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{value}
}

func hardenDirectory(path string) error { return hardenWindowsPath(path) }
func hardenFile(path string) error      { return hardenWindowsPath(path) }

func hardenWindowsPath(path string) error {
	identity, err := exec.Command("whoami.exe").Output()
	if err != nil || strings.TrimSpace(string(identity)) == "" {
		return err
	}
	command := exec.Command("icacls.exe", path, "/inheritance:r", "/grant:r", `SYSTEM:(F)`, `Administrators:(F)`, strings.TrimSpace(string(identity))+`:(F)`)
	command.Stdout, command.Stderr = nil, nil
	return command.Run()
}
