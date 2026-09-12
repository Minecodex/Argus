package directexecutor

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/ssh"

	"github.com/kakj-go/Argus/internal/artifacthttp"
	"github.com/kakj-go/Argus/internal/collectormanager"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/hostonboarding"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/operationsecret"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/sshtarget"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (executor *Executor) runHostOnboardingLoop(ctx context.Context) {
	ticker := time.NewTicker(700 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = executor.Store.Queries.RecoverHostOnboardingOperations(ctx)
			_, _ = executor.Store.Queries.ExpireHostOnboardingOperations(ctx)
			reserved := executor.reserveAvailable()
			if reserved == 0 {
				continue
			}
			operations, err := executor.Store.Queries.ClaimHostOnboardingOperations(ctx, db.ClaimHostOnboardingOperationsParams{Limit: int32(reserved), LeaseOwner: executor.InstanceID})
			if err != nil {
				executor.release(reserved)
				continue
			}
			executor.release(reserved - len(operations))
			for _, operation := range operations {
				go func(item db.HostOnboardingOperation) {
					defer executor.release(1)
					executor.executeHostOnboarding(ctx, item)
				}(operation)
			}
		}
	}
}

func (executor *Executor) executeHostOnboarding(parent context.Context, operation db.HostOnboardingOperation) {
	ctx, cancel := context.WithDeadline(parent, operation.ExpiresAt.Time)
	defer cancel()
	leaseDone := make(chan struct{})
	defer close(leaseDone)
	go watchInstallAuthority(ctx, cancel, leaseDone, func(readCtx context.Context) (bool, error) {
		current, err := executor.Store.Queries.GetHostOnboardingOperation(readCtx, db.GetHostOnboardingOperationParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID})
		return err == nil && current.Status == "running" && current.LeaseOwner == operation.LeaseOwner &&
			current.Fence == operation.Fence && current.Attempts == operation.Attempts && current.LeaseExpiresAt.Valid && time.Now().Before(current.LeaseExpiresAt.Time), err
	})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-leaseDone:
				return
			case <-ticker.C:
				rows, err := executor.Store.Queries.RenewHostOnboardingOperationLease(ctx, db.RenewHostOnboardingOperationLeaseParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID, LeaseOwner: operation.LeaseOwner})
				if err != nil || rows != 1 {
					cancel()
					return
				}
			}
		}
	}()
	if err := hostonboarding.RecordClaim(ctx, executor.Store.Queries, operation); err != nil {
		executor.failHostOnboarding(ctx, operation, "HOST_ONBOARDING_PROGRESS_FAILED", err)
		return
	}
	var plan installation.HostConnectorInstallPlan
	if json.Unmarshal(operation.Plan, &plan) != nil || plan.HostID != operation.HostID || plan.ConnectorID != operation.ConnectorID || plan.SSHPath != "direct_executor" {
		executor.failHostOnboarding(ctx, operation, "HOST_ONBOARDING_PLAN_INVALID", errors.New("host onboarding plan is invalid"))
		return
	}
	canonical, err := json.Marshal(plan)
	if err != nil || !bytes.Equal(operation.PlanHash, hashBytes(canonical)) {
		executor.failHostOnboarding(ctx, operation, "HOST_ONBOARDING_PLAN_INVALID", err)
		return
	}
	secretRecord, err := executor.Store.Queries.GetHostOnboardingOperationSecret(ctx, db.GetHostOnboardingOperationSecretParams{OperationID: operation.ID, EnterpriseID: operation.EnterpriseID})
	if err != nil {
		executor.failHostOnboarding(ctx, operation, "HOST_ONBOARDING_SECRET_UNAVAILABLE", err)
		return
	}
	material, err := operationsecret.Decrypt(executor.OperationSecretKey, secretRecord.Nonce, secretRecord.Ciphertext, operation.EnterpriseID, operation.ID)
	if err != nil {
		executor.failHostOnboarding(ctx, operation, "HOST_ONBOARDING_SECRET_UNAVAILABLE", err)
		return
	}
	if plan.ControlPath == "executor_tunnel" {
		if err = executor.waitHostControlTunnel(ctx, operation); err != nil {
			executor.failHostOnboarding(ctx, operation, "HOST_CONTROL_TUNNEL_UNAVAILABLE", err)
			return
		}
	}
	if err = executor.installHostConnectorOverSSH(ctx, operation, plan, material.EnrollmentToken); err != nil {
		executor.failHostOnboarding(ctx, operation, hostOnboardingErrorCode(err), err)
		return
	}
	_, _ = executor.Store.Queries.ConsumeHostOnboardingOperationSecret(ctx, db.ConsumeHostOnboardingOperationSecretParams{OperationID: operation.ID, EnterpriseID: operation.EnterpriseID})
	_ = hostonboarding.Advance(ctx, executor.Store.Queries, operation.ID, operation.EnterpriseID, "waiting_online")
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		connector, getErr := executor.Store.Queries.GetConnector(ctx, db.GetConnectorParams{ID: operation.ConnectorID, EnterpriseID: operation.EnterpriseID})
		if getErr == nil && connector.Status == "online" {
			_ = hostonboarding.Complete(ctx, executor.Store.Queries, operation.ID, operation.EnterpriseID)
			return
		}
		select {
		case <-ctx.Done():
			executor.failHostOnboarding(ctx, operation, "HOST_CONNECTOR_ONLINE_TIMEOUT", ctx.Err())
			return
		case <-time.After(time.Second):
		}
	}
	executor.failHostOnboarding(ctx, operation, "HOST_CONNECTOR_ONLINE_TIMEOUT", errors.New("Host Connector did not become online"))
}

