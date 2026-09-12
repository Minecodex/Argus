package hostremoval

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kakj-go/Argus/internal/installinstruction"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (service Service) createInstruction(ctx context.Context, q *db.Queries, operation db.HostRemovalOperation) (Instruction, error) {
	if operation.DeliveryMethod != "manual" || operation.Status != "awaiting_manual_execution" && operation.Status != "cleanup_unknown" && operation.Status != "failed" {
		return Instruction{}, ErrOperationState
	}
	if _, err := q.RevokeHostRemovalTokens(ctx, db.RevokeHostRemovalTokensParams{OperationID: operation.ID, EnterpriseID: operation.EnterpriseID}); err != nil {
		return Instruction{}, err
	}
	bootstrapToken, bootstrapHash, err := newToken()
	if err != nil {
		return Instruction{}, err
	}
	receiptToken, receiptHash, err := newToken()
	if err != nil {
		return Instruction{}, err
	}
	bootstrapNonce, bootstrapCiphertext, err := sealToken(service.TokenKey, operation.EnterpriseID, operation.ID,
		tokenEnvelope{Token: bootstrapToken, ReceiptToken: receiptToken})
	if err != nil {
		return Instruction{}, err
	}
	receiptNonce, receiptCiphertext, err := sealToken(service.TokenKey, operation.EnterpriseID, operation.ID, tokenEnvelope{Token: receiptToken})
	if err != nil {
		return Instruction{}, err
	}
	expires := time.Now().UTC().Add(tokenTTL)
	if operation.ExpiresAt.Valid && operation.ExpiresAt.Time.Before(expires) {
		expires = operation.ExpiresAt.Time
	}
	for _, token := range []db.CreateHostRemovalTokenParams{
		{ID: uuid.New(), OperationID: operation.ID, EnterpriseID: operation.EnterpriseID, Purpose: "bootstrap", TokenHash: bootstrapHash,
			KeyVersion: tokenKeyVersion, Nonce: bootstrapNonce, Ciphertext: bootstrapCiphertext, ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}},
		{ID: uuid.New(), OperationID: operation.ID, EnterpriseID: operation.EnterpriseID, Purpose: "receipt", TokenHash: receiptHash,
			KeyVersion: tokenKeyVersion, Nonce: receiptNonce, Ciphertext: receiptCiphertext, ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}},
	} {
		if _, err := q.CreateHostRemovalToken(ctx, token); err != nil {
			return Instruction{}, err
		}
	}
	var plan Plan
	if json.Unmarshal(operation.Plan, &plan) != nil || !plan.OperationID.Valid || plan.OperationID.UUID != operation.ID || plan.ConnectorID != operation.ConnectorID || plan.RemovalGeneration != operation.RemovalGeneration {
		return Instruction{}, ErrIdentityChanged
	}
	receiptURL, err := service.receiptURL()
	if err != nil {
		return Instruction{}, err
	}
	script := buildRemovalScript(plan, receiptURL, receiptToken)
	digest := sha256.Sum256([]byte(script))
	bootstrapURL, err := service.bootstrapURL(operation.ID)
	if err != nil {
		return Instruction{}, err
	}
	command, shell, platform := buildDownloadCommand(plan, bootstrapURL, bootstrapToken, hex.EncodeToString(digest[:]))
	bundle, err := service.TrustBundles.Current(ctx)
	if err != nil || bundle.Epoch != operation.TrustBundleEpoch {
		return Instruction{}, ErrIdentityChanged
	}
	return Instruction{OperationID: operation.ID, Set: installinstruction.Set{Platform: platform, Shell: shell, Privilege: "system",
		Scope: removalScope(platform), Command: command, BootstrapTLSMode: "strict", ReleaseVersion: "removal-v1", ExpiresAt: expires,
		TrustBundleEpoch: bundle.Epoch, TrustBundleSHA256: bundle.Material.SHA256, BootstrapSHA256: hex.EncodeToString(digest[:]),
		InstallerSHA256: hex.EncodeToString(digest[:]), CapabilityWarnings: []string{"Stops Argus-managed services before removing local files."}}}, nil
}

