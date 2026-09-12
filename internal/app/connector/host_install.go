package connector

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kakj-go/Argus/internal/collectormanager"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/sshtarget"
)

type hostConnectorInstallStageError struct {
	stage string
	cause error
}

func (err hostConnectorInstallStageError) Error() string { return err.stage + ": " + err.cause.Error() }
func (err hostConnectorInstallStageError) Unwrap() error { return err.cause }

func hostConnectorInstallFailure(stage string, cause error) error {
	return hostConnectorInstallStageError{stage: stage, cause: cause}
}

func hostConnectorInstallFailureStage(cause error) string {
	var staged hostConnectorInstallStageError
	if errors.As(cause, &staged) {
		return staged.stage
	}
	return "unknown"
}

func executeHostConnectorInstall(ctx context.Context, payload *anypb.Any, credential, operationSecret []byte) (*connectorv1.HostConnectorInstallResult, error) {
	var request connectorv1.HostConnectorInstall
	if payload == nil || payload.UnmarshalTo(&request) != nil || request.GetOperationId() == "" || request.GetHostId() == "" || request.GetConnectorId() == "" ||
		len(credential) == 0 || len(operationSecret) == 0 || request.GetArtifact() == nil {
		return nil, errors.New("Host Connector install command is invalid")
	}
	platform := installation.Platform(request.GetTargetPlatform())
	if !platform.Valid() {
		return nil, errors.New("Host Connector target platform is invalid")
	}
	publicKey, err := base64.RawStdEncoding.DecodeString(request.GetSigningPublicKey())
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return nil, hostConnectorInstallFailure("artifact_trust", errors.New("Host Connector signing key is invalid"))
	}
	auth, err := connectorSSHAuth(credential)
	if err != nil {
		return nil, hostConnectorInstallFailure("ssh_auth", err)
	}
	config := &ssh.ClientConfig{User: request.GetUsername(), Auth: []ssh.AuthMethod{auth}, Timeout: commandTimeout,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if ssh.FingerprintSHA256(key) != request.GetPinnedHostKey() {
				return errors.New("Host key changed")
			}
			return nil
		}}
	client, err := ssh.Dial("tcp", net.JoinHostPort(request.GetAddress(), fmt.Sprint(request.GetPort())), config)
	if err != nil {
		return nil, hostConnectorInstallFailure("ssh_connect", err)
	}
	defer client.Close()
	stopOnCancel := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stopOnCancel()
	evidence, err := sshtarget.Probe(client, platform.OS())
	if err != nil || evidence.Platform+"_"+evidence.Architecture != string(platform) || evidence.DistributionVersion != request.GetTargetDistributionVersion() {
		return nil, hostConnectorInstallFailure("target_probe", errors.New("Host Connector target platform changed"))
	}
	plan := installation.HostConnectorInstallPlan{TargetPlatform: platform, DistributionVersion: request.GetTargetDistributionVersion(), ConnectorID: parseUUID(request.GetConnectorId()), EnrollmentEndpoint: request.GetEnrollmentEndpoint(),
		GatewayEndpoint: request.GetGatewayEndpoint(), EnrollDialAddress: request.GetEnrollDialAddress(), GatewayDialAddress: request.GetGatewayDialAddress(), TrustBundlePEM: request.GetTrustBundlePem(),
		SigningPublicKey: request.GetSigningPublicKey(), Artifact: installation.Artifact{SigningKeyID: request.GetArtifact().GetSigningKeyId()}}
	if plan.ConnectorID.String() != request.GetConnectorId() || plan.DistributionVersion == "" {
		return nil, errors.New("Host Connector identity is invalid")
	}
	callbacks, err := plan.CallbackAddresses()
	if err != nil {
		return nil, hostConnectorInstallFailure("target_callback", err)
	}
	if err = sshtarget.ProbeCallbacks(ctx, client, callbacks); err != nil {
		return nil, hostConnectorInstallFailure("target_callback", err)
	}
	if err = sshtarget.ProbeEnrollment(ctx, client, plan.EnrollmentEndpoint, plan.EnrollDialAddress, plan.TrustBundlePEM); err != nil {
		return nil, hostConnectorInstallFailure("target_callback", err)
	}
	if err = sshtarget.ProbeGateway(ctx, client, plan.GatewayEndpoint, plan.GatewayDialAddress, plan.TrustBundlePEM); err != nil {
		return nil, hostConnectorInstallFailure("target_callback", err)
	}
	artifactClient, err := collectormanager.NewArtifactHTTPClient(os.Getenv("ARGUS_OTELCOL_ARTIFACT_CA_PATH"))
	if err != nil {
		return nil, hostConnectorInstallFailure("artifact_trust", err)
	}
	artifact, err := os.CreateTemp("", ".argus-host-connector-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(artifact.Name())
	defer artifact.Close()
	if err = (collectormanager.Manager{HTTPClient: artifactClient, TrustedSigningKeys: map[string]ed25519.PublicKey{request.GetArtifact().GetSigningKeyId(): publicKey}}).FetchArtifactTo(ctx, request.GetArtifact(), artifact); err != nil {
		return nil, hostConnectorInstallFailure("artifact_fetch", err)
	}
	if _, err = artifact.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if platform.OS() == "windows" {
		err = installWindowsViaBastion(client, artifact, plan, string(operationSecret))
	} else {
		err = installLinuxViaBastion(client, artifact, plan, string(operationSecret))
	}
	if err != nil {
		if hostConnectorInstallFailureStage(err) != "unknown" {
			return nil, err
		}
		return nil, hostConnectorInstallFailure("remote_install", err)
	}
	return &connectorv1.HostConnectorInstallResult{HostId: request.GetHostId(), ConnectorId: request.GetConnectorId(), ServiceStarted: true}, nil
}

