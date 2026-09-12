package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"sigs.k8s.io/yaml"

	"github.com/kakj-go/Argus/internal/sshtarget"
)

const windowsHostE2ESchema = "argus.windows_host_e2e/v1"

var windowsE2EIdentifier = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)
var windowsE2EHostKey = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{43}$`)

type windowsHostE2EConfig struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Targets       []windowsHostE2ETarget `json:"targets"`
}

type windowsHostE2ETarget struct {
	Name                  string `json:"name"`
	Scenario              string `json:"scenario"`
	Address               string `json:"address"`
	Port                  int    `json:"port"`
	Username              string `json:"username"`
	HostKeySHA256         string `json:"hostKeySHA256"`
	PasswordEnv           string `json:"passwordEnv,omitempty"`
	PrivateKeyFile        string `json:"privateKeyFile,omitempty"`
	InstallCommandEnv     string `json:"installCommandEnv,omitempty"`
	UpgradeCommandEnv     string `json:"upgradeCommandEnv,omitempty"`
	UninstallCommandEnv   string `json:"uninstallCommandEnv,omitempty"`
	MinimumBuild          int    `json:"minimumBuild"`
	RequireRDP            bool   `json:"requireRDP"`
	WaitForInstallSeconds int    `json:"waitForInstallSeconds,omitempty"`
}

type windowsHostObservation struct {
	OSCaption               string `json:"os_caption"`
	OSVersion               string `json:"os_version"`
	BuildNumber             int    `json:"build_number"`
	Architecture            string `json:"architecture"`
	MachineGUIDPresent      bool   `json:"machine_guid_present"`
	ConnectorServicePresent bool   `json:"connector_service_present"`
	ConnectorServiceStatus  string `json:"connector_service_status"`
	ConnectorStartMode      string `json:"connector_start_mode"`
	FailureActionsPresent   bool   `json:"failure_actions_present"`
	ProgramPresent          bool   `json:"program_present"`
	IdentityPresent         bool   `json:"identity_present"`
	PrivateKeyPresent       bool   `json:"private_key_present"`
	ACLProtected            bool   `json:"acl_protected"`
	ACLSafe                 bool   `json:"acl_safe"`
	ConnectorID             string `json:"connector_id,omitempty"`
	TrustBundleEpoch        int64  `json:"trust_bundle_epoch,omitempty"`
	CertificateExpiresAt    string `json:"certificate_expires_at,omitempty"`
	OpenSSHRunning          bool   `json:"openssh_running"`
	RDPEnabled              bool   `json:"rdp_enabled"`
	RDPNLAEnabled           bool   `json:"rdp_nla_enabled"`
	RDPFirewallEnabled      bool   `json:"rdp_firewall_enabled"`
	RDPServiceRunning       bool   `json:"rdp_service_running"`
}

type windowsHostTargetResult struct {
	Name        string                 `json:"name"`
	Scenario    string                 `json:"scenario"`
	Status      string                 `json:"status"`
	CheckedAt   time.Time              `json:"checked_at"`
	Probe       sshtarget.Evidence     `json:"probe"`
	Observation windowsHostObservation `json:"observation"`
	Checks      []string               `json:"checks"`
}

const windowsHostObservationScript = `$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$program=Join-Path $env:ProgramFiles 'Argus\Connector\argus-connector.exe'
$state=Join-Path $env:ProgramData 'Argus\Connector'
$identityPath=Join-Path $state 'identity.json'
$keyPath=Join-Path $state 'connector-key.pem'
$service=Get-CimInstance Win32_Service -Filter "Name='ArgusConnector'" -ErrorAction SilentlyContinue
$identity=$null;if(Test-Path -LiteralPath $identityPath){$identity=Get-Content -LiteralPath $identityPath -Raw|ConvertFrom-Json}
$aclProtected=$false;$aclSafe=$false
if(Test-Path -LiteralPath $state){
  $acl=Get-Acl -LiteralPath $state;$aclProtected=$acl.AreAccessRulesProtected;$unsafe=$false
  foreach($rule in $acl.Access){
    $sid=$rule.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value
    $write=[Security.AccessControl.FileSystemRights]::Write -bor [Security.AccessControl.FileSystemRights]::Modify -bor [Security.AccessControl.FileSystemRights]::FullControl
    if($rule.AccessControlType -eq 'Allow' -and (($rule.FileSystemRights -band $write) -ne 0) -and $sid -notin @('S-1-5-18','S-1-5-32-544')){$unsafe=$true}
  }
  $aclSafe=-not $unsafe
}
$failureProperty=Get-ItemProperty -LiteralPath 'HKLM:\SYSTEM\CurrentControlSet\Services\ArgusConnector' -Name FailureActions -ErrorAction SilentlyContinue
$failure=$(if($null-ne $failureProperty){$failureProperty.FailureActions}else{$null})
$os=Get-CimInstance Win32_OperatingSystem
$machineGuid=(Get-ItemPropertyValue -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Cryptography' -Name MachineGuid -ErrorAction SilentlyContinue)
$ssh=Get-Service -Name sshd -ErrorAction SilentlyContinue
$deny=(Get-ItemPropertyValue -LiteralPath 'HKLM:\SYSTEM\CurrentControlSet\Control\Terminal Server' -Name fDenyTSConnections -ErrorAction SilentlyContinue)
$nla=(Get-ItemPropertyValue -LiteralPath 'HKLM:\SYSTEM\CurrentControlSet\Control\Terminal Server\WinStations\RDP-Tcp' -Name UserAuthentication -ErrorAction SilentlyContinue)
$rdpService=Get-Service -Name TermService -ErrorAction SilentlyContinue
$rdpFirewall=Get-NetFirewallRule -Name 'RemoteDesktop-UserMode-In-TCP' -ErrorAction SilentlyContinue
$machineGuidPresent=-not [string]::IsNullOrWhiteSpace($machineGuid)
@{os_caption=$os.Caption;os_version=$os.Version;build_number=[int]$os.BuildNumber;architecture=[Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant();machine_guid_present=$machineGuidPresent;connector_service_present=$null-ne $service;connector_service_status=$(if($null-ne $service){$service.State}else{''});connector_start_mode=$(if($null-ne $service){$service.StartMode}else{''});failure_actions_present=$null-ne $failure -and $failure.Length-gt 0;program_present=Test-Path -LiteralPath $program;identity_present=Test-Path -LiteralPath $identityPath;private_key_present=Test-Path -LiteralPath $keyPath;acl_protected=$aclProtected;acl_safe=$aclSafe;connector_id=$(if($null-ne $identity){$identity.connector_id}else{''});trust_bundle_epoch=$(if($null-ne $identity){[int64]$identity.trust_bundle_epoch}else{0});certificate_expires_at=$(if($null-ne $identity){[string]$identity.certificate_expires_at}else{''});openssh_running=$null-ne $ssh -and $ssh.Status-eq 'Running';rdp_enabled=$deny-eq 0;rdp_nla_enabled=$nla-eq 1;rdp_firewall_enabled=$null-ne $rdpFirewall -and $rdpFirewall.Enabled-eq 'True';rdp_service_running=$null-ne $rdpService -and $rdpService.Status-eq 'Running'}|ConvertTo-Json -Compress`

func (a *App) runWindowsHostE2E(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("e2e windows-host", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	configPath := flags.String("config", "", "Windows host E2E YAML config")
	runID := flags.String("run-id", "", "run identifier")
	artifacts := flags.String("artifacts", "", "artifact directory")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*configPath) == "" {
		return fmt.Errorf("%w: usage: argus-dev e2e windows-host --config FILE [--run-id ID] [--artifacts DIR]", errUsage)
	}
	config, err := loadWindowsHostE2EConfig(*configPath)
	if err != nil {
		return err
	}
	if *runID == "" {
		*runID = time.Now().UTC().Format("20060102150405") + fmt.Sprintf("-%d", os.Getpid())
	}
	if *artifacts == "" {
		*artifacts = filepath.Join(a.root, "artifacts", "windows-host-e2e", *runID)
	} else if !filepath.IsAbs(*artifacts) {
		*artifacts = filepath.Join(a.root, *artifacts)
	}
	if err = os.MkdirAll(*artifacts, 0o700); err != nil {
		return err
	}
	results := make([]windowsHostTargetResult, 0, len(config.Targets))
	for _, target := range config.Targets {
		_, _ = fmt.Fprintf(a.stdout, "Windows E2E %s (%s): probing pinned OpenSSH target\n", target.Name, target.Scenario)
		result, runErr := a.runWindowsHostTarget(ctx, target)
		if runErr != nil {
			failed := map[string]any{"schema_version": windowsHostE2ESchema, "run_id": *runID, "status": "failed",
				"target": target.Name, "scenario": target.Scenario, "error": stableWindowsE2EError(runErr)}
			encoded, _ := json.MarshalIndent(failed, "", "  ")
			_ = writePrivate(filepath.Join(*artifacts, target.Name+".json"), append(encoded, '\n'))
			return fmt.Errorf("Windows E2E %s failed: %w", target.Name, runErr)
		}
		results = append(results, result)
		encoded, _ := json.MarshalIndent(result, "", "  ")
		if err = writePrivate(filepath.Join(*artifacts, target.Name+".json"), append(encoded, '\n')); err != nil {
			return err
		}
	}
	output := map[string]any{"schema_version": windowsHostE2ESchema, "run_id": *runID, "status": "passed", "targets": results}
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(*artifacts, "result.json"), append(encoded, '\n')); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(a.stdout, "Windows host E2E passed for %d target(s); evidence: %s\n", len(results), *artifacts)
	return nil
}

func loadWindowsHostE2EConfig(path string) (windowsHostE2EConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return windowsHostE2EConfig{}, err
	}
	var config windowsHostE2EConfig
	if err = yaml.UnmarshalStrict(data, &config); err != nil {
		return windowsHostE2EConfig{}, fmt.Errorf("decode Windows E2E config: %w", err)
	}
	if err = validateWindowsHostE2EConfig(config); err != nil {
		return windowsHostE2EConfig{}, err
	}
	return config, nil
}

func validateWindowsHostE2EConfig(config windowsHostE2EConfig) error {
	if config.SchemaVersion != windowsHostE2ESchema || len(config.Targets) == 0 || len(config.Targets) > 16 {
		return errors.New("Windows E2E config requires schemaVersion argus.windows_host_e2e/v1 and 1..16 targets")
	}
	seen := map[string]bool{}
	for index, target := range config.Targets {
		if !windowsE2EIdentifier.MatchString(target.Name) || seen[target.Name] {
			return fmt.Errorf("Windows E2E target %d has an invalid or duplicate name", index)
		}
		seen[target.Name] = true
		if !oneOf(target.Scenario, "manual_direct", "manual_bastion_relay", "direct_ssh", "bastion_ssh") ||
			strings.TrimSpace(target.Address) == "" || target.Port < 1 || target.Port > 65535 || strings.TrimSpace(target.Username) == "" ||
			!windowsE2EHostKey.MatchString(target.HostKeySHA256) || target.MinimumBuild < 17763 {
			return fmt.Errorf("Windows E2E target %s has invalid platform or SSH fields", target.Name)
		}
		if (target.PasswordEnv == "") == (target.PrivateKeyFile == "") {
			return fmt.Errorf("Windows E2E target %s must select exactly one SSH credential source", target.Name)
		}
		for _, variable := range []string{target.PasswordEnv, target.InstallCommandEnv, target.UpgradeCommandEnv, target.UninstallCommandEnv} {
			if variable != "" && !windowsE2EIdentifier.MatchString(variable) {
				return fmt.Errorf("Windows E2E target %s has an invalid environment variable name", target.Name)
			}
		}
		if strings.HasPrefix(target.Scenario, "manual_") && target.InstallCommandEnv == "" {
			return fmt.Errorf("Windows E2E target %s manual scenario requires installCommandEnv", target.Name)
		}
	}
	return nil
}

func (a *App) runWindowsHostTarget(ctx context.Context, target windowsHostE2ETarget) (windowsHostTargetResult, error) {
	client, err := dialWindowsE2ESSH(ctx, target)
	if err != nil {
		return windowsHostTargetResult{}, err
	}
	probe, err := sshtarget.Probe(client, "windows")
	_ = client.Close()
	if err != nil {
		return windowsHostTargetResult{}, err
	}
	checks := []string{"host_key", "windows_server", "amd64", "administrator", "windows_scm", "disk"}
	if target.InstallCommandEnv != "" {
		if err = runWindowsE2ESensitiveCommand(ctx, target, target.InstallCommandEnv, 15*time.Minute); err != nil {
			return windowsHostTargetResult{}, err
		}
		checks = append(checks, "one_line_install")
	}
	wait := time.Duration(target.WaitForInstallSeconds) * time.Second
	if wait <= 0 {
		wait = 10 * time.Minute
	}
	observation, err := waitWindowsConnector(ctx, target, wait)
	if err != nil {
		return windowsHostTargetResult{}, err
	}
	if err = validateWindowsObservation(observation, target); err != nil {
		return windowsHostTargetResult{}, err
	}
	checks = append(checks, "service", "recovery", "acl", "identity", "trust_bundle", "openssh")
	if err = runWindowsE2EPowerShell(ctx, target,
		`& "$env:ProgramFiles\Argus\Connector\argus-connector.exe" self-test; if ($LASTEXITCODE -ne 0) { throw "ConPTY self-test failed" }`, 2*time.Minute, true); err != nil {
		return windowsHostTargetResult{}, err
	}
	checks = append(checks, "powershell_conpty")
	if err = runWindowsE2EPowerShell(ctx, target,
		`Restart-Service -Name ArgusConnector -Force; (Get-Service -Name ArgusConnector).WaitForStatus("Running",[TimeSpan]::FromSeconds(60))`, 2*time.Minute, true); err != nil {
		return windowsHostTargetResult{}, err
	}
	checks = append(checks, "service_restart")
	if target.UpgradeCommandEnv != "" {
		before := observation.ConnectorID
		if err = runWindowsE2ESensitiveCommand(ctx, target, target.UpgradeCommandEnv, 15*time.Minute); err != nil {
			return windowsHostTargetResult{}, err
		}
		observation, err = waitWindowsConnector(ctx, target, wait)
		if err != nil || observation.ConnectorID == "" || observation.ConnectorID != before {
			return windowsHostTargetResult{}, errors.New("Windows Connector upgrade did not preserve its enrolled identity")
		}
		checks = append(checks, "atomic_upgrade")
	}
	if target.RequireRDP {
		if !observation.RDPEnabled || !observation.RDPNLAEnabled || !observation.RDPFirewallEnabled || !observation.RDPServiceRunning {
			return windowsHostTargetResult{}, errors.New("Windows RDP/NLA/firewall/service readiness is incomplete")
		}
		checks = append(checks, "rdp_nla", "rdp_firewall", "rdp_service")
	}
	if target.UninstallCommandEnv != "" {
		if err = runWindowsE2ESensitiveCommand(ctx, target, target.UninstallCommandEnv, 10*time.Minute); err != nil {
			return windowsHostTargetResult{}, err
		}
		if err = waitWindowsConnectorRemoved(ctx, target, 3*time.Minute); err != nil {
			return windowsHostTargetResult{}, err
		}
		checks = append(checks, "complete_uninstall")
	}
	sort.Strings(checks)
	return windowsHostTargetResult{Name: target.Name, Scenario: target.Scenario, Status: "passed", CheckedAt: time.Now().UTC(), Probe: probe,
		Observation: observation, Checks: checks}, nil
}

func dialWindowsE2ESSH(ctx context.Context, target windowsHostE2ETarget) (*ssh.Client, error) {
	var auth ssh.AuthMethod
	if target.PasswordEnv != "" {
		value := os.Getenv(target.PasswordEnv)
		if value == "" {
			return nil, fmt.Errorf("SSH password environment variable %s is empty", target.PasswordEnv)
		}
		auth = ssh.Password(value)
	} else {
		value, err := os.ReadFile(target.PrivateKeyFile)
		if err != nil {
			return nil, err
		}
		signer, err := ssh.ParsePrivateKey(value)
		clear(value)
		if err != nil {
			return nil, errors.New("parse Windows E2E SSH private key")
		}
		auth = ssh.PublicKeys(signer)
	}
	configuration := &ssh.ClientConfig{User: target.Username, Auth: []ssh.AuthMethod{auth}, Timeout: 20 * time.Second,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if ssh.FingerprintSHA256(key) != target.HostKeySHA256 {
				return errors.New("Windows E2E SSH host key changed")
			}
			return nil
		}}
	dialer := net.Dialer{Timeout: 20 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(target.Address, fmt.Sprint(target.Port)))
	if err != nil {
		return nil, err
	}
	clientConnection, channels, requests, err := ssh.NewClientConn(connection, net.JoinHostPort(target.Address, fmt.Sprint(target.Port)), configuration)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	return ssh.NewClient(clientConnection, channels, requests), nil
}

func runWindowsE2ESensitiveCommand(ctx context.Context, target windowsHostE2ETarget, variable string, timeout time.Duration) error {
	command := os.Getenv(variable)
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("Windows command environment variable %s is empty", variable)
	}
	if err := runWindowsE2EPowerShell(ctx, target, command, timeout, false); err != nil {
		return fmt.Errorf("sensitive Windows operation from %s failed", variable)
	}
	return nil
}

func runWindowsE2EPowerShell(ctx context.Context, target windowsHostE2ETarget, script string, timeout time.Duration, includeSafeOutput bool) error {
	operationCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	client, err := dialWindowsE2ESSH(operationCtx, target)
	if err != nil {
		return err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	done := make(chan struct {
		output []byte
		err    error
	}, 1)
	go func() {
		output, runErr := session.CombinedOutput(sshtarget.PowerShellCommand(script))
		done <- struct {
			output []byte
			err    error
		}{output: output, err: runErr}
	}()
	select {
	case result := <-done:
		if result.err != nil {
			if includeSafeOutput {
				return fmt.Errorf("PowerShell operation failed: %w: %s", result.err, strings.TrimSpace(string(result.output)))
			}
			return errors.New("sensitive PowerShell operation failed")
		}
		return nil
	case <-operationCtx.Done():
		_ = client.Close()
		return operationCtx.Err()
	}
}

func observeWindowsHost(ctx context.Context, target windowsHostE2ETarget) (windowsHostObservation, error) {
	client, err := dialWindowsE2ESSH(ctx, target)
	if err != nil {
		return windowsHostObservation{}, err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return windowsHostObservation{}, err
	}
	defer session.Close()
	output, err := session.Output(sshtarget.PowerShellCommand(windowsHostObservationScript))
	if err != nil {
		return windowsHostObservation{}, errors.New("collect sanitized Windows host observation")
	}
	var observation windowsHostObservation
	if err = json.Unmarshal(output, &observation); err != nil {
		return windowsHostObservation{}, errors.New("decode sanitized Windows host observation")
	}
	return observation, nil
}

func waitWindowsConnector(ctx context.Context, target windowsHostE2ETarget, timeout time.Duration) (windowsHostObservation, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		observation, err := observeWindowsHost(ctx, target)
		if err == nil && observation.ConnectorServicePresent && strings.EqualFold(observation.ConnectorServiceStatus, "Running") && observation.IdentityPresent {
			return observation, nil
		}
		select {
		case <-ctx.Done():
			return windowsHostObservation{}, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return windowsHostObservation{}, errors.New("Windows Connector did not become installed and running before the deadline")
}

func waitWindowsConnectorRemoved(ctx context.Context, target windowsHostE2ETarget, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		observation, err := observeWindowsHost(ctx, target)
		if err == nil && !observation.ConnectorServicePresent && !observation.ProgramPresent && !observation.IdentityPresent && !observation.PrivateKeyPresent {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return errors.New("Windows Connector service, program, or identity remained after uninstall")
}

func validateWindowsObservation(observation windowsHostObservation, target windowsHostE2ETarget) error {
	if observation.BuildNumber < target.MinimumBuild || observation.Architecture != "x64" || !observation.MachineGUIDPresent {
		return errors.New("Windows Server build, amd64 architecture, or MachineGuid requirement failed")
	}
	if !observation.ConnectorServicePresent || !strings.EqualFold(observation.ConnectorServiceStatus, "Running") ||
		!strings.EqualFold(observation.ConnectorStartMode, "Auto") || !observation.FailureActionsPresent || !observation.ProgramPresent ||
		!observation.IdentityPresent || !observation.PrivateKeyPresent || !observation.ACLProtected || !observation.ACLSafe ||
		observation.ConnectorID == "" || observation.TrustBundleEpoch < 1 || observation.CertificateExpiresAt == "" || !observation.OpenSSHRunning {
		return errors.New("Windows Connector service, recovery, identity, ACL, Trust Bundle, or OpenSSH requirement failed")
	}
	return nil
}

func stableWindowsE2EError(err error) string {
	if err == nil {
		return "UNKNOWN"
	}
	text := strings.ToLower(err.Error())
	for _, candidate := range []struct{ pattern, code string }{
		{"host key", "SSH_HOST_KEY_CHANGED"}, {"password environment", "CREDENTIAL_UNAVAILABLE"}, {"private key", "CREDENTIAL_UNAVAILABLE"},
		{"windows server", "WINDOWS_VERSION_UNSUPPORTED"}, {"conpty", "WINDOWS_CONPTY_FAILED"}, {"rdp", "WINDOWS_RDP_NOT_READY"},
		{"uninstall", "WINDOWS_UNINSTALL_INCOMPLETE"}, {"deadline", "WINDOWS_E2E_TIMEOUT"},
	} {
		if strings.Contains(text, candidate.pattern) {
			return candidate.code
		}
	}
	return "WINDOWS_E2E_FAILED"
}
