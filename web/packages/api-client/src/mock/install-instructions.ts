import type { ActionOneTimeResult } from "../generated/contracts";

type MockConnectorPlatform = "linux_amd64" | "linux_arm64" | "windows_amd64";

export function mockInstallInstructionSets(
  token: string,
  expiresAt: string,
  platform: MockConnectorPlatform = "linux_amd64",
): ActionOneTimeResult["instruction_sets"] {
  const windows = platform === "windows_amd64";
  const bootstrapSha256 = "c".repeat(64);
  const command = windows
    ? `$ErrorActionPreference='Stop'; $d=Join-Path ([IO.Path]::GetTempPath()) ('argus-bootstrap-'+[Guid]::NewGuid().ToString('N')); New-Item -ItemType Directory -Path $d | Out-Null; try { $s=Join-Path $d 'bootstrap.ps1'; & curl.exe -fsS --proto '=https' --tlsv1.2 --insecure --header 'X-Argus-Enrollment-Token: ${token}' --output $s 'https://argus.example.com/api/v1/connectors/bootstrap-script?scope=windows-system'; if ($LASTEXITCODE -ne 0) { throw 'Argus bootstrap download failed' }; if ((Get-FileHash -LiteralPath $s -Algorithm SHA256).Hash.ToLowerInvariant() -ne '${bootstrapSha256}') { throw 'Argus bootstrap SHA-256 mismatch' }; & powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File $s } finally { Remove-Item -LiteralPath $d -Recurse -Force -ErrorAction SilentlyContinue }` // ARGUS_INSECURE_FIRST_FETCH_ONLY
    : `(set -eu; umask 077; ARGUS_BOOTSTRAP=$(mktemp "\${TMPDIR:-/tmp}/argus-bootstrap.XXXXXX"); trap 'rm -f "$ARGUS_BOOTSTRAP"' EXIT HUP INT TERM; curl -fsS --proto '=https' --tlsv1.2 --insecure --header 'X-Argus-Enrollment-Token: ${token}' --output "$ARGUS_BOOTSTRAP" 'https://argus.example.com/api/v1/connectors/bootstrap-script?scope=linux-system'; printf '%s  %s\\n' '${bootstrapSha256}' "$ARGUS_BOOTSTRAP" | sha256sum -c - >/dev/null; chmod 0700 "$ARGUS_BOOTSTRAP"; sh "$ARGUS_BOOTSTRAP")`; // ARGUS_INSECURE_FIRST_FETCH_ONLY
  return [
    {
      platform,
      shell: windows ? ("powershell" as const) : ("posix_sh" as const),
      privilege: "system" as const,
      command,
      bootstrap_tls_mode: "insecure-first-fetch" as const,
      release_version: "0.1.0-test",
      expires_at: expiresAt,
      trust_bundle_epoch: 1,
      trust_bundle_sha256: "b".repeat(64),
      bootstrap_sha256: bootstrapSha256,
      installer_sha256: "a".repeat(64),
      capability_warnings: [],
    },
  ];
}