func (executor *Executor) waitHostControlTunnel(ctx context.Context, operation db.HostOnboardingOperation) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		tunnel, err := executor.Store.Queries.GetConnectorControlTunnelByConnector(ctx, db.GetConnectorControlTunnelByConnectorParams{
			ConnectorID: operation.ConnectorID, EnterpriseID: operation.EnterpriseID})
		if err == nil && tunnel.Status == "established" {
			return nil
		}
		if err == nil && tunnel.Status == "down" && tunnel.LastDropReason == "tunnel_quota_exceeded" {
			return errors.New("control tunnel quota exceeded")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (executor *Executor) installHostConnectorOverSSH(ctx context.Context, operation db.HostOnboardingOperation, plan installation.HostConnectorInstallPlan, enrollmentToken string) error {
	addresses, err := executor.Validator.Resolve(ctx, plan.Address)
	if err != nil || len(addresses) == 0 {
		return errors.New("SSH target is unavailable")
	}
	credential, err := executor.Store.Queries.GetCredential(ctx, db.GetCredentialParams{ID: plan.CredentialID, EnterpriseID: operation.EnterpriseID})
	if err != nil || credential.Status != "active" || credential.Version != plan.CredentialVersion {
		return errors.New("SSH credential changed")
	}
	lease, err := executor.Secrets.IssueLease(secret.WithActorType(ctx, "direct_executor"), executor.InstanceID, operation.EnterpriseID, secret.LeaseRequest{
		CredentialID: plan.CredentialID, OperationRef: "host_onboarding:" + operation.ID.String(), TargetResourceType: "host", TargetResourceID: operation.HostID,
		RecipientType: "direct_executor", RecipientID: executor.InstanceID, Protocol: "ssh", TTL: secret.MaxLeaseTTL})
	if err != nil {
		return err
	}
	defer clear(lease.Value)
	client, err := executor.dialPinnedSSH(ctx, addresses[0], plan.Port, plan.Username, plan.PinnedHostKey, lease.Value)
	if err != nil {
		return err
	}
	defer client.Close()
	stopOnCancel := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stopOnCancel()
	if err = executor.Validator.Revalidate(ctx, plan.Address, addresses); err != nil {
		return err
	}
	evidence, err := sshtarget.Probe(client, plan.TargetPlatform.OS())
	if err != nil || evidence.Platform+"_"+evidence.Architecture != string(plan.TargetPlatform) || evidence.DistributionVersion != plan.DistributionVersion {
		return errors.New("SSH target platform changed")
	}
	callbacks, err := plan.CallbackAddresses()
	if err != nil {
		return err
	}
	if err = sshtarget.ProbeCallbacks(ctx, client, callbacks); err != nil {
		return err
	}
	if err = sshtarget.ProbeEnrollment(ctx, client, plan.EnrollmentEndpoint, plan.EnrollDialAddress, plan.TrustBundlePEM); err != nil {
		return err
	}
	if err = sshtarget.ProbeGateway(ctx, client, plan.GatewayEndpoint, plan.GatewayDialAddress, plan.TrustBundlePEM); err != nil {
		return err
	}
	if err = hostonboarding.Advance(ctx, executor.Store.Queries, operation.ID, operation.EnterpriseID, "transferring"); err != nil {
		return err
	}
	publicRaw, err := base64.RawStdEncoding.DecodeString(plan.SigningPublicKey)
	if err != nil || len(publicRaw) != ed25519.PublicKeySize {
		return errors.New("Connector signing key is invalid")
	}
	artifactFile, err := os.CreateTemp("", ".argus-host-connector-*")
	if err != nil {
		return err
	}
	artifactPath := artifactFile.Name()
	defer os.Remove(artifactPath)
	defer artifactFile.Close()
	descriptor := &connectorv1.CollectorArtifact{Platform: plan.Artifact.Platform, Uri: plan.Artifact.URI, Sha256: plan.Artifact.SHA256, Signature: plan.Artifact.Signature, SigningKeyId: plan.Artifact.SigningKeyID, ByteSize: uint64(plan.Artifact.ByteSize)}
	if err = (collectormanager.Manager{HTTPClient: executor.CollectorArtifactHTTPClient, TrustedSigningKeys: map[string]ed25519.PublicKey{plan.Artifact.SigningKeyID: publicRaw}}).FetchArtifactTo(ctx, descriptor, artifactFile); err != nil {
		return err
	}
	if _, err = artifactFile.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err = hostonboarding.Advance(ctx, executor.Store.Queries, operation.ID, operation.EnterpriseID, "installing"); err != nil {
		return err
	}
	if plan.TargetPlatform.OS() == "windows" {
		return installWindowsHostConnector(client, artifactFile, plan, enrollmentToken)
	}
	return installLinuxHostConnector(client, artifactFile, plan, enrollmentToken)
}

func installLinuxHostConnector(client *ssh.Client, binary io.Reader, plan installation.HostConnectorInstallPlan, token string) error {
	stage := "/var/lib/argus-connector-install/" + plan.ConnectorID.String()
	upload := `set -eu; umask 077; install -d -m 0700 ` + shellQuote(stage) + `; tmp=$(mktemp ` + shellQuote(stage+`/.argus-connector.XXXXXX`) + `); cat > "$tmp"; chmod 0700 "$tmp"; mv -f "$tmp" ` + shellQuote(stage+`/argus-connector`)
	if err := runSSHCommandReader(client, upload, binary); err != nil {
		return err
	}
	prepare := `set -eu; id argus-connector >/dev/null 2>&1 || useradd --system --home-dir /var/lib/argus-connector --shell /usr/sbin/nologin argus-connector; install -d -m 0700 -o argus-connector -g argus-connector /var/lib/argus-connector; install -d -m 0755 /etc/argus-connector; install -d -m 0700 /var/lib/argus-otelcol /etc/argus-otelcol; current=""; if [ -f /var/lib/argus-connector/identity.json ]; then current=$(sed -n 's/.*"connector_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' /var/lib/argus-connector/identity.json | sed -n '1p'); fi; [ -n "$current" ] || { [ ! -f /var/lib/argus-connector/.desired-connector-id ] || current=$(sed -n '1p' /var/lib/argus-connector/.desired-connector-id); }; printf '%s' "$current" > ` + shellQuote(stage+`/previous-connector-id`)
	if err := runSSHCommand(client, prepare, nil); err != nil {
		return err
	}
	if err := writeSSHFile(client, `umask 077; cat > `+shellQuote(stage+`/server-ca.pem`)+`; chmod 0600 `+shellQuote(stage+`/server-ca.pem`), plan.TrustBundlePEM); err != nil {
		return err
	}
	signingKeys, _ := json.Marshal(map[string]string{plan.Artifact.SigningKeyID: plan.SigningPublicKey})
	if err := writeSSHFile(client, `umask 077; cat > `+shellQuote(stage+`/otelcol-signing-keys.json`)+`; chmod 0600 `+shellQuote(stage+`/otelcol-signing-keys.json`), signingKeys); err != nil {
		return err
	}
	if err := writeSSHFile(client, `umask 077; cat > `+shellQuote(stage+`/enrollment-token`)+`; chmod 0600 `+shellQuote(stage+`/enrollment-token`), []byte(token)); err != nil {
		return err
	}
	envEnroll := ""
	if plan.EnrollDialAddress != "" {
		envEnroll = "ARGUS_CONNECTOR_ENROLL_ADDRESS=" + shellQuote(plan.EnrollDialAddress) + " "
	}
	enroll := envEnroll + shellQuote(stage+`/argus-connector`) + ` enroll --connector-id ` + shellQuote(plan.ConnectorID.String()) + ` --token-file ` + shellQuote(stage+`/enrollment-token`) + ` --server ` + shellQuote(plan.EnrollmentEndpoint) + ` --ca-file ` + shellQuote(stage+`/server-ca.pem`) + ` --role host --data-dir /var/lib/argus-connector`
	if err := runSSHCommand(client, enroll, nil); err != nil {
		return err
	}
	promote := `set -eu; current=$(cat ` + shellQuote(stage+`/previous-connector-id`) + `); systemctl disable --now argus-connector-privileged.service >/dev/null 2>&1 || true; systemctl disable --now argus-connector.service >/dev/null 2>&1 || true; if [ -n "$current" ] && [ "$current" != ` + shellQuote(plan.ConnectorID.String()) + ` ]; then systemctl disable --now argus-otelcol.service >/dev/null 2>&1 || true; rm -f /etc/systemd/system/argus-otelcol.service /usr/local/bin/argus-otelcol; rm -rf /var/lib/argus-otelcol /etc/argus-otelcol; install -d -m 0700 /var/lib/argus-otelcol /etc/argus-otelcol; fi; install -m 0755 ` + shellQuote(stage+`/argus-connector`) + ` /usr/local/bin/argus-connector; install -m 0600 ` + shellQuote(stage+`/server-ca.pem`) + ` /etc/argus-connector/server-ca.pem; install -m 0600 ` + shellQuote(stage+`/otelcol-signing-keys.json`) + ` /etc/argus-connector/otelcol-signing-keys.json; printf '%s' ` + shellQuote(plan.ConnectorID.String()) + ` > /var/lib/argus-connector/.desired-connector-id`
	if err := runSSHCommand(client, promote, nil); err != nil {
		return err
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
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/argus-connector /etc/argus-connector

[Install]
WantedBy=multi-user.target
`
	if err := writeSSHFile(client, `cat > /etc/systemd/system/argus-connector.service; chmod 0644 /etc/systemd/system/argus-connector.service`, []byte(unit)); err != nil {
		return err
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
	if err := writeSSHFile(client, `cat > /etc/systemd/system/argus-connector-privileged.service; chmod 0644 /etc/systemd/system/argus-connector-privileged.service`, []byte(helper)); err != nil {
		return err
	}
	return runSSHCommand(client, `chown -R argus-connector:argus-connector /var/lib/argus-connector /etc/argus-connector; systemctl daemon-reload; systemctl enable --now argus-connector-privileged.service; systemctl enable --now argus-connector.service; systemctl is-active --quiet argus-connector.service; rm -rf `+shellQuote(stage)+`; rmdir /var/lib/argus-connector-install >/dev/null 2>&1 || true`, nil)
}

func installWindowsHostConnector(client *ssh.Client, binary io.Reader, plan installation.HostConnectorInstallPlan, token string) error {
	stage := `C:\ProgramData\Argus\Install\` + plan.ConnectorID.String()
	writeBinary := sshtarget.PowerShellCommand(`$ErrorActionPreference='Stop';$p=` + powerShellLiteral(stage) + `;New-Item -ItemType Directory -Force -Path $p|Out-Null;$f=[IO.File]::Open((Join-Path $p 'argus-connector.next.exe'),[IO.FileMode]::Create,[IO.FileAccess]::Write,[IO.FileShare]::None);try{[Console]::OpenStandardInput().CopyTo($f)}finally{$f.Dispose()}`)
	if err := runSSHCommandReader(client, writeBinary, binary); err != nil {
		return err
	}
	prepare := sshtarget.PowerShellCommand(`$ErrorActionPreference='Stop';$p='C:\ProgramData\Argus\Connector';$stage=` + powerShellLiteral(stage) + `;New-Item -ItemType Directory -Force -Path $p,$stage|Out-Null;foreach($securePath in @($p,$stage)){& icacls.exe $securePath /inheritance:r /grant:r 'SYSTEM:(OI)(CI)(F)' 'Administrators:(OI)(CI)(F)'|Out-Null;if($LASTEXITCODE-ne 0){throw 'ACL failed'}};$current='';$identity=Join-Path $p 'identity.json';if(Test-Path -LiteralPath $identity){$current=[string]((Get-Content -LiteralPath $identity -Raw|ConvertFrom-Json).connector_id)};[IO.File]::WriteAllText((Join-Path $stage 'previous-connector-id'),$current,(New-Object Text.UTF8Encoding($false)))`)
	if err := runSSHCommand(client, prepare, nil); err != nil {
		return err
	}
	if err := writeWindowsSSHFile(client, stage+`\server-ca.pem`, plan.TrustBundlePEM); err != nil {
		return err
	}
	signingKeys, _ := json.Marshal(map[string]string{plan.Artifact.SigningKeyID: plan.SigningPublicKey})
	if err := writeWindowsSSHFile(client, stage+`\otelcol-signing-keys.json`, signingKeys); err != nil {
		return err
	}
	if err := writeWindowsSSHFile(client, stage+`\enrollment-token`, []byte(token)); err != nil {
		return err
	}
	script := buildWindowsHostTakeoverScript(plan, stage)
	return runSSHCommand(client, sshtarget.PowerShellCommand(script), nil)
}

func buildWindowsHostTakeoverScript(plan installation.HostConnectorInstallPlan, stage string) string {
	script := `$ErrorActionPreference='Stop';$stage=` + powerShellLiteral(stage) + `;$next=Join-Path $stage 'argus-connector.next.exe';$bin='C:\Program Files\Argus\Connector\argus-connector.exe';$state='C:\ProgramData\Argus\Connector';`
	if plan.EnrollDialAddress != "" {
		script += `$env:ARGUS_CONNECTOR_ENROLL_ADDRESS=` + powerShellLiteral(plan.EnrollDialAddress) + `;`
	}
	script += `& $next enroll --connector-id ` + powerShellLiteral(plan.ConnectorID.String()) + ` --token-file (Join-Path $stage 'enrollment-token') --server ` + powerShellLiteral(plan.EnrollmentEndpoint) + ` --ca-file (Join-Path $stage 'server-ca.pem') --role host --data-dir $state;if($LASTEXITCODE-ne 0){throw 'enrollment failed'};$current=[IO.File]::ReadAllText((Join-Path $stage 'previous-connector-id')).Trim();if($current -and $current -ne ` + powerShellLiteral(plan.ConnectorID.String()) + `){Stop-Service ArgusCollector -Force -ErrorAction SilentlyContinue;& sc.exe delete ArgusCollector|Out-Null;Remove-Item -LiteralPath 'C:\Program Files\Argus\Collector','C:\ProgramData\Argus\Collector' -Recurse -Force -ErrorAction SilentlyContinue};foreach($serviceName in @('ArgusConnectorPrivileged','ArgusConnector')){$service=Get-Service $serviceName -ErrorAction SilentlyContinue;if($service -and $service.Status -ne 'Stopped'){Stop-Service $serviceName -Force;$service.WaitForStatus('Stopped',[TimeSpan]::FromSeconds(30))}};New-Item -ItemType Directory -Force -Path (Split-Path $bin)|Out-Null;$copied=$false;for($attempt=0;$attempt -lt 20;$attempt++){try{Copy-Item -LiteralPath $next -Destination $bin -Force;$copied=$true;break}catch{Start-Sleep -Milliseconds 250}};if(-not $copied){throw 'Connector executable replacement failed'};Copy-Item -LiteralPath (Join-Path $stage 'server-ca.pem') -Destination (Join-Path $state 'server-ca.pem') -Force;Copy-Item -LiteralPath (Join-Path $stage 'otelcol-signing-keys.json') -Destination (Join-Path $state 'otelcol-signing-keys.json') -Force;[IO.File]::WriteAllText((Join-Path $state '.desired-connector-id'),` + powerShellLiteral(plan.ConnectorID.String()) + `,(New-Object Text.UTF8Encoding($false)));`
	if plan.GatewayDialAddress != "" {
		script += `[Environment]::SetEnvironmentVariable('ARGUS_CONNECTOR_DIAL_ADDRESS',` + powerShellLiteral(plan.GatewayDialAddress) + `,'Machine');`
	}
	if plan.EnrollDialAddress != "" {
		script += `[Environment]::SetEnvironmentVariable('ARGUS_ARTIFACT_DIAL_ADDRESS',` + powerShellLiteral(plan.EnrollDialAddress) + `,'Machine');`
	}
	script += `[Environment]::SetEnvironmentVariable('ARGUS_OTELCOL_SIGNING_PUBLIC_KEYS_FILE',(Join-Path $state 'otelcol-signing-keys.json'),'Machine');`
	script += `$cmd='"'+$bin+'" run --data-dir "'+$state+'"';& sc.exe create ArgusConnector binPath= $cmd start= auto|Out-Null;if($LASTEXITCODE-ne 0){& sc.exe config ArgusConnector binPath= $cmd start= auto|Out-Null};& sc.exe failure ArgusConnector reset= 86400 actions= restart/5000/restart/15000/restart/60000|Out-Null;Start-Service ArgusConnector;(Get-Service ArgusConnector).WaitForStatus('Running',[TimeSpan]::FromSeconds(30));Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue;$parent=Split-Path $stage;if((Test-Path -LiteralPath $parent)-and -not(Get-ChildItem -LiteralPath $parent -Force)){Remove-Item -LiteralPath $parent -Force}`
	return script
}

func writeSSHFile(client *ssh.Client, command string, content []byte) error {
	return runSSHCommandReader(client, command, bytes.NewReader(content))
}
func writeWindowsSSHFile(client *ssh.Client, path string, content []byte) error {
	script := `$f=[IO.File]::Open(` + powerShellLiteral(path) + `,[IO.FileMode]::Create,[IO.FileAccess]::Write,[IO.FileShare]::None);try{[Console]::OpenStandardInput().CopyTo($f)}finally{$f.Dispose()}`
	return runSSHCommandReader(client, sshtarget.PowerShellCommand(script), bytes.NewReader(content))
}
func powerShellLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
func hashBytes(value []byte) []byte         { digest := sha256.Sum256(value); return digest[:] }

func (executor *Executor) failHostOnboarding(ctx context.Context, operation db.HostOnboardingOperation, code string, cause error) {
	slog.Warn("Host onboarding attempt failed", "operation_id", operation.ID, "host_id", operation.HostID, "attempt", operation.Attempts, "error_code", code,
		"callback_error", sshtarget.CallbackFailureDetail(cause))
	if operation.Attempts < 3 && time.Now().Before(operation.ExpiresAt.Time) {
		_ = hostonboarding.RecordOutcome(ctx, executor.Store.Queries, operation.ID, operation.EnterpriseID, "retrying", code)
		if _, err := executor.Store.Queries.RetryHostOnboardingOperation(ctx, db.RetryHostOnboardingOperationParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID, ErrorCode: pgtype.Text{String: code, Valid: true}}); err == nil {
			return
		}
	}
	_ = hostonboarding.RecordOutcome(ctx, executor.Store.Queries, operation.ID, operation.EnterpriseID, "failed", code)
	_, _ = executor.Store.Queries.FailHostOnboardingOperation(ctx, db.FailHostOnboardingOperationParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID, ErrorCode: pgtype.Text{String: code, Valid: true}})
}
func hostOnboardingErrorCode(err error) string {
	if code := sshtarget.CallbackFailureCode(err); code != "" {
		return "HOST_ONBOARDING_CALLBACK_" + code
	}
	if code := artifacthttp.FailureCode(err); code != "" {
		return "HOST_ONBOARDING_ARTIFACT_" + code
	}
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "credential"):
		return "HOST_ONBOARDING_CREDENTIAL_INVALID"
	case strings.Contains(text, "host key"):
		return "HOST_ONBOARDING_HOST_KEY_CHANGED"
	case strings.Contains(text, "platform"):
		return "HOST_ONBOARDING_PLATFORM_CHANGED"
	case strings.Contains(text, "artifact"):
		return "HOST_ONBOARDING_ARTIFACT_INVALID"
	default:
		return "HOST_ONBOARDING_FAILED"
	}
}