func (service Service) RegenerateInstruction(ctx context.Context, actorID string, enterpriseID, operationID uuid.UUID, idempotencyKey string) (Instruction, error) {
	return postgres.ExecuteIdempotent(ctx, service.Store, service.Actions.Idempotency, "enterprise", actorID, "host_removal.command.regenerate",
		idempotencyKey, map[string]any{"operation_id": operationID}, 201, func(q *db.Queries) (Instruction, error) {
			operation, err := q.GetHostRemovalOperationForUpdate(ctx, db.GetHostRemovalOperationForUpdateParams{ID: operationID, EnterpriseID: enterpriseID})
			if err != nil || operation.DeliveryMethod != "manual" || operation.Status != "awaiting_manual_execution" && operation.Status != "running" {
				return Instruction{}, ErrOperationState
			}
			if err = service.validateOperationFence(ctx, q, operation); err != nil {
				return Instruction{}, err
			}
			// Reissuing the command rotates tokens for this operation, including
			// after a lost download/receipt. It does not create another removal.
			operation.Status = "awaiting_manual_execution"
			return service.createInstruction(ctx, q, operation)
		})
}

func (service Service) ClaimBootstrap(ctx context.Context, operationID uuid.UUID, plainToken string) (string, string, error) {
	var script, contentType string
	err := service.Store.InTx(ctx, func(q *db.Queries) error {
		token, err := q.GetActiveHostRemovalToken(ctx, tokenDigest(plainToken))
		if err != nil || token.OperationID != operationID || token.Purpose != "bootstrap" || (token.Status != "active" && token.Status != "consumed") || token.KeyVersion != tokenKeyVersion ||
			!token.ExpiresAt.Valid || !time.Now().UTC().Before(token.ExpiresAt.Time) {
			return ErrTokenInvalid
		}
		envelope, err := openToken(service.TokenKey, token.Nonce, token.Ciphertext, token.EnterpriseID, token.OperationID)
		if err != nil || envelope.Token != plainToken || envelope.ReceiptToken == "" {
			return ErrTokenInvalid
		}
		operation, err := q.GetHostRemovalOperationForUpdate(ctx, db.GetHostRemovalOperationForUpdateParams{ID: operationID, EnterpriseID: token.EnterpriseID})
		if err != nil || operation.DeliveryMethod != "manual" || (operation.Status != "awaiting_manual_execution" && operation.Status != "cleanup_unknown" && operation.Status != "running") {
			return ErrOperationState
		}
		if err = service.validateOperationFence(ctx, q, operation); err != nil {
			return err
		}
		var plan Plan
		if json.Unmarshal(operation.Plan, &plan) != nil {
			return ErrIdentityChanged
		}
		receiptURL, err := service.receiptURL()
		if err != nil {
			return err
		}
		script = buildRemovalScript(plan, receiptURL, envelope.ReceiptToken)
		contentType = "text/x-shellscript"
		if strings.HasPrefix(operation.TargetPlatform, "windows_") {
			contentType = "text/x-powershell"
		}
		if token.Status == "consumed" {
			// A retry can recover a response lost after the first successful GET.
			return nil
		}
		if rows, consumeErr := q.ConsumeHostRemovalToken(ctx, db.ConsumeHostRemovalTokenParams{ID: token.ID, OperationID: operation.ID}); consumeErr != nil || rows != 1 {
			return ErrTokenInvalid
		}
		if operation.TargetType == TargetBastion {
			if rows, updateErr := q.MarkRemovalBastionUninstalling(ctx, db.MarkRemovalBastionUninstallingParams{ID: operation.BastionScopeID.UUID,
				EnterpriseID: operation.EnterpriseID, ActiveConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true},
				RemovalGeneration: operation.RemovalGeneration}); updateErr != nil || rows != 1 {
				return ErrIdentityChanged
			}
		} else if rows, updateErr := q.MarkRemovalHostUninstalling(ctx, db.MarkRemovalHostUninstallingParams{ID: operation.HostID,
			EnterpriseID: operation.EnterpriseID, ConnectorID: uuid.NullUUID{UUID: operation.ConnectorID, Valid: true},
			RemovalGeneration: operation.RemovalGeneration}); updateErr != nil || rows != 1 {
			return ErrIdentityChanged
		}
		_, err = q.AdvanceHostRemovalOperation(ctx, db.AdvanceHostRemovalOperationParams{ID: operation.ID, EnterpriseID: operation.EnterpriseID,
			Status: "running", Stage: "uninstalling_workloads"})
		return err
	})
	return script, contentType, err
}

