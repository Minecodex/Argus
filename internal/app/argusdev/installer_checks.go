package argusdev

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"debug/elf"
	"debug/pe"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kakj-go/Argus/internal/installation"
)

type connectorBuildTarget struct {
	goos, goarch string
	format       string
}

func (a *App) checkInstallerContainers(ctx context.Context) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("%w: docker is required for installer container checks", errCapability)
	}
	if err := a.runner.Run(ctx, nil, "docker", "version", "--format", "{{.Server.Version}}"); err != nil {
		return fmt.Errorf("Docker Engine is unavailable: %w", err)
	}
	rootMount := a.root + ":/src:ro"
	for _, candidate := range []struct {
		image, shell string
	}{
		{image: "ubuntu:24.04", shell: "dash"},
		{image: "debian:bookworm-slim", shell: "dash"},
		{image: "alpine:3.22", shell: "/bin/sh"},
	} {
		for _, architecture := range []string{"amd64", "arm64"} {
			if err := a.runner.Run(ctx, nil, "docker", "run", "--rm", "--platform", "linux/"+architecture,
				"-v", rootMount, candidate.image, candidate.shell, "-n", "/src/deploy/scripts/connector-install.sh"); err != nil {
				return fmt.Errorf("%s linux/%s installer syntax: %w", candidate.image, architecture, err)
			}
			if _, err := a.runner.Output(ctx, nil, "docker", "run", "--rm", "--platform", "linux/"+architecture,
				"-v", rootMount, candidate.image, candidate.shell, "/src/deploy/scripts/connector-install.sh"); err == nil ||
				!strings.Contains(err.Error(), "a readable --token-file is required") {
				return fmt.Errorf("%s linux/%s installer did not execute its fail-closed argument preflight: %v", candidate.image, architecture, err)
			}
		}
	}

	temporary, err := os.MkdirTemp("", "argus-installer-containers-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	payload := []byte("argus installer container verification\n")
	payloadPath := filepath.Join(temporary, "payload")
	if err = os.WriteFile(payloadPath, payload, 0o600); err != nil {
		return err
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	signature := ed25519.Sign(privateKey, digest[:])
	for _, architecture := range []string{"amd64", "arm64"} {
		binaryName := "argus-connector-linux-" + architecture
		binaryPath := filepath.Join(temporary, binaryName)
		if err = a.runner.Run(ctx, map[string]string{"GOOS": "linux", "GOARCH": architecture, "CGO_ENABLED": "0"},
			"go", "build", "-trimpath", "-o", binaryPath, "./cmd/argus-connector"); err != nil {
			return err
		}
		if err = os.Chmod(binaryPath, 0o755); err != nil {
			return err
		}
		mount := temporary + ":/work:ro"
		if err = a.runner.Run(ctx, nil, "docker", "run", "--rm", "--platform", "linux/"+architecture, "-v", mount,
			"alpine:3.22", "/work/"+binaryName, "verify-artifact", "--file", "/work/payload",
			"--sha256", hex.EncodeToString(digest[:]), "--signature", base64.RawStdEncoding.EncodeToString(signature),
			"--public-key", base64.RawStdEncoding.EncodeToString(publicKey), "--byte-size", fmt.Sprint(len(payload))); err != nil {
			return fmt.Errorf("execute linux/%s Connector under container emulation: %w", architecture, err)
		}
		cleanupProgram := `set -eu
operation=11111111-1111-4111-8111-111111111111
connector=22222222-2222-4222-8222-222222222222
journal=/var/lib/argus-uninstall/$operation
mkdir -p "$journal" /usr/local/bin /etc/systemd/system /var/lib/argus-otelcol /etc/argus-otelcol /var/lib/argus-connector /etc/argus-connector
cp /work/` + binaryName + ` "$journal/argus-uninstaller"
chmod 0700 "$journal/argus-uninstaller"
cp /work/` + binaryName + ` /usr/local/bin/argus-connector
touch /usr/local/bin/argus-otelcol /usr/local/bin/argus-connector-privileged /etc/systemd/system/argus-otelcol.service /etc/systemd/system/argus-connector.service /etc/systemd/system/argus-connector-privileged.service
printf '{"connector_id":"%s"}' "$connector" > /var/lib/argus-connector/identity.json
nc -l -p 18445 >/dev/null 2>&1 & listener=$!
sleep 1
if "$journal/argus-uninstaller" uninstall-local --operation-id "$operation" --connector-id "$connector" --removal-generation 7 --journal-dir "$journal" --relay-https-port 18445 > "$journal/failed-output.json" 2>/dev/null; then exit 21; fi
test -s "$journal/evidence.failed.json"
test ! -e "$journal/evidence.json"
kill "$listener" >/dev/null 2>&1 || true
wait "$listener" 2>/dev/null || true
"$journal/argus-uninstaller" uninstall-local --operation-id "$operation" --connector-id "$connector" --removal-generation 7 --journal-dir "$journal" --relay-https-port 18445 > "$journal/verified-output.json"
"$journal/argus-uninstaller" uninstall-local --operation-id "$operation" --connector-id "$connector" --removal-generation 7 --journal-dir "$journal" --relay-https-port 18445 > "$journal/retry-output.json"
test ! -e /usr/local/bin/argus-connector
test ! -e /usr/local/bin/argus-otelcol
test ! -e /etc/argus-connector
test ! -e /var/lib/argus-connector
test -s "$journal/evidence.json"
grep -q '"connector_files_absent":true' "$journal/evidence.json"
grep -q '"connector_process_absent":true' "$journal/evidence.json"
grep -q '"collector_process_absent":true' "$journal/evidence.json"
grep -q '"relay_ports_released":true' "$journal/evidence.json"
test "$(wc -l < "$journal/events.jsonl")" -ge 9`
		if err = a.runner.Run(ctx, nil, "docker", "run", "--rm", "--platform", "linux/"+architecture, "-v", mount,
			"alpine:3.22", "/bin/sh", "-ec", cleanupProgram); err != nil {
			return fmt.Errorf("execute resumable linux/%s uninstall Helper check: %w", architecture, err)
		}
	}
	_, _ = fmt.Fprintln(a.stdout, "Ubuntu, Debian, and Alpine parsed the fail-closed installer on amd64/arm64; both Connector architectures verified signed artifacts and resumed idempotent local cleanup after an injected relay-port failure")
	return nil
}

func (a *App) checkInstallers(ctx context.Context) error {
	posixPath := filepath.Join(a.root, "deploy", "scripts", "connector-install.sh")
	posix, err := os.ReadFile(posixPath)
	if err != nil {
		return err
	}
	if bytes.HasPrefix(posix, []byte{0xef, 0xbb, 0xbf}) || bytes.ContainsRune(posix, '\r') {
		return errors.New("connector-install.sh must be UTF-8 without BOM and use LF line endings")
	}
	if _, err = installation.CanonicalScript(posix, installation.POSIXShell); err != nil {
		return err
	}
	if dash, lookupErr := exec.LookPath("dash"); lookupErr == nil {
		if err = a.runner.Run(ctx, nil, dash, "-n", posixPath); err != nil {
			return fmt.Errorf("dash syntax: %w", err)
		}
	} else {
		_, _ = fmt.Fprintln(a.stdout, "dash is unavailable; LF and canonical POSIX checks passed, syntax execution skipped on this host")
	}

	powerShellPath := filepath.Join(a.root, "deploy", "scripts", "connector-install.ps1")
	powerShell, err := installation.ReadCanonicalScript(powerShellPath, installation.PowerShell)
	if err != nil {
		return err
	}
	if err = validateConnectorTakeoverOrdering(posix, powerShell); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "argus-installer-check-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	canonicalPowerShellPath := filepath.Join(temporary, "connector-install.ps1")
	if err = os.WriteFile(canonicalPowerShellPath, powerShell, 0o600); err != nil {
		return err
	}
	if err = a.parsePowerShell(ctx, canonicalPowerShellPath); err != nil {
		return err
	}
	harnessScript, err := installation.CanonicalScript([]byte(windowsHostObservationScript), installation.PowerShell)
	if err != nil {
		return err
	}
	harnessPath := filepath.Join(temporary, "windows-host-observation.ps1")
	if err = os.WriteFile(harnessPath, harnessScript, 0o600); err != nil {
		return err
	}
	if err = a.parsePowerShell(ctx, harnessPath); err != nil {
		return fmt.Errorf("Windows E2E observation syntax: %w", err)
	}

	for _, target := range []connectorBuildTarget{
		{goos: "linux", goarch: "amd64", format: "elf"},
		{goos: "linux", goarch: "arm64", format: "elf"},
		{goos: "windows", goarch: "amd64", format: "pe"},
	} {
		name := "argus-connector-" + target.goos + "-" + target.goarch
		if target.goos == "windows" {
			name += ".exe"
		}
		path := filepath.Join(temporary, name)
		if err = a.runner.Run(ctx, map[string]string{"GOOS": target.goos, "GOARCH": target.goarch, "CGO_ENABLED": "0"},
			"go", "build", "-trimpath", "-o", path, "./cmd/argus-connector"); err != nil {
			return err
		}
		if err = verifyConnectorMachine(path, target); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintln(a.stdout, "installers parsed and Connector linux/amd64, linux/arm64, windows/amd64 binaries verified")
	return nil
}

func validateConnectorTakeoverOrdering(posix, powerShell []byte) error {
	ordered := func(content []byte, names ...string) bool {
		cursor := 0
		for _, name := range names {
			index := bytes.Index(content[cursor:], []byte(name))
			if index < 0 {
				return false
			}
			cursor += index + len(name)
		}
		return true
	}
	if !ordered(posix,
		`"$TMP/argus-connector" enroll`,
		`disable --now argus-connector-privileged.service`,
		`install -m 0755 "$TMP/argus-connector" "$BIN.tmp"`,
		`printf '%s' "$CONNECTOR_ID" > "$MARKER"`,
	) {
		return errors.New("POSIX Connector takeover must enroll before stopping and replacing the live Connector")
	}
	if !ordered(powerShell,
		`& $binary enroll`,
		`Stop-Service -Name $serviceName`,
		`Copy-Item -Force -LiteralPath $binary -Destination $nextExecutable`,
		`Start-Service -Name "ArgusConnector"`,
	) {
		return errors.New("PowerShell Connector takeover must enroll before stopping and replacing the live Connector")
	}
	return nil
}

func (a *App) parsePowerShell(ctx context.Context, path string) error {
	executable := ""
	for _, candidate := range []string{"pwsh", "powershell.exe", "powershell"} {
		if resolved, err := exec.LookPath(candidate); err == nil {
			executable = resolved
			break
		}
	}
	if executable == "" {
		return fmt.Errorf("%w: PowerShell 5.1+ or pwsh is required to parse connector-install.ps1", errCapability)
	}
	parser := strings.Join([]string{
		"$tokens = $null",
		"$parseErrors = $null",
		"[System.Management.Automation.Language.Parser]::ParseFile($env:ARGUS_POWERSHELL_PARSE_PATH, [ref]$tokens, [ref]$parseErrors) | Out-Null",
		"if ($parseErrors.Count -gt 0) { $parseErrors | ForEach-Object { [Console]::Error.WriteLine($_.Message) }; exit 1 }",
	}, "; ")
	if err := a.runner.Run(ctx, map[string]string{"ARGUS_POWERSHELL_PARSE_PATH": path}, executable, "-NoProfile", "-NonInteractive", "-Command", parser); err != nil {
		return fmt.Errorf("PowerShell syntax: %w", err)
	}
	return nil
}

func verifyConnectorMachine(path string, target connectorBuildTarget) error {
	switch target.format {
	case "elf":
		binary, err := elf.Open(path)
		if err != nil {
			return fmt.Errorf("open %s/%s Connector ELF: %w", target.goos, target.goarch, err)
		}
		defer binary.Close()
		expected := elf.EM_X86_64
		if target.goarch == "arm64" {
			expected = elf.EM_AARCH64
		}
		if binary.Machine != expected {
			return fmt.Errorf("Connector %s/%s machine is %s", target.goos, target.goarch, binary.Machine)
		}
	case "pe":
		binary, err := pe.Open(path)
		if err != nil {
			return fmt.Errorf("open %s/%s Connector PE: %w", target.goos, target.goarch, err)
		}
		defer binary.Close()
		if binary.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
			return fmt.Errorf("Connector %s/%s machine is %#x", target.goos, target.goarch, binary.Machine)
		}
	default:
		return fmt.Errorf("unknown Connector binary format %q", target.format)
	}
	return nil
}
