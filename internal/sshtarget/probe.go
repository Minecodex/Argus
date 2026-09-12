package sshtarget

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Evidence struct {
	Platform            string
	Architecture        string
	DistributionVersion string
	ServiceManager      string
	Privileged          bool
	FreeDiskBytes       uint64
}

const MinimumInstallFreeBytes = 512 << 20

func Probe(client *ssh.Client, expectedPlatform string) (Evidence, error) {
	switch expectedPlatform {
	case "linux":
		output, err := run(client, `set -eu; architecture=$(uname -m); distribution=unknown; if [ -r /etc/os-release ]; then id=$(sed -n 's/^ID=//p' /etc/os-release | head -n 1 | tr -d '"\r'); version=$(sed -n 's/^VERSION_ID=//p' /etc/os-release | head -n 1 | tr -d '"\r'); distribution="${id:-unknown}:${version:-unknown}"; fi; privileged=false; [ "$(id -u)" = 0 ] && privileged=true; service_manager=unavailable; if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then service_manager=systemd; fi; disk_kb=$(df -Pk /var/lib 2>/dev/null | awk 'END {print $4}'); free_disk_bytes=$((disk_kb * 1024)); printf 'architecture=%s\ndistribution_version=%s\nservice_manager=%s\nprivileged=%s\nfree_disk_bytes=%s\n' "$architecture" "$distribution" "$service_manager" "$privileged" "$free_disk_bytes"`)
		if err != nil {
			return Evidence{}, fmt.Errorf("probe Linux target: %w", err)
		}
		evidence, err := parseLinuxEvidence(output)
		if err != nil {
			return Evidence{}, err
		}
		switch evidence.Architecture {
		case "x86_64", "amd64":
			evidence.Architecture = "amd64"
		case "aarch64", "arm64":
			evidence.Architecture = "arm64"
		default:
			return Evidence{}, fmt.Errorf("unsupported Linux architecture %q", evidence.Architecture)
		}
		evidence.Platform = "linux"
		return validateEvidence(evidence)
	case "windows":
		output, err := run(client, PowerShellCommand(`$ErrorActionPreference='Stop';$os=Get-CimInstance Win32_OperatingSystem;$disk=Get-CimInstance Win32_LogicalDisk -Filter ("DeviceID='"+$env:SystemDrive+"'");$principal=New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent());$admin=$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator);$serviceManager=$false;try{$null=Get-Service -Name EventLog -ErrorAction Stop;$serviceManager=$true}catch{};@{architecture=[System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString();distribution_version=($os.Caption+':'+$os.Version);product_type=[int]$os.ProductType;build_number=[int]$os.BuildNumber;service_manager=$(if($serviceManager){'windows_scm'}else{'unavailable'});privileged=$admin;free_disk_bytes=[uint64]$disk.FreeSpace}|ConvertTo-Json -Compress`))
		if err != nil {
			return Evidence{}, fmt.Errorf("probe Windows target: %w", err)
		}
		var raw struct {
			Architecture        string `json:"architecture"`
			DistributionVersion string `json:"distribution_version"`
			ServiceManager      string `json:"service_manager"`
			ProductType         int    `json:"product_type"`
			BuildNumber         int    `json:"build_number"`
			Privileged          bool   `json:"privileged"`
			FreeDiskBytes       uint64 `json:"free_disk_bytes"`
		}
		if json.Unmarshal(output, &raw) != nil || raw.ProductType == 1 || raw.BuildNumber < 17763 {
			return Evidence{}, errors.New("Windows Server 2019 or newer is required")
		}
		evidence := Evidence{Platform: "windows", Architecture: strings.ToLower(raw.Architecture), DistributionVersion: raw.DistributionVersion,
			ServiceManager: raw.ServiceManager, Privileged: raw.Privileged, FreeDiskBytes: raw.FreeDiskBytes}
		switch evidence.Architecture {
		case "x64", "amd64":
			evidence.Architecture = "amd64"
		default:
			return Evidence{}, fmt.Errorf("unsupported Windows architecture %q", evidence.Architecture)
		}
		return validateEvidence(evidence)
	default:
		return Evidence{}, errors.New("expected SSH target platform is required")
	}
}

func parseLinuxEvidence(output []byte) (Evidence, error) {
	values := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key != "" {
			values[key] = value
		}
	}
	free, err := strconv.ParseUint(values["free_disk_bytes"], 10, 64)
	if err != nil {
		return Evidence{}, errors.New("Linux free disk evidence is invalid")
	}
	return Evidence{Architecture: values["architecture"], DistributionVersion: values["distribution_version"],
		ServiceManager: values["service_manager"], Privileged: values["privileged"] == "true", FreeDiskBytes: free}, nil
}

func validateEvidence(evidence Evidence) (Evidence, error) {
	if evidence.DistributionVersion == "" || evidence.DistributionVersion == "unknown" {
		return Evidence{}, errors.New("target distribution version is unavailable")
	}
	if !evidence.Privileged {
		return Evidence{}, errors.New("target SSH account must have system administrator privileges")
	}
	if (evidence.Platform == "linux" && evidence.ServiceManager != "systemd") ||
		(evidence.Platform == "windows" && evidence.ServiceManager != "windows_scm") {
		return Evidence{}, errors.New("target service manager is unavailable")
	}
	if evidence.FreeDiskBytes > math.MaxInt64 {
		return Evidence{}, errors.New("target free disk evidence exceeds the supported range")
	}
	if evidence.FreeDiskBytes < MinimumInstallFreeBytes {
		return Evidence{}, fmt.Errorf("target free disk space is below %d bytes", MinimumInstallFreeBytes)
	}
	return evidence, nil
}

func ConnectionChecks(evidence Evidence) []map[string]string {
	return []map[string]string{
		{"name": "platform", "status": "passed", "detail": evidence.Platform + "/" + evidence.Architecture},
		{"name": "distribution", "status": "passed", "detail": evidence.DistributionVersion},
		{"name": "privilege", "status": "passed", "detail": "system administrator"},
		{"name": "service_manager", "status": "passed", "detail": evidence.ServiceManager},
		{"name": "disk", "status": "passed", "detail": strconv.FormatUint(evidence.FreeDiskBytes, 10) + " bytes free"},
	}
}

func ProbeCallbacks(ctx context.Context, client *ssh.Client, addresses []string) error {
	if client == nil || len(addresses) == 0 {
		return &CallbackError{Code: "CONFIG_INVALID", cause: errors.New("callback targets are required")}
	}
	for _, address := range addresses {
		if host, port, err := net.SplitHostPort(address); err != nil || host == "" || port == "" {
			return &CallbackError{Code: "CONFIG_INVALID", cause: errors.New("callback target must be host:port")}
		}
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		connection, err := client.DialContext(probeCtx, "tcp", address)
		cancel()
		if err != nil {
			return callbackTransportError(fmt.Errorf("target cannot reach callback %s: %w", address, err))
		}
		_ = connection.Close()
	}
	return nil
}

func PowerShellCommand(script string) string {
	runes := []rune(script)
	raw := make([]byte, len(runes)*2)
	for index, value := range runes {
		binary.LittleEndian.PutUint16(raw[index*2:], uint16(value))
	}
	return "powershell.exe -NoProfile -NonInteractive -EncodedCommand " + base64.StdEncoding.EncodeToString(raw)
}

func run(client *ssh.Client, command string) ([]byte, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("open SSH probe session: %w", err)
	}
	defer session.Close()
	return session.Output(command)
}