func buildDownloadCommand(plan Plan, bootstrapURL, token, digest string) (string, string, string) {
	if strings.HasPrefix(plan.TargetPlatform, "windows_") {
		connect := ""
		if value := dialArgument(plan.HTTPSDialAddress); value != "" {
			connect = " --connect-to " + psQuote("::"+value)
		}
		command := `$ErrorActionPreference='Stop';$ca='C:\ProgramData\Argus\Connector\connector-ca.pem';if(-not(Test-Path -LiteralPath $ca)){$ca=` + psQuote(`C:\ProgramData\Argus\Uninstall\`+plan.OperationID.UUID.String()+`\server-ca.pem`) + `};$f=Join-Path ([IO.Path]::GetTempPath()) ('argus-remove-'+[Guid]::NewGuid().ToString('N')+'.ps1');try{& curl.exe -fsS --proto '=https' --tlsv1.2 --ssl-revoke-best-effort --cacert $ca` + connect + ` --header ` + psQuote("X-Argus-Removal-Token: "+token) + ` --output $f ` + psQuote(bootstrapURL) + `;if($LASTEXITCODE-ne 0){throw 'Argus removal bootstrap download failed'};if((Get-FileHash -LiteralPath $f -Algorithm SHA256).Hash.ToLowerInvariant()-ne ` + psQuote(digest) + `){throw 'Argus removal bootstrap SHA-256 mismatch'};& powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File $f}finally{Remove-Item -LiteralPath $f -Force -ErrorAction SilentlyContinue}`
		return command, "powershell", "windows"
	}
	connect := ""
	if value := dialArgument(plan.HTTPSDialAddress); value != "" {
		connect = " --connect-to " + shQuote("::"+value)
	}
	command := `(set -eu; umask 077; ARGUS_CA=/var/lib/argus-connector/connector-ca.pem; [ -r "$ARGUS_CA" ] || ARGUS_CA=` + shQuote("/var/lib/argus-uninstall/"+plan.OperationID.UUID.String()+"/server-ca.pem") + `; ARGUS_REMOVE=$(mktemp "${TMPDIR:-/tmp}/argus-remove.XXXXXX"); trap 'rm -f "$ARGUS_REMOVE"' EXIT HUP INT TERM; ARGUS_HTTP=$(curl -sS --proto '=https' --tlsv1.2 --cacert "$ARGUS_CA"` + connect + ` --header ` + shQuote("X-Argus-Removal-Token: "+token) + ` --output "$ARGUS_REMOVE" --write-out '%{http_code}' ` + shQuote(bootstrapURL) + `); if [ "$ARGUS_HTTP" != 200 ]; then ARGUS_ERROR=$(sed -n 's/.*"code"[[:space:]]*:[[:space:]]*"\([A-Z_][A-Z_0-9]*\)".*/\1/p' "$ARGUS_REMOVE" | head -n 1); printf 'Argus removal bootstrap failed (HTTP %s): %s\n' "$ARGUS_HTTP" "${ARGUS_ERROR:-REQUEST_FAILED}" >&2; exit 1; fi; printf '%s  %s\n' ` + shQuote(digest) + ` "$ARGUS_REMOVE" | sha256sum -c - >/dev/null; chmod 0700 "$ARGUS_REMOVE"; sh "$ARGUS_REMOVE")`
	return command, "posix_sh", "linux"
}

func removalScope(platform string) installinstruction.Scope {
	if platform == "windows" {
		return installinstruction.ScopeWindowsSystem
	}
	return installinstruction.ScopeLinuxSystem
}

func shQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
func psQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func buildRemovalScript(plan Plan, receiptURL, receiptToken string) string {
	if strings.HasPrefix(plan.TargetPlatform, "windows_") {
		return buildWindowsRemovalScript(plan, receiptURL, receiptToken)
	}
	return buildLinuxRemovalScript(plan, receiptURL, receiptToken)
}

func buildLinuxRemovalScript(plan Plan, receiptURL, receiptToken string) string {
	connect := ""
	if value := dialArgument(plan.HTTPSDialAddress); value != "" {
		connect = " --connect-to " + shQuote("::"+value)
	}
	return fmt.Sprintf(`#!/bin/sh
set -eu
umask 077
if [ "$(id -u)" -ne 0 ]; then command -v sudo >/dev/null 2>&1 || { echo 'sudo is required' >&2; exit 1; }; exec sudo sh "$0"; fi
OP=%s
CONNECTOR=%s
GENERATION=%d
%s
JOURNAL="/var/lib/argus-uninstall/$OP"
mkdir -p "$JOURNAL"
chmod 0700 /var/lib/argus-uninstall "$JOURNAL"
if [ ! -f "$JOURNAL/server-ca.pem" ]; then cp /var/lib/argus-connector/connector-ca.pem "$JOURNAL/server-ca.pem"; chmod 0600 "$JOURNAL/server-ca.pem"; fi
HELPER="$JOURNAL/argus-uninstaller"
if [ -s "$JOURNAL/evidence.json" ]; then
  EVIDENCE=$(cat "$JOURNAL/evidence.json")
else
  [ -x "$HELPER" ] || { cp /usr/local/bin/argus-connector "$HELPER"; chmod 0700 "$HELPER"; }
  EVIDENCE=$("$HELPER" uninstall-local --operation-id "$OP" --connector-id "$CONNECTOR" --removal-generation "$GENERATION" --journal-dir "$JOURNAL" --relay-https-port %d --relay-gateway-port %d --rdp-config-status not_applicable)
  printf '%%s' "$EVIDENCE" > "$JOURNAL/evidence.json"
fi
HASH=$(printf '%%s' "$EVIDENCE" | sha256sum | cut -d ' ' -f 1)
BODY="$JOURNAL/receipt.json"
printf '{"operation_id":"%%s","removal_generation":%%s,"connector_id":"%%s","stage":"verifying_cleanup","local_cleanup":"verified","result_hash":"%%s","evidence":%%s}' "$OP" "$GENERATION" "$CONNECTOR" "$HASH" "$EVIDENCE" > "$BODY"
curl -fsS --proto '=https' --tlsv1.2 --cacert "$JOURNAL/server-ca.pem"%s --header %s --header 'Content-Type: application/json' --data-binary "@$BODY" %s >/dev/null

printf '{"stage":"completed","completed":true}\n' > "$JOURNAL/state.json.tmp"
mv -f "$JOURNAL/state.json.tmp" "$JOURNAL/state.json"
rm -f "$JOURNAL/server-ca.pem" "$BODY" "$HELPER"
`, shQuote(plan.OperationID.UUID.String()), shQuote(plan.ConnectorID.String()), plan.RemovalGeneration, linuxRemovalIdentityGuard(plan), plan.RelayHTTPSPort, plan.RelayGatewayPort, connect,
		shQuote("X-Argus-Removal-Token: "+receiptToken), shQuote(receiptURL))
}

func buildWindowsRemovalScript(plan Plan, receiptURL, receiptToken string) string {
	connect := ""
	if value := dialArgument(plan.HTTPSDialAddress); value != "" {
		connect = " --connect-to " + psQuote("::"+value)
	}
	restore := windowsRDPRestoreSnippet(plan)
	return fmt.Sprintf(`$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$principal=New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if(-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){throw 'Windows Connector removal requires an elevated PowerShell'}
$operation=%s
%s
$journal=Join-Path $env:ProgramData ('Argus\Uninstall\'+$operation)
New-Item -ItemType Directory -Force -Path $journal|Out-Null
& icacls.exe $journal /inheritance:r /grant:r 'SYSTEM:(OI)(CI)(F)' 'Administrators:(OI)(CI)(F)'|Out-Null
$ca=Join-Path $journal 'server-ca.pem'
if(-not(Test-Path -LiteralPath $ca)){Copy-Item -LiteralPath (Join-Path $env:ProgramData 'Argus\Connector\connector-ca.pem') -Destination $ca}
$helper=Join-Path $journal 'argus-uninstaller.exe'
$evidenceFile=Join-Path $journal 'evidence.json'
if(Test-Path -LiteralPath $evidenceFile){
  $evidenceRaw=[IO.File]::ReadAllText($evidenceFile).Trim()
}else{
  if(-not(Test-Path -LiteralPath $helper)){Copy-Item -LiteralPath (Join-Path $env:ProgramFiles 'Argus\Connector\argus-connector.exe') -Destination $helper}
  %s
  $evidenceRaw=(& $helper uninstall-local --operation-id $operation --connector-id %s --removal-generation %d --journal-dir $journal --relay-https-port %d --relay-gateway-port %d --rdp-config-status $rdpStatus|Out-String).Trim()
  if($LASTEXITCODE-ne 0){throw 'Argus local cleanup postconditions failed'}
}
[IO.File]::WriteAllText($evidenceFile,$evidenceRaw,(New-Object Text.UTF8Encoding($false)))
$evidence=$evidenceRaw|ConvertFrom-Json
$hash=(Get-FileHash -LiteralPath $evidenceFile -Algorithm SHA256).Hash.ToLowerInvariant()
$body=Join-Path $journal 'receipt.json'
$receipt=[ordered]@{operation_id=$operation;removal_generation=%d;connector_id=%s;stage='verifying_cleanup';local_cleanup='verified';result_hash=$hash;evidence=$evidence}
[IO.File]::WriteAllText($body,($receipt|ConvertTo-Json -Compress),(New-Object Text.UTF8Encoding($false)))
& curl.exe -fsS --proto '=https' --tlsv1.2 --ssl-revoke-best-effort --cacert $ca%s --header %s --header 'Content-Type: application/json' --data-binary ('@'+$body) %s|Out-Null
if($LASTEXITCODE-ne 0){throw 'Argus removal receipt failed'}
[IO.File]::WriteAllText((Join-Path $journal 'state.json'),(@{stage='completed';completed=$true}|ConvertTo-Json -Compress),(New-Object Text.UTF8Encoding($false)))
Remove-Item -LiteralPath $ca,$body,$helper -Force -ErrorAction SilentlyContinue
`, psQuote(plan.OperationID.UUID.String()), windowsRemovalIdentityGuard(plan), restore, psQuote(plan.ConnectorID.String()), plan.RemovalGeneration, plan.RelayHTTPSPort, plan.RelayGatewayPort,
		plan.RemovalGeneration, psQuote(plan.ConnectorID.String()), connect, psQuote("X-Argus-Removal-Token: "+receiptToken), psQuote(receiptURL))
}

func windowsRDPRestoreSnippet(plan Plan) string {
	if !plan.ManagedChangeID.Valid || !json.Valid(plan.ManagedChangeBefore) || !json.Valid(plan.ManagedChangeApplied) {
		return "$drift=$false;$rdpStatus='not_applicable'"
	}
	before := base64.StdEncoding.EncodeToString(plan.ManagedChangeBefore)
	applied := base64.StdEncoding.EncodeToString(plan.ManagedChangeApplied)
	return fmt.Sprintf(`$drift=$false
$rdpStatus='restored'
$before=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String(%s))|ConvertFrom-Json
$applied=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String(%s))|ConvertFrom-Json
$terminal='HKLM:\SYSTEM\CurrentControlSet\Control\Terminal Server'
$rdpTcp='HKLM:\SYSTEM\CurrentControlSet\Control\Terminal Server\WinStations\RDP-Tcp'
$currentDeny=(Get-ItemProperty -LiteralPath $terminal -Name fDenyTSConnections).fDenyTSConnections
if($currentDeny -eq $applied.f_deny_ts_connections){Set-ItemProperty -LiteralPath $terminal -Name fDenyTSConnections -Type DWord -Value ([int]$before.f_deny_ts_connections)}else{$drift=$true}
$currentNla=(Get-ItemProperty -LiteralPath $rdpTcp -Name UserAuthentication).UserAuthentication
if($currentNla -eq $applied.user_authentication){Set-ItemProperty -LiteralPath $rdpTcp -Name UserAuthentication -Type DWord -Value ([int]$before.user_authentication)}else{$drift=$true}
foreach($ruleName in @('RemoteDesktop-UserMode-In-TCP','RemoteDesktop-UserMode-In-UDP')){
  $field=if($ruleName.EndsWith('TCP')){'firewall_tcp_enabled'}else{'firewall_udp_enabled'}
  $current=((Get-NetFirewallRule -Name $ruleName).Enabled.ToString() -eq 'True')
  if($current -eq [bool]$applied.$field){if([bool]$before.$field){Enable-NetFirewallRule -Name $ruleName|Out-Null}else{Disable-NetFirewallRule -Name $ruleName|Out-Null}}else{$drift=$true}
}
$currentService=(Get-Service -Name TermService).Status -eq 'Running'
if($currentService -eq [bool]$applied.term_service_running){if(-not [bool]$before.term_service_running){Stop-Service -Name TermService -Force}}else{$drift=$true}
Set-Content -LiteralPath (Join-Path $journal 'windows-rdp-restore.json') -Value (@{drift=$drift}|ConvertTo-Json -Compress) -Encoding UTF8
if($drift){$rdpStatus='drifted'}
`, psQuote(before), psQuote(applied))
}
