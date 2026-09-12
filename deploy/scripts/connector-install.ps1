param(
  [Parameter(Mandatory = $true)][string]$Manifest,
  [Parameter(Mandatory = $true)][string]$KeyId,
  [Parameter(Mandatory = $true)][string]$PublicKey,
  [Parameter(Mandatory = $true)][Guid]$ConnectorId,
  [Parameter(Mandatory = $true)][string]$TokenFile,
  [Parameter(Mandatory = $true)][string]$Server,
  [Parameter(Mandatory = $true)][ValidateSet("host")][string]$Role,
  [Parameter(Mandatory = $true)][string]$CAFile,
  [string]$EnrollDialAddress = "",
  [string]$GatewayDialAddress = "",
  [string]$HttpsDialAddress = ""
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
Set-StrictMode -Version Latest

if ($HttpsDialAddress -and $HttpsDialAddress -notmatch '^(\[[0-9A-Fa-f:.%]+\]|[A-Za-z0-9._-]+):[0-9]{1,5}$') {
  throw "HTTPS dial address must be host:port"
}

function Invoke-ArgusDownload([string]$Uri, [string]$Path) {
  $arguments = @("-fsSL", "--proto", "=https", "--tlsv1.2", "--ssl-revoke-best-effort", "--cacert", $CAFile)
  if ($HttpsDialAddress) { $arguments += @("--connect-to", "::$HttpsDialAddress") }
  $arguments += @("--output", $Path, $Uri)
  & $curl.Source @arguments
  if ($LASTEXITCODE -ne 0) { throw "Argus HTTPS download failed" }
}

foreach ($required in @($TokenFile, $CAFile)) {
  if (-not (Test-Path -LiteralPath $required -PathType Leaf)) { throw "Required installation input is missing" }
}
if (-not ([Uri]$Manifest).Scheme.Equals("https") -or -not ([Uri]$Server).Scheme.Equals("https")) {
  throw "Argus installation endpoints must use HTTPS"
}
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  throw "Windows Connector installation requires an elevated PowerShell"
}
$curl = Get-Command "curl.exe" -ErrorAction Stop
$temporary = Join-Path ([IO.Path]::GetTempPath()) ("argus-connector-install-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $temporary | Out-Null
try {
  $manifestPath = Join-Path $temporary "manifest.json"
  Invoke-ArgusDownload $Manifest $manifestPath
  $release = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
  if ($release.schema_version -ne "argus.connector_release/v3" -or $release.signing_key_id -ne $KeyId -or $release.signing_public_key -ne $PublicKey) {
    throw "Connector release manifest is invalid"
  }
  $artifact = @($release.artifacts | Where-Object { $_.platform -eq "windows_amd64" })
  if ($artifact.Count -ne 1 -or $artifact[0].signing_key_id -ne $KeyId) { throw "Windows Connector artifact is unavailable" }
  $binary = Join-Path $temporary "argus-connector.exe"
  Invoke-ArgusDownload $artifact[0].uri $binary
  $actualHash = (Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($actualHash -ne ([string]$artifact[0].sha256).ToLowerInvariant()) { throw "Connector artifact SHA-256 mismatch" }
  & $binary verify-artifact --file $binary --sha256 $artifact[0].sha256 --signature $artifact[0].signature --public-key $PublicKey --byte-size $artifact[0].byte_size
  if ($LASTEXITCODE -ne 0) { throw "Connector artifact Ed25519 signature verification failed" }

  $stateRoot = Join-Path $env:ProgramData "Argus\Connector"
  New-Item -ItemType Directory -Force -Path $stateRoot | Out-Null
  & icacls.exe $stateRoot /inheritance:r /grant:r "SYSTEM:(OI)(CI)(F)" "Administrators:(OI)(CI)(F)" | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "Connector state ACL configuration failed" }
  $currentId = ""
  $identityPath = Join-Path $stateRoot "identity.json"
  if (Test-Path -LiteralPath $identityPath) {
    try { $currentId = [string]((Get-Content -LiteralPath $identityPath -Raw | ConvertFrom-Json).connector_id) } catch { throw "Existing Connector identity metadata is invalid" }
  }
  $takeover = $currentId -and $currentId -ne $ConnectorId.ToString()
  $token = [IO.File]::ReadAllText((Resolve-Path -LiteralPath $TokenFile)).Trim()
  if ([String]::IsNullOrWhiteSpace($token)) { throw "Connector enrollment token is empty" }
  [Environment]::SetEnvironmentVariable("ARGUS_CONNECTOR_ENROLL_ADDRESS", $(if ($EnrollDialAddress) { $EnrollDialAddress } else { $null }), "Process")
  & $binary enroll --connector-id $ConnectorId --token-file $TokenFile --server $Server --ca-file $CAFile --role host --data-dir $stateRoot
  if ($LASTEXITCODE -ne 0) { throw "Connector enrollment failed" }

  if ($takeover) {
    Write-Host "argus connector-install: replacing existing Connector $currentId with $ConnectorId"
    Stop-Service -Name "ArgusCollector" -Force -ErrorAction SilentlyContinue
    & sc.exe delete ArgusCollector | Out-Null
    Remove-Item -LiteralPath (Join-Path $env:ProgramFiles "Argus\Collector"),(Join-Path $env:ProgramData "Argus\Collector") -Recurse -Force -ErrorAction SilentlyContinue
  }
  foreach ($serviceName in @("ArgusConnectorPrivileged", "ArgusConnector")) {
    $existingService = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if ($null -ne $existingService -and $existingService.Status -ne "Stopped") {
      Stop-Service -Name $serviceName -Force
      $existingService.WaitForStatus("Stopped", [TimeSpan]::FromSeconds(30))
    }
  }

  $installRoot = Join-Path $env:ProgramFiles "Argus\Connector"
  New-Item -ItemType Directory -Force -Path $installRoot | Out-Null
  & icacls.exe $installRoot /inheritance:r /grant:r "SYSTEM:(OI)(CI)(F)" "Administrators:(OI)(CI)(F)" | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "Connector program ACL configuration failed" }
  $service = Get-Service -Name "ArgusConnector" -ErrorAction SilentlyContinue
  $executable = Join-Path $installRoot "argus-connector.exe"
  $nextExecutable = Join-Path $installRoot "argus-connector.next.exe"
  Copy-Item -Force -LiteralPath $binary -Destination $nextExecutable
  Move-Item -Force -LiteralPath $nextExecutable -Destination $executable
  Copy-Item -Force -LiteralPath $CAFile -Destination (Join-Path $stateRoot "server-ca.pem")
  $signingKeys = @{ $KeyId = $PublicKey } | ConvertTo-Json -Compress
  [IO.File]::WriteAllText((Join-Path $stateRoot "otelcol-signing-keys.json"), $signingKeys, (New-Object Text.UTF8Encoding($false)))
  [Environment]::SetEnvironmentVariable("ARGUS_CONNECTOR_DIAL_ADDRESS", $(if ($GatewayDialAddress) { $GatewayDialAddress } else { $null }), "Machine")
  [Environment]::SetEnvironmentVariable("ARGUS_ARTIFACT_DIAL_ADDRESS", $(if ($HttpsDialAddress) { $HttpsDialAddress } else { $null }), "Machine")
  [Environment]::SetEnvironmentVariable("ARGUS_OTELCOL_SIGNING_PUBLIC_KEYS_FILE", (Join-Path $stateRoot "otelcol-signing-keys.json"), "Machine")
  [Environment]::SetEnvironmentVariable("ARGUS_CONNECTOR_ENROLL_ADDRESS", $null, "Process")
  $token = $null
  $serviceCommand = "`"$executable`" run --data-dir `"$stateRoot`""
  if ($null -eq $service) {
    sc.exe create ArgusConnector binPath= $serviceCommand start= auto | Out-Null
  } else {
    if ($service.Status -ne "Stopped") { Stop-Service -Name "ArgusConnector" -Force }
    sc.exe config ArgusConnector binPath= $serviceCommand start= auto | Out-Null
  }
  if ($LASTEXITCODE -ne 0) { throw "Windows Connector service configuration failed" }
  sc.exe failure ArgusConnector reset= 86400 actions= restart/5000/restart/15000/restart/60000 | Out-Null
  Start-Service -Name "ArgusConnector"
  (Get-Service -Name "ArgusConnector").WaitForStatus("Running", [TimeSpan]::FromSeconds(30))
} finally {
  Remove-Item -LiteralPath $temporary -Recurse -Force -ErrorAction SilentlyContinue
}