func installLinuxViaBastion(client *ssh.Client, binary io.Reader, plan installation.HostConnectorInstallPlan, token string) error {
	stage := "/var/lib/argus-connector-install/" + plan.ConnectorID.String()
	if err := sshRunReader(client, `set -eu; umask 077; install -d -m 0700 `+shellLiteral(stage)+`; tmp=$(mktemp `+shellLiteral(stage+`/.argus-connector.XXXXXX`)+`); cat > "$tmp"; chmod 0700 "$tmp"; mv -f "$tmp" `+shellLiteral(stage+`/argus-connector`), binary); err != nil {
		return hostConnectorInstallFailure("binary_transfer", err)
	}
	if err := sshRun(client, `set -eu; id argus-connector >/dev/null 2>&1 || useradd --system --home-dir /var/lib/argus-connector --shell /usr/sbin/nologin argus-connector; install -d -m 0700 -o argus-connector -g argus-connector /var/lib/argus-connector; install -d -m 0755 /etc/argus-connector; install -d -m 0700 /var/lib/argus-otelcol /etc/argus-otelcol; current=""; if [ -f /var/lib/argus-connector/identity.json ]; then current=$(sed -n 's/.*"connector_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' /var/lib/argus-connector/identity.json | sed -n '1p'); fi; [ -n "$current" ] || { [ ! -f /var/lib/argus-connector/.desired-connector-id ] || current=$(sed -n '1p' /var/lib/argus-connector/.desired-connector-id); }; printf '%s' "$current" > `+shellLiteral(stage+`/previous-connector-id`)); err != nil {
		return hostConnectorInstallFailure("runtime_prepare", err)
	}
	if err := sshRunReader(client, `umask 077; cat > `+shellLiteral(stage+`/server-ca.pem`)+`; chmod 0600 `+shellLiteral(stage+`/server-ca.pem`), bytes.NewReader(plan.TrustBundlePEM)); err != nil {
		return hostConnectorInstallFailure("trust_write", err)
	}
	signingKeys, _ := json.Marshal(map[string]string{plan.Artifact.SigningKeyID: plan.SigningPublicKey})
	if err := sshRunReader(client, `umask 077; cat > `+shellLiteral(stage+`/otelcol-signing-keys.json`)+`; chmod 0600 `+shellLiteral(stage+`/otelcol-signing-keys.json`), bytes.NewReader(signingKeys)); err != nil {
		return hostConnectorInstallFailure("trust_write", err)
	}
	if err := sshRunReader(client, `umask 077; cat > `+shellLiteral(stage+`/enrollment-token`)+`; chmod 0600 `+shellLiteral(stage+`/enrollment-token`), strings.NewReader(token)); err != nil {
		return hostConnectorInstallFailure("enrollment_material_write", err)
	}
	env := ""
	if plan.EnrollDialAddress != "" {
		env = "ARGUS_CONNECTOR_ENROLL_ADDRESS=" + shellLiteral(plan.EnrollDialAddress) + " "
	}
	if err := sshRun(client, env+shellLiteral(stage+`/argus-connector`)+` enroll --connector-id `+shellLiteral(plan.ConnectorID.String())+` --token-file `+shellLiteral(stage+`/enrollment-token`)+` --server `+shellLiteral(plan.EnrollmentEndpoint)+` --ca-file `+shellLiteral(stage+`/server-ca.pem`)+` --role host --data-dir /var/lib/argus-connector`); err != nil {
		return hostConnectorInstallFailure("enrollment", err)
	}
	promote := `set -eu; current=$(cat ` + shellLiteral(stage+`/previous-connector-id`) + `); systemctl disable --now argus-connector-privileged.service >/dev/null 2>&1 || true; systemctl disable --now argus-connector.service >/dev/null 2>&1 || true; if [ -n "$current" ] && [ "$current" != ` + shellLiteral(plan.ConnectorID.String()) + ` ]; then systemctl disable --now argus-otelcol.service >/dev/null 2>&1 || true; rm -f /etc/systemd/system/argus-otelcol.service /usr/local/bin/argus-otelcol; rm -rf /var/lib/argus-otelcol /etc/argus-otelcol; install -d -m 0700 /var/lib/argus-otelcol /etc/argus-otelcol; fi; install -m 0755 ` + shellLiteral(stage+`/argus-connector`) + ` /usr/local/bin/argus-connector; install -m 0600 ` + shellLiteral(stage+`/server-ca.pem`) + ` /etc/argus-connector/server-ca.pem; install -m 0600 ` + shellLiteral(stage+`/otelcol-signing-keys.json`) + ` /etc/argus-connector/otelcol-signing-keys.json; printf '%s' ` + shellLiteral(plan.ConnectorID.String()) + ` > /var/lib/argus-connector/.desired-connector-id`
	if err := sshRun(client, promote); err != nil {
		return hostConnectorInstallFailure("takeover", err)
	}
	unit := `[Unit]
Description=Argus Host Connector
After=network-online.target argus-connector-privileged.service
Wants=network-online.target argus-connector-privileged.service
[Service]
Type=simple
User=argus-connector
Group=argus-connector
Environment=ARGUS_CONNECTOR_DIAL_ADDRESS=` + plan.GatewayDialAddress + `
Environment=ARGUS_ARTIFACT_DIAL_ADDRESS=` + plan.EnrollDialAddress + `
Environment=ARGUS_OTELCOL_SIGNING_PUBLIC_KEYS_FILE=/etc/argus-connector/otelcol-signing-keys.json
ExecStart=/usr/local/bin/argus-connector run --data-dir=/var/lib/argus-connector
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/argus-connector /etc/argus-connector
[Install]
WantedBy=multi-user.target
`
	if err := sshRunReader(client, `cat > /etc/systemd/system/argus-connector.service; chmod 0644 /etc/systemd/system/argus-connector.service`, strings.NewReader(unit)); err != nil {
		return hostConnectorInstallFailure("service_write", err)
	}
	helper := `[Unit]
Description=Argus Connector privileged local lifecycle helper
After=network-online.target
[Service]
Type=simple
User=root
Group=argus-connector
RuntimeDirectory=argus-connector
RuntimeDirectoryMode=0750
Environment=ARGUS_OTELCOL_ARTIFACT_CA_PATH=/etc/argus-connector/server-ca.pem
Environment=ARGUS_ARTIFACT_DIAL_ADDRESS=` + plan.EnrollDialAddress + `
Environment=ARGUS_OTELCOL_SIGNING_PUBLIC_KEYS_FILE=/etc/argus-connector/otelcol-signing-keys.json
ExecStart=/usr/local/bin/argus-connector privileged-helper
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/run/argus-connector /var/lib/argus-otelcol /etc/argus-otelcol /usr/local/bin /etc/systemd/system /run/systemd/system
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
[Install]
WantedBy=multi-user.target
`
	if err := sshRunReader(client, `cat > /etc/systemd/system/argus-connector-privileged.service; chmod 0644 /etc/systemd/system/argus-connector-privileged.service`, strings.NewReader(helper)); err != nil {
		return hostConnectorInstallFailure("helper_service_write", err)
	}
	if err := sshRun(client, `chown -R argus-connector:argus-connector /var/lib/argus-connector /etc/argus-connector; systemctl daemon-reload; systemctl enable --now argus-connector-privileged.service; systemctl enable --now argus-connector.service; systemctl is-active --quiet argus-connector.service; rm -rf `+shellLiteral(stage)+`; rmdir /var/lib/argus-connector-install >/dev/null 2>&1 || true`); err != nil {
		return hostConnectorInstallFailure("service_activate", err)
	}
	return nil
}

