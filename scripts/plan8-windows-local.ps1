#Requires -Version 5.1
param(
  [ValidateSet('Preflight','Prepare','Restore')][string]$Mode = 'Preflight',
  [ValidatePattern('^[a-z0-9][a-z0-9-]{0,39}$')][string]$RunId = 'plan8-windows11',
  [ValidateRange(10240,65535)][int]$SitePort = 32180
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$plan8Repo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$plan8Root = [IO.Path]::GetFullPath((Join-Path $plan8Repo ('artifacts\windows-local\' + $RunId)))
if (-not $plan8Root.StartsWith($plan8Repo + [IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Run directory escaped the workspace' }
$plan8StatePath = Join-Path $plan8Root 'state.json'
$plan8ReportPath = Join-Path $plan8Root 'preflight.json'
$plan8Identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$plan8Admin = ([Security.Principal.WindowsPrincipal]$plan8Identity).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
$plan8OS = Get-CimInstance Win32_OperatingSystem
$plan8Client = [int]$plan8OS.ProductType -eq 1 -and [int]$plan8OS.BuildNumber -ge 22000
$plan8Services = @(Get-CimInstance Win32_Service | Where-Object { $_.Name -match '^Argus' } | Select-Object Name,State,StartMode,PathName)
$plan8Site = 'ArgusPlan8-' + $RunId
$plan8Pool = $plan8Site
$plan8Source = 'ArgusPlan8-' + $RunId

function Save-Plan8Json([string]$Path,$Value) {
  $absolute = [IO.Path]::GetFullPath($Path)
  if (-not $absolute.StartsWith($plan8Root + [IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase)) { throw 'Evidence path escaped the run directory' }
  [IO.Directory]::CreateDirectory($plan8Root) | Out-Null
  [IO.File]::WriteAllText($absolute,($Value | ConvertTo-Json -Depth 12),[Text.UTF8Encoding]::new($false))
}

$plan8Features = @()
if ($plan8Admin) { $plan8Features = @(Get-WindowsOptionalFeature -Online | Select-Object FeatureName,@{n='State';e={$_.State.ToString()}}) }
$plan8Report = [ordered]@{
  schema_version='argus.plan8_windows_local_preflight/v1'; observed_at=[DateTime]::UtcNow.ToString('o')
  run_id=$RunId; computer=$env:COMPUTERNAME; product_type=[int]$plan8OS.ProductType; build=[int]$plan8OS.BuildNumber
  caption=$plan8OS.Caption; windows11_client=$plan8Client; administrator=$plan8Admin
  existing_argus_services=$plan8Services; feature_states=$plan8Features
  ready_for_prepare=($plan8Client -and $plan8Admin -and $plan8Services.Count -eq 0)
  acceptance='not_run'; server_physical_acceptance='not_verified'
}
Save-Plan8Json $plan8ReportPath $plan8Report
if ($Mode -eq 'Preflight') {
  $plan8Report | ConvertTo-Json -Depth 4
  if (-not $plan8Report.ready_for_prepare) { exit 2 }
  exit 0
}
if (-not $plan8Admin) { throw 'Administrator Windows PowerShell is required for IIS/event source setup or restoration. No machine setup was changed.' }
if (-not $plan8Client) { throw 'This local driver targets Windows 11 clients only; it does not certify Windows Server.' }
if ($Mode -eq 'Prepare' -and $plan8Services.Count -gt 0) { throw 'Existing Argus services require a separately reviewed lifecycle test; preparation will not overwrite them.' }

if ($Mode -eq 'Restore') {
  if (-not (Test-Path -LiteralPath $plan8StatePath -PathType Leaf)) { throw 'Owned preparation state is missing' }
  $state = Get-Content -LiteralPath $plan8StatePath -Raw | ConvertFrom-Json
  if ($state.schema_version -ne 'argus.plan8_windows_local_setup/v1' -or $state.run_id -ne $RunId -or $state.computer -ne $env:COMPUTERNAME -or $state.site_name -ne $plan8Site) { throw 'Preparation ownership does not match this run and machine' }
  Import-Module WebAdministration
  $site = Get-Website -Name $plan8Site -ErrorAction SilentlyContinue
  if ($null -ne $site -and $state.site_created) {
    if ([IO.Path]::GetFullPath($site.physicalPath) -ne [IO.Path]::GetFullPath($state.site_path)) { throw 'Owned site physical path changed; restoration stopped' }
    $bindings = @(Get-WebBinding -Name $plan8Site)
    if ($bindings.Count -ne 1 -or $bindings[0].bindingInformation -ne ('127.0.0.1:' + $state.site_port + ':')) { throw 'Owned site bindings changed; restoration stopped' }
    Remove-Website -Name $plan8Site
  }
  $poolPath = 'IIS:\AppPools\' + $plan8Pool
  if ($state.pool_created -and (Test-Path -LiteralPath $poolPath)) {
    if (@(Get-Website | Where-Object { $_.applicationPool -eq $plan8Pool }).Count -gt 0) { throw 'Another website now uses the run application pool' }
    Remove-WebAppPool -Name $plan8Pool
  }
  if ($state.event_source_created -and [Diagnostics.EventLog]::SourceExists($plan8Source)) {
    if ([Diagnostics.EventLog]::LogNameFromSourceName($plan8Source,'.') -ne 'Application') { throw 'Event source ownership changed' }
    [Diagnostics.EventLog]::DeleteEventSource($plan8Source)
  }
  $state.status = 'owned_resources_removed_features_retained'
  $state | Add-Member -NotePropertyName restored_at -NotePropertyValue ([DateTime]::UtcNow.ToString('o')) -Force
  Save-Plan8Json $plan8StatePath $state
  Write-Output 'Owned website, pool and event source removed. Feature changes remain recorded; inspect dependencies before disabling shared IIS/WAS features.'
  exit 0
}

if (Test-Path -LiteralPath $plan8StatePath) { throw 'Preparation state already exists; inspect or restore this exact run before preparing again' }
$sitePath = Join-Path $plan8Root 'site'
$state = [ordered]@{
  schema_version='argus.plan8_windows_local_setup/v1'; run_id=$RunId; computer=$env:COMPUTERNAME
  prepared_by_sid=$plan8Identity.User.Value; started_at=[DateTime]::UtcNow.ToString('o'); status='preparing'
  feature_baseline=$plan8Features; enabled_features=@(); restart_needed=$false
  site_name=$plan8Site; pool_name=$plan8Pool; site_path=$sitePath; site_port=$SitePort
  event_source=$plan8Source; telemetry_acceptance='not_run'; formal_installation='not_run'
  site_created=$false; pool_created=$false; event_source_created=$false
  site_baseline=@(); stopped_auto_default_site=$false
}
if ([Diagnostics.EventLog]::SourceExists($plan8Source)) { throw 'The run event source already exists; preparation will not claim it' }
if (Get-Service W3SVC -ErrorAction SilentlyContinue) {
  Import-Module WebAdministration
  $state.site_baseline = @(Get-Website | ForEach-Object Name)
  if ((Test-Path -LiteralPath ('IIS:\Sites\' + $plan8Site)) -or (Test-Path -LiteralPath ('IIS:\AppPools\' + $plan8Pool))) { throw 'The run site or pool already exists; preparation will not claim it' }
}
Save-Plan8Json $plan8StatePath $state
$required = @('IIS-WebServerRole','IIS-WebServer','IIS-CommonHttpFeatures','IIS-StaticContent','IIS-DefaultDocument','IIS-HttpErrors','IIS-HttpLogging','IIS-Security','IIS-RequestFiltering','IIS-ManagementScriptingTools')
try {
  $missing = @($plan8Features | Where-Object { $_.FeatureName -in $required -and $_.State -ne 'Enabled' } | ForEach-Object FeatureName)
  if ($missing.Count -gt 0) {
    $result = Enable-WindowsOptionalFeature -Online -FeatureName $missing -All -NoRestart
    $state.restart_needed = [bool]$result.RestartNeeded
  }
  $after = @(Get-WindowsOptionalFeature -Online | Where-Object State -eq 'Enabled' | ForEach-Object FeatureName)
  $before = @($plan8Features | Where-Object State -eq 'Enabled' | ForEach-Object FeatureName)
  $state.enabled_features = @($after | Where-Object { $_ -notin $before })
  Save-Plan8Json $plan8StatePath $state
  if ($state.restart_needed) { throw 'Windows reports a required restart. Restore/review the saved state before further setup.' }
  Import-Module WebAdministration
  if ('Default Web Site' -notin $state.site_baseline) {
    $autoSite = Get-Website -Name 'Default Web Site' -ErrorAction SilentlyContinue
    if ($null -ne $autoSite) { Stop-Website -Name 'Default Web Site'; $state.stopped_auto_default_site=$true; Save-Plan8Json $plan8StatePath $state }
  }
  if ((Test-Path -LiteralPath ('IIS:\Sites\' + $plan8Site)) -or (Test-Path -LiteralPath ('IIS:\AppPools\' + $plan8Pool)) -or [Diagnostics.EventLog]::SourceExists($plan8Source)) { throw 'Run site, pool or event source unexpectedly exists; ownership cannot be assumed' }
  [IO.Directory]::CreateDirectory($sitePath) | Out-Null
  [IO.File]::WriteAllText((Join-Path $sitePath 'index.html'),('Argus Plan8 local IIS ' + $RunId),[Text.UTF8Encoding]::new($false))
  New-WebAppPool -Name $plan8Pool | Out-Null
  $state.pool_created=$true; Save-Plan8Json $plan8StatePath $state
  New-Website -Name $plan8Site -Port $SitePort -IPAddress '127.0.0.1' -PhysicalPath $sitePath -ApplicationPool $plan8Pool | Out-Null
  $state.site_created=$true; Save-Plan8Json $plan8StatePath $state
  [Diagnostics.EventLog]::CreateEventSource($plan8Source,'Application')
  $state.event_source_created=$true; Save-Plan8Json $plan8StatePath $state
  Write-EventLog -LogName Application -Source $plan8Source -EventId 19100 -EntryType Information -Message ('Argus Plan8 local event ' + $RunId)
  $response = Invoke-WebRequest -UseBasicParsing -Uri ('http://127.0.0.1:' + $SitePort + '/')
  if ($response.StatusCode -ne 200 -or $response.Content -notlike ('*' + $RunId + '*')) { throw 'Owned loopback IIS site did not return its actual run marker' }
  $state.status='prepared'
  $state | Add-Member -NotePropertyName prepared_at -NotePropertyValue ([DateTime]::UtcNow.ToString('o'))
  Save-Plan8Json $plan8StatePath $state
  Write-Output 'Prepared a loopback-only IIS site and owned Application event source. Formal enrollment, service installation and telemetry remain unverified.'
} catch {
  $state.status='preparation_failed'
  try {
    $observedFeatures = @(Get-WindowsOptionalFeature -Online | Where-Object State -eq 'Enabled' | ForEach-Object FeatureName)
    $originalFeatures = @($plan8Features | Where-Object State -eq 'Enabled' | ForEach-Object FeatureName)
    $state.enabled_features = @($observedFeatures | Where-Object { $_ -notin $originalFeatures })
  } catch { }
  Save-Plan8Json $plan8StatePath $state
  throw
}
