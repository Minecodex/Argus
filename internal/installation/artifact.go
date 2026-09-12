// Package installation owns cross-platform installation contracts and
// canonical script bytes shared by publishers, bootstrap generators and
// execution adapters.
package installation

import (
	"bytes"
	"errors"
	"os"
	"strings"
)

type Platform string
type Shell string

const (
	LinuxAMD64   Platform = "linux_amd64"
	LinuxARM64   Platform = "linux_arm64"
	WindowsAMD64 Platform = "windows_amd64"

	POSIXShell Shell = "posix_sh"
	PowerShell Shell = "powershell"
)

func (platform Platform) Valid() bool {
	return platform == LinuxAMD64 || platform == LinuxARM64 || platform == WindowsAMD64
}

func (platform Platform) OS() string {
	if platform == WindowsAMD64 {
		return "windows"
	}
	if platform == LinuxAMD64 || platform == LinuxARM64 {
		return "linux"
	}
	return ""
}

func (platform Platform) Architecture() string {
	switch platform {
	case LinuxAMD64, WindowsAMD64:
		return "amd64"
	case LinuxARM64:
		return "arm64"
	default:
		return ""
	}
}

func ShellFor(platform Platform) Shell {
	if platform == WindowsAMD64 {
		return PowerShell
	}
	return POSIXShell
}

// CanonicalScript normalizes a source checkout into the exact immutable bytes
// that are hashed and uploaded. POSIX scripts always use LF; Windows scripts
// always use CRLF. A UTF-8 BOM is rejected so every downloader hashes the same
// content on every build host.
func CanonicalScript(raw []byte, shell Shell) ([]byte, error) {
	if bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) {
		return nil, errors.New("installation script must not contain a UTF-8 BOM")
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimRight(text, "\n") + "\n"
	switch shell {
	case POSIXShell:
		if !strings.HasPrefix(text, "#!/bin/sh\n") {
			return nil, errors.New("POSIX installation script must start with #!/bin/sh")
		}
		return []byte(text), nil
	case PowerShell:
		if !strings.Contains(text, "$ErrorActionPreference") || !strings.Contains(text, "Set-StrictMode") {
			return nil, errors.New("PowerShell installation script must enable fail-closed execution")
		}
		return []byte(strings.ReplaceAll(text, "\n", "\r\n")), nil
	default:
		return nil, errors.New("installation script shell is invalid")
	}
}

func ReadCanonicalScript(path string, shell Shell) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return CanonicalScript(raw, shell)
}