func installWindowsViaBastion(client *ssh.Client, binary io.Reader, plan installation.HostConnectorInstallPlan, token string) error {
	stage := `C:\ProgramData\Argus\Install\` + plan.ConnectorID.String()
	writeBinary := sshtarget.PowerShellCommand(`$p=` + psLiteral(stage) + `;New-Item -ItemType Directory -Force -Path $p|Out-Null;$f=[IO.File]::Open((Join-Path $p 'argus-connector.next.exe'),'Create','Write','None');try{[Console]::OpenStandardInput().CopyTo($f)}finally{$f.Dispose()}`)
	if err := sshRunReader(client, writeBinary, binary); err != nil {
		return err
	}
	if err := windowsSSHWrite(client, stage+`\server-ca.pem`, plan.TrustBundlePEM); err != nil {
		return err
	}
	signingKeys, _ := json.Marshal(map[string]string{plan.Artifact.SigningKeyID: plan.SigningPublicKey})
	if err := windowsSSHWrite(client, stage+`\otelcol-signing-keys.json`, signingKeys); err != nil {
		return err
	}
	if err := windowsSSHWrite(client, stage+`\enrollment-token`, []byte(token)); err != nil {
		return err
	}
	script := buildWindowsBastionTakeoverScript(plan, stage)
	return sshRun(client, sshtarget.PowerShellCommand(script))
}

func buildWindowsBastionTakeoverScript(plan installation.HostConnectorInstallPlan, stage string) string {
	script := `$ErrorActionPreference='Stop';$stage=` + psLiteral(stage) + `;$state='C:\ProgramData\Argus\Connector';New-Item -ItemType Directory -Force -Path $state,$stage|Out-Null;foreach($securePath in @($state,$stage)){& icacls.exe $securePath /inheritance:r /grant:r 'SYSTEM:(OI)(CI)(F)' 'Administrators:(OI)(CI)(F)'|Out-Null;if($LASTEXITCODE-ne 0){throw 'ACL failed'}};$current='';$identity=Join-Path $state 'identity.json';if(Test-Path -LiteralPath $identity){$current=[string]((Get-Content -LiteralPath $identity -Raw|ConvertFrom-Json).connector_id)};[IO.File]::WriteAllText((Join-Path $stage 'previous-connector-id'),$current,(New-Object Text.UTF8Encoding($false)));$next=Join-Path $stage 'argus-connector.next.exe';$bin='C:\Program Files\Argus\Connector\argus-connector.exe';`
	if plan.EnrollDialAddress != "" {
		script += `$env:ARGUS_CONNECTOR_ENROLL_ADDRESS=` + psLiteral(plan.EnrollDialAddress) + `;`
	}
	script += `& $next enroll --connector-id ` + psLiteral(plan.ConnectorID.String()) + ` --token-file (Join-Path $stage 'enrollment-token') --server ` + psLiteral(plan.EnrollmentEndpoint) + ` --ca-file (Join-Path $stage 'server-ca.pem') --role host --data-dir $state;if($LASTEXITCODE-ne 0){throw 'enrollment failed'};if($current -and $current -ne ` + psLiteral(plan.ConnectorID.String()) + `){Stop-Service ArgusCollector -Force -ErrorAction SilentlyContinue;& sc.exe delete ArgusCollector|Out-Null;Remove-Item -LiteralPath 'C:\Program Files\Argus\Collector','C:\ProgramData\Argus\Collector' -Recurse -Force -ErrorAction SilentlyContinue};foreach($serviceName in @('ArgusConnectorPrivileged','ArgusConnector')){$service=Get-Service $serviceName -ErrorAction SilentlyContinue;if($service -and $service.Status -ne 'Stopped'){Stop-Service $serviceName -Force;$service.WaitForStatus('Stopped',[TimeSpan]::FromSeconds(30))}};New-Item -ItemType Directory -Force -Path (Split-Path $bin)|Out-Null;$copied=$false;for($attempt=0;$attempt -lt 20;$attempt++){try{Copy-Item -LiteralPath $next -Destination $bin -Force;$copied=$true;break}catch{Start-Sleep -Milliseconds 250}};if(-not $copied){throw 'Connector executable replacement failed'};Copy-Item -LiteralPath (Join-Path $stage 'server-ca.pem') -Destination (Join-Path $state 'server-ca.pem') -Force;Copy-Item -LiteralPath (Join-Path $stage 'otelcol-signing-keys.json') -Destination (Join-Path $state 'otelcol-signing-keys.json') -Force;[IO.File]::WriteAllText((Join-Path $state '.desired-connector-id'),` + psLiteral(plan.ConnectorID.String()) + `,(New-Object Text.UTF8Encoding($false)));`
	if plan.GatewayDialAddress != "" {
		script += `[Environment]::SetEnvironmentVariable('ARGUS_CONNECTOR_DIAL_ADDRESS',` + psLiteral(plan.GatewayDialAddress) + `,'Machine');`
	}
	if plan.EnrollDialAddress != "" {
		script += `[Environment]::SetEnvironmentVariable('ARGUS_ARTIFACT_DIAL_ADDRESS',` + psLiteral(plan.EnrollDialAddress) + `,'Machine');`
	}
	script += `[Environment]::SetEnvironmentVariable('ARGUS_OTELCOL_SIGNING_PUBLIC_KEYS_FILE',(Join-Path $state 'otelcol-signing-keys.json'),'Machine');`
	script += `$cmd='"'+$bin+'" run --data-dir "'+$state+'"';& sc.exe create ArgusConnector binPath= $cmd start= auto|Out-Null;if($LASTEXITCODE-ne 0){& sc.exe config ArgusConnector binPath= $cmd start= auto|Out-Null};& sc.exe failure ArgusConnector reset= 86400 actions= restart/5000/restart/15000/restart/60000|Out-Null;Start-Service ArgusConnector;(Get-Service ArgusConnector).WaitForStatus('Running',[TimeSpan]::FromSeconds(30));Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue;$parent=Split-Path $stage;if((Test-Path -LiteralPath $parent)-and -not(Get-ChildItem -LiteralPath $parent -Force)){Remove-Item -LiteralPath $parent -Force}`
	return script
}

func sshRun(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	output, err := session.CombinedOutput(command)
	if err != nil {
		return fmt.Errorf("remote install command failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
func sshRunReader(client *ssh.Client, command string, input io.Reader) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdin = input
	output, err := session.CombinedOutput(command)
	if err != nil {
		return fmt.Errorf("remote install transfer failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
func windowsSSHWrite(client *ssh.Client, path string, content []byte) error {
	script := `$p=` + psLiteral(path) + `;New-Item -ItemType Directory -Force -Path ([IO.Path]::GetDirectoryName($p))|Out-Null;$f=[IO.File]::Open($p,'Create','Write','None');try{[Console]::OpenStandardInput().CopyTo($f)}finally{$f.Dispose()}`
	return sshRunReader(client, sshtarget.PowerShellCommand(script), bytes.NewReader(content))
}
func shellLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
func psLiteral(value string) string    { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
func parseUUID(value string) uuid.UUID { parsed, _ := uuid.Parse(value); return parsed }
