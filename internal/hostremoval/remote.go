package hostremoval

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func DecodeOperationPlan(operation db.HostRemovalOperation) (Plan, error) {
	var plan Plan
	if json.Unmarshal(operation.Plan, &plan) != nil || plan.SchemaVersion != "argus.host_removal/v1" || !plan.OperationID.Valid || plan.OperationID.UUID != operation.ID ||
		plan.HostID != operation.HostID || plan.ConnectorID != operation.ConnectorID || plan.RemovalGeneration != operation.RemovalGeneration {
		return Plan{}, ErrIdentityChanged
	}
	// Hash all persisted fields, including any unknown field, before using the
	// typed plan. Remarshalling the struct would discard unrecognized content.
	canonical, err := resource.CanonicalJSON(operation.Plan)
	if err != nil {
		return Plan{}, err
	}
	digest := sha256.Sum256(canonical)
	if !bytes.Equal(digest[:], operation.PlanHash) {
		return Plan{}, errors.New("host removal plan hash mismatch")
	}
	return plan, nil
}

func ParseSSHCleanupOutput(output string, operationID, connectorID uuid.UUID, generation int64) (CleanupEvidence, []byte, []byte, error) {
	const marker = "ARGUS_CLEANUP_EVIDENCE_BASE64="
	index := strings.LastIndex(output, marker)
	if index < 0 {
		return CleanupEvidence{}, nil, nil, errors.New("SSH cleanup evidence is missing")
	}
	encoded := strings.TrimSpace(strings.SplitN(output[index+len(marker):], "\n", 2)[0])
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return CleanupEvidence{}, nil, nil, errors.New("SSH cleanup evidence encoding is invalid")
	}
	evidence, canonical, err := ParseCleanupEvidence(raw, operationID, connectorID, generation)
	if err != nil {
		return CleanupEvidence{}, nil, nil, err
	}
	return evidence, canonical, CleanupEvidenceDigest(canonical), nil
}

// BuildSSHScript returns an idempotent local cleanup program. The caller owns
// the independent SSH process and reports the verified result only after this
// program exits successfully.
func BuildSSHScript(plan Plan) (script string, windows bool) {
	if strings.HasPrefix(plan.TargetPlatform, "windows_") {
		restore := windowsRDPRestoreSnippet(plan)
		return fmt.Sprintf(`$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$principal=New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if(-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)){throw 'Administrator privileges are required'}
%s
$journal=Join-Path $env:ProgramData %s
New-Item -ItemType Directory -Force -Path $journal|Out-Null
& icacls.exe $journal /inheritance:r /grant:r 'SYSTEM:(OI)(CI)(F)' 'Administrators:(OI)(CI)(F)'|Out-Null
$helper=Join-Path $journal 'argus-uninstaller.exe'
$evidenceFile=Join-Path $journal 'evidence.json'
if(Test-Path -LiteralPath $evidenceFile){
  $evidenceRaw=[IO.File]::ReadAllText($evidenceFile).Trim()
}else{
  if(-not(Test-Path -LiteralPath $helper)){Copy-Item -LiteralPath (Join-Path $env:ProgramFiles 'Argus\Connector\argus-connector.exe') -Destination $helper}
  %s
  $evidenceRaw=(& $helper uninstall-local --operation-id %s --connector-id %s --removal-generation %d --journal-dir $journal --relay-https-port %d --relay-gateway-port %d --rdp-config-status $rdpStatus|Out-String).Trim()
  if($LASTEXITCODE-ne 0){throw 'Argus local cleanup postconditions failed'}
  [IO.File]::WriteAllText($evidenceFile,$evidenceRaw,(New-Object Text.UTF8Encoding($false)))
}
$encoded=[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($evidenceRaw))
Write-Output ('ARGUS_CLEANUP_EVIDENCE_BASE64='+$encoded)
Remove-Item -LiteralPath $helper -Force -ErrorAction SilentlyContinue
`, windowsRemovalIdentityGuard(plan), psQuote("Argus\\Uninstall\\"+plan.OperationID.UUID.String()), restore, psQuote(plan.OperationID.UUID.String()), psQuote(plan.ConnectorID.String()), plan.RemovalGeneration, plan.RelayHTTPSPort, plan.RelayGatewayPort), true
	}
	return fmt.Sprintf(`#!/bin/sh
set -eu
umask 077
[ "$(id -u)" -eq 0 ] || { echo 'root privileges are required' >&2; exit 1; }
%s
JOURNAL=%s
mkdir -p "$JOURNAL"
chmod 0700 /var/lib/argus-uninstall "$JOURNAL"
HELPER="$JOURNAL/argus-uninstaller"
if [ -s "$JOURNAL/evidence.json" ]; then
  EVIDENCE=$(cat "$JOURNAL/evidence.json")
else
  [ -x "$HELPER" ] || { cp /usr/local/bin/argus-connector "$HELPER"; chmod 0700 "$HELPER"; }
  EVIDENCE=$("$HELPER" uninstall-local --operation-id %s --connector-id %s --removal-generation %d --journal-dir "$JOURNAL" --relay-https-port %d --relay-gateway-port %d --rdp-config-status not_applicable)
  printf '%%s' "$EVIDENCE" > "$JOURNAL/evidence.json"
fi
printf 'ARGUS_CLEANUP_EVIDENCE_BASE64=%%s\n' "$(printf '%%s' "$EVIDENCE" | base64 | tr -d '\n')"
rm -f "$HELPER"
`, linuxRemovalIdentityGuard(plan), shQuote("/var/lib/argus-uninstall/"+plan.OperationID.UUID.String()), shQuote(plan.OperationID.UUID.String()), shQuote(plan.ConnectorID.String()), plan.RemovalGeneration, plan.RelayHTTPSPort, plan.RelayGatewayPort), false
}

// Check the actual machine immediately before any mutation, including when a
// previous run left a cached receipt. SSH host keys alone do not bind an Argus
// installation: another cluster may have taken over this very same machine.
func linuxRemovalIdentityGuard(plan Plan) string {
	return `if [ -f /var/lib/argus-connector/identity.json ]; then
  CURRENT_CONNECTOR=$(sed -n 's/.*"connector_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' /var/lib/argus-connector/identity.json | head -n 1)
  [ "$CURRENT_CONNECTOR" = ` + shQuote(plan.ConnectorID.String()) + ` ] || { printf '%s\n' 'TARGET_IDENTITY_CHANGED' >&2; exit 1; }
fi`
}

func windowsRemovalIdentityGuard(plan Plan) string {
	return `$identityFile=Join-Path $env:ProgramData 'Argus\Connector\identity.json'
if(Test-Path -LiteralPath $identityFile){
  $currentIdentity=Get-Content -LiteralPath $identityFile -Raw|ConvertFrom-Json
  if($currentIdentity.connector_id -ne ` + psQuote(plan.ConnectorID.String()) + `){throw 'TARGET_IDENTITY_CHANGED'}
}`
}
