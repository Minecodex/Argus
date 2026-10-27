param(
    [ValidateSet('Prepare', 'PrepareControlMetrics', 'RestoreControlMetrics', 'Test', 'TestControlMetrics', 'Cleanup')][string]$Mode = 'Test',
    [ValidatePattern('^plan8-drain-[a-z0-9-]{1,40}$')][string]$RunId = 'plan8-drain-20261019',
    [string]$EvidenceRunId = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$p8Root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$p8Artifacts = Join-Path $p8Root 'artifacts'
$p8RunDir = Join-Path $p8Artifacts $RunId
$p8OwnerPath = Join-Path $p8RunDir 'owner.json'
$p8Kind = Join-Path $p8Artifacts 'tools/plan8-kind-v0.33.0.exe'
$p8KindHash = '4b22adaa135368c5a465d56bbd8e520cbea87272a06ca00b6078e7b81515c9fc'
$p8Utf8 = New-Object System.Text.UTF8Encoding($false)

function Invoke-P8Native([string]$P8Program, [string[]]$P8Arguments) {
    $p8PreviousPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = 'Continue'
        & $P8Program @P8Arguments 2>&1 | ForEach-Object { $_.ToString() }
        $p8NativeExit = $LASTEXITCODE
    } finally { $ErrorActionPreference = $p8PreviousPreference }
    if ($p8NativeExit -ne 0) { throw "$P8Program failed with exit code $p8NativeExit" }
}

function Resolve-P8Artifact([string]$P8Path) {
    $p8Absolute = [IO.Path]::GetFullPath($P8Path)
    if (-not $p8Absolute.StartsWith($p8Artifacts + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing path outside this workspace's artifacts directory"
    }
    return $p8Absolute
}

function Get-P8Nodes {
    $p8Names = @(& docker ps -a --filter "label=io.x-k8s.kind.cluster=$RunId" --format '{{.Names}}')
    if ($LASTEXITCODE -ne 0) { throw 'Cannot inventory Docker containers' }
    return @($p8Names | Where-Object { $_ } | ForEach-Object {
        $p8Raw = & docker inspect $_
        if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect candidate node' }
        $p8Node = @($p8Raw | ConvertFrom-Json)[0]
        [pscustomobject]@{ name = $_; id = $p8Node.Id; image_id = $p8Node.Image; cluster = $p8Node.Config.Labels.'io.x-k8s.kind.cluster' }
    })
}

function Assert-P8Owner($P8Owner) {
    if ($P8Owner.schema_version -ne 'argus.plan8_kind_owner/v1' -or $P8Owner.run_id -ne $RunId -or $P8Owner.workspace -ne $p8Root) { throw 'Invalid cluster ownership record' }
    $p8Current = @(Get-P8Nodes)
    if ($p8Current.Count -ne 2 -or @($P8Owner.nodes).Count -ne 2) { throw 'Owned two-node cluster is incomplete; inspect it before continuing' }
    foreach ($p8Node in $p8Current) {
        $p8Saved = @($P8Owner.nodes | Where-Object { $_.name -eq $p8Node.name -and $_.id -eq $p8Node.id })
        if ($p8Node.cluster -ne $RunId -or $p8Saved.Count -ne 1 -or $p8Node.name -notin @("$RunId-control-plane", "$RunId-worker")) { throw 'Node ownership changed; refusing mutation' }
    }
    return (Resolve-P8Artifact $P8Owner.kubeconfig)
}

function Set-P8ControlMetrics([bool]$P8Restore) {
    $p8ControlNode = "$RunId-control-plane"
    $p8PatchDir = Join-Path $p8RunDir 'control-metrics'
    New-Item -ItemType Directory -Path $p8PatchDir -Force | Out-Null
    $p8PriorPath = Join-Path $p8PatchDir 'record.json'
    $p8PriorPlans = @()
    if (Test-Path -LiteralPath $p8PriorPath) {
        $p8Prior = [IO.File]::ReadAllText($p8PriorPath, $p8Utf8) | ConvertFrom-Json
        if ($p8Prior.run_id -ne $RunId -or $p8Prior.node -ne $p8ControlNode) { throw 'Metric preparation ownership changed' }
        $p8PriorPlans = @($p8Prior.plans)
    }
    if (-not $P8Restore) {
        Invoke-P8Native 'docker' @('exec', $p8ControlNode, 'mkdir', '-p', '/etc/kubernetes/argus-plan8-metrics')
        foreach ($p8Component in @('kube-scheduler', 'kube-controller-manager')) {
            $p8Base = "/etc/kubernetes/argus-plan8-metrics/$p8Component"
            $p8Extension = Join-Path $p8PatchDir "$p8Component.ext"
            [IO.File]::WriteAllText($p8Extension, "subjectAltName=DNS:$p8Component`nextendedKeyUsage=serverAuth`nkeyUsage=digitalSignature,keyEncipherment`nbasicConstraints=critical,CA:FALSE`n", $p8Utf8)
            Invoke-P8Native 'docker' @('cp', $p8Extension, "${p8ControlNode}:$p8Base.ext")
            Invoke-P8Native 'docker' @('exec', $p8ControlNode, 'openssl', 'req', '-new', '-newkey', 'rsa:2048', '-nodes', '-keyout', "$p8Base.key", '-out', "$p8Base.csr", '-subj', "/CN=$p8Component")
            Invoke-P8Native 'docker' @('exec', $p8ControlNode, 'openssl', 'x509', '-req', '-in', "$p8Base.csr", '-CA', '/etc/kubernetes/pki/ca.crt', '-CAkey', '/etc/kubernetes/pki/ca.key', '-CAcreateserial', '-out', "$p8Base.crt", '-days', '2', '-extfile', "$p8Base.ext")
        }
    }
    $p8Changes = @(
        @{ name = 'kube-scheduler'; before = '--bind-address=127.0.0.1'; after = '--bind-address=0.0.0.0' },
        @{ name = 'kube-controller-manager'; before = '--bind-address=127.0.0.1'; after = '--bind-address=0.0.0.0' },
        @{ name = 'etcd'; before = '--listen-metrics-urls=http://127.0.0.1:2381'; after = '--listen-metrics-urls=http://0.0.0.0:2381' }
    )
    $p8Plans = @()
    # Inspect all originals before changing any static Pod. Only this owned
    # Docker node is touched; credentials and TLS/authentication stay intact.
    foreach ($p8Change in $p8Changes) {
        $p8Name = $p8Change.name
        $p8Original = Join-Path $p8PatchDir "$p8Name.original.yaml"
        $p8Applied = Join-Path $p8PatchDir "$p8Name.applied.yaml"
        $p8Current = Join-Path $p8PatchDir "$p8Name.current.yaml"
        $p8Remote = "${p8ControlNode}:/etc/kubernetes/manifests/$p8Name.yaml"
        if (-not (Test-Path -LiteralPath $p8Original)) {
            if ($P8Restore) { throw 'No saved metric-listener baseline to restore' }
            Invoke-P8Native 'docker' @('cp', $p8Remote, $p8Original)
        }
        $p8Text = [IO.File]::ReadAllText($p8Original, $p8Utf8)
        if (($p8Text.Split([string[]]@($p8Change.before), [StringSplitOptions]::None).Count - 1) -ne 1) { throw 'Unexpected component configuration; refusing to rewrite it' }
        $p8Updated = $p8Text.Replace($p8Change.before, $p8Change.after)
        if ($p8Name -ne 'etcd') {
            $p8Updated = $p8Updated.Replace("    - $($p8Change.after)", "    - $($p8Change.after)`n    - --tls-cert-file=/argus-plan8-metrics/$p8Name.crt`n    - --tls-private-key-file=/argus-plan8-metrics/$p8Name.key")
            $p8Updated = $p8Updated.Replace("    volumeMounts:`n", "    volumeMounts:`n    - mountPath: /argus-plan8-metrics`n      name: argus-plan8-metrics`n      readOnly: true`n")
            $p8Updated = $p8Updated.Replace("  volumes:`n", "  volumes:`n  - hostPath:`n      path: /etc/kubernetes/argus-plan8-metrics`n      type: Directory`n    name: argus-plan8-metrics`n")
        }
        [IO.File]::WriteAllText($p8Applied, $p8Updated, $p8Utf8)
        Invoke-P8Native 'docker' @('cp', $p8Remote, $p8Current)
        $p8OriginalHash = (Get-FileHash -LiteralPath $p8Original -Algorithm SHA256).Hash
        $p8AppliedHash = (Get-FileHash -LiteralPath $p8Applied -Algorithm SHA256).Hash
        $p8CurrentHash = (Get-FileHash -LiteralPath $p8Current -Algorithm SHA256).Hash
        $p8AllowedHashes = @($p8OriginalHash, $p8AppliedHash)
        foreach ($p8PriorPlan in $p8PriorPlans) { if ($p8PriorPlan.name -eq $p8Name -and $p8PriorPlan.original_sha256 -eq $p8OriginalHash) { $p8AllowedHashes += $p8PriorPlan.applied_sha256 } }
        if ($p8CurrentHash -notin $p8AllowedHashes) { throw 'Component changed outside this test; refusing to overwrite it' }
        $p8Plans += [pscustomobject]@{ name = $p8Name; original = $p8Original; applied = $p8Applied; original_sha256 = $p8OriginalHash; applied_sha256 = $p8AppliedHash; current_sha256 = $p8CurrentHash }
    }
    $p8PatchRecord = @{ run_id = $RunId; mode = $Mode; node = $p8ControlNode; plans = $p8Plans; status = 'prepared_not_applied'; recorded_at = [DateTime]::UtcNow.ToString('o') }
    $p8RecordPath = Join-Path $p8PatchDir 'record.json'
    [IO.File]::WriteAllText($p8RecordPath, ($p8PatchRecord | ConvertTo-Json -Depth 8), $p8Utf8)
    $null = Assert-P8Owner $p8Owner
    Invoke-P8Native 'docker' @('exec', $p8ControlNode, 'mkdir', '-p', '/etc/kubernetes/argus-plan8-staging')
    foreach ($p8Plan in $p8Plans) {
        $p8Desired = $p8Plan.applied
        $p8DesiredHash = $p8Plan.applied_sha256
        if ($P8Restore) { $p8Desired = $p8Plan.original; $p8DesiredHash = $p8Plan.original_sha256 }
        if ($p8Plan.current_sha256 -eq $p8DesiredHash) { continue }
        $p8Staged = "/etc/kubernetes/argus-plan8-staging/$($p8Plan.name).yaml"
        Invoke-P8Native 'docker' @('cp', $p8Desired, "${p8ControlNode}:$p8Staged")
        Invoke-P8Native 'docker' @('exec', $p8ControlNode, 'mv', $p8Staged, "/etc/kubernetes/manifests/$($p8Plan.name).yaml")
    }
    $p8Deadline = [DateTime]::UtcNow.AddSeconds(120)
    $p8Ready = $false
    while ([DateTime]::UtcNow -lt $p8Deadline) {
        $p8Ready = $true
        foreach ($p8Change in $p8Changes) {
            try {
                $p8PodRaw = & kubectl --kubeconfig $p8Kubeconfig --request-timeout=3s get pod "$($p8Change.name)-$p8ControlNode" -n kube-system -o json 2>$null
                if ($LASTEXITCODE -ne 0) { $p8Ready = $false; break }
                $p8Pod = $p8PodRaw | ConvertFrom-Json
                $p8Expected = $p8Change.after
                if ($P8Restore) { $p8Expected = $p8Change.before }
                $p8Command = @($p8Pod.spec.containers[0].command)
                $p8ConditionProperty = $p8Pod.status.PSObject.Properties['conditions']
                if ($p8Command -notcontains $p8Expected -or -not $p8ConditionProperty -or @($p8ConditionProperty.Value | Where-Object { $_.type -eq 'Ready' -and $_.status -eq 'True' }).Count -ne 1) { $p8Ready = $false; break }
                if (-not $P8Restore -and $p8Change.name -ne 'etcd' -and $p8Command -notcontains "--tls-cert-file=/argus-plan8-metrics/$($p8Change.name).crt") { $p8Ready = $false; break }
            } catch { $p8Ready = $false; break }
        }
        if ($p8Ready) { break }
        Start-Sleep -Milliseconds 500
    }
    if (-not $p8Ready) { throw 'Owned control components did not become Ready with the requested listeners; baseline and partial state are preserved' }
    $p8PatchRecord.status = 'applied_target_data_not_yet_verified'
    if ($P8Restore) { $p8PatchRecord.status = 'original_listeners_restored' }
    $p8PatchRecord.ready_observed_at = [DateTime]::UtcNow.ToString('o')
    [IO.File]::WriteAllText($p8RecordPath, ($p8PatchRecord | ConvertTo-Json -Depth 8), $p8Utf8)
}

Push-Location $p8Root
try {
    New-Item -ItemType Directory -Path $p8RunDir -Force | Out-Null
    if (-not (Test-Path -LiteralPath $p8Kind)) {
        New-Item -ItemType Directory -Path (Split-Path $p8Kind) -Force | Out-Null
        Invoke-WebRequest -Uri 'https://kind.sigs.k8s.io/dl/v0.33.0/kind-windows-amd64' -OutFile $p8Kind -TimeoutSec 60
    }
    if ((Get-FileHash -LiteralPath $p8Kind -Algorithm SHA256).Hash.ToLowerInvariant() -ne $p8KindHash) { throw 'Kind binary checksum mismatch' }

    if ($Mode -eq 'Prepare' -and -not (Test-Path -LiteralPath $p8OwnerPath)) {
        if (@(Get-P8Nodes).Count -ne 0) { throw 'Cluster exists without this driver ownership record; refusing to adopt it automatically' }
        $p8Config = Join-Path $p8RunDir 'kind.yaml'
        $p8Kubeconfig = Join-Path $p8RunDir 'kubeconfig'
        [IO.File]::WriteAllText($p8Config, "kind: Cluster`napiVersion: kind.x-k8s.io/v1alpha4`nnetworking:`n  apiServerAddress: '127.0.0.1'`nnodes:`n  - role: control-plane`n  - role: worker`n", $p8Utf8)
        [IO.File]::WriteAllText((Join-Path $p8RunDir 'creation-intent.json'), (@{ run_id = $RunId; workspace = $p8Root; created_at = [DateTime]::UtcNow.ToString('o') } | ConvertTo-Json), $p8Utf8)
        Invoke-P8Native $p8Kind @('create', 'cluster', '--name', $RunId, '--image', 'kindest/node:v1.36.1', '--config', $p8Config, '--kubeconfig', $p8Kubeconfig, '--wait', '120s', '--retain')
        $p8Owner = @{ schema_version = 'argus.plan8_kind_owner/v1'; run_id = $RunId; workspace = $p8Root; kubeconfig = $p8Kubeconfig; nodes = @(Get-P8Nodes); kind_version = 'v0.33.0'; kind_sha256 = $p8KindHash; recorded_at = [DateTime]::UtcNow.ToString('o') }
        [IO.File]::WriteAllText($p8OwnerPath, ($p8Owner | ConvertTo-Json -Depth 8), $p8Utf8)
    }
    if (-not (Test-Path -LiteralPath $p8OwnerPath)) { throw 'Prepare the owned cluster first' }
    $p8Owner = [IO.File]::ReadAllText($p8OwnerPath, $p8Utf8) | ConvertFrom-Json
    $p8Kubeconfig = Assert-P8Owner $p8Owner

    switch ($Mode) {
        'PrepareControlMetrics' { Set-P8ControlMetrics $false }
        'RestoreControlMetrics' { Set-P8ControlMetrics $true }
        'Prepare' {
            $p8ImageArchive = Join-Path $p8RunDir 'alpine-amd64.tar'
            Invoke-P8Native 'docker' @('image', 'save', '--platform', 'linux/amd64', '-o', $p8ImageArchive, 'alpine:3.21')
            Invoke-P8Native $p8Kind @('load', 'image-archive', $p8ImageArchive, '--name', $RunId)
            # The other owned node is replacement capacity. No business cluster is modified.
            $p8NodeRaw = & kubectl --kubeconfig $p8Kubeconfig get node "$RunId-control-plane" -o json
            if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect replacement node' }
            $p8Node = $p8NodeRaw | ConvertFrom-Json
            $p8TaintsProperty = $p8Node.spec.PSObject.Properties['taints']
            if ($p8TaintsProperty -and @($p8TaintsProperty.Value | Where-Object { $_.key -eq 'node-role.kubernetes.io/control-plane' }).Count -gt 0) {
                Invoke-P8Native 'kubectl' @('--kubeconfig', $p8Kubeconfig, 'taint', 'node', "$RunId-control-plane", 'node-role.kubernetes.io/control-plane:NoSchedule-')
            }
            Invoke-P8Native 'kubectl' @('--kubeconfig', $p8Kubeconfig, 'wait', '--for=condition=Ready', 'nodes', '--all', '--timeout=90s')
        }
        'Test' {
            if (-not $env:ARGUS_MONITORING_TEST_DATABASE_URL) { throw 'ARGUS_MONITORING_TEST_DATABASE_URL must name an owned disposable database' }
            $p8OldKubeconfig = $env:KUBECONFIG
            $p8OldContext = $env:ARGUS_KUBERNETES_DRAIN_CONTEXT
            try {
                $env:KUBECONFIG = $p8Kubeconfig
                $env:ARGUS_KUBERNETES_DRAIN_CONTEXT = "kind-$RunId"
                $p8Args = @('run', './cmd/argus-dev', 'e2e-plan8', '--suite', 'drain')
                if ($EvidenceRunId) { $p8Args += @('--run-id', $EvidenceRunId) }
                Invoke-P8Native 'go' $p8Args
            } finally {
                $env:KUBECONFIG = $p8OldKubeconfig
                $env:ARGUS_KUBERNETES_DRAIN_CONTEXT = $p8OldContext
            }
        }
        'TestControlMetrics' {
            $p8Overrides = @{
                KUBECONFIG = $p8Kubeconfig
                ARGUS_KUBERNETES_COMPONENTS_CONTEXT = "kind-$RunId"
                ARGUS_KUBERNETES_COMPONENTS_NODE = "$RunId-control-plane"
                ARGUS_KUBERNETES_COMPONENTS_API_PORT = '6443'
                ARGUS_KUBERNETES_COMPONENTS_NETWORK = 'kind'
            }
            $p8Previous = @{}
            try {
                foreach ($p8Key in $p8Overrides.Keys) {
                    $p8Previous[$p8Key] = [Environment]::GetEnvironmentVariable($p8Key, 'Process')
                    [Environment]::SetEnvironmentVariable($p8Key, $p8Overrides[$p8Key], 'Process')
                }
                $p8Args = @('run', './cmd/argus-dev', 'e2e-plan8', '--suite', 'control-positive')
                if ($EvidenceRunId) { $p8Args += @('--run-id', $EvidenceRunId) }
                Invoke-P8Native 'go' $p8Args
            } finally {
                foreach ($p8Key in $p8Previous.Keys) { [Environment]::SetEnvironmentVariable($p8Key, $p8Previous[$p8Key], 'Process') }
            }
        }
        'Cleanup' {
            $p8RawNamespaces = & kubectl --kubeconfig $p8Kubeconfig get namespaces -o json
            if ($LASTEXITCODE -ne 0) { throw 'Cannot verify namespaces before cleanup' }
            $p8Namespaces = $p8RawNamespaces | ConvertFrom-Json
            foreach ($p8Namespace in $p8Namespaces.items) {
                if ($p8Namespace.metadata.name -in @('default', 'kube-system', 'kube-public', 'kube-node-lease', 'local-path-storage')) { continue }
                $p8Label = $p8Namespace.metadata.labels.PSObject.Properties['argus.owner']
                if (-not $p8Label -or $p8Label.Value -notlike 'plan8-*') { throw 'Foreign namespace found; refusing cluster cleanup' }
            }
            $p8DefaultPods = & kubectl --kubeconfig $p8Kubeconfig get pods -n default -o json
            if ($LASTEXITCODE -ne 0 -or @((($p8DefaultPods | ConvertFrom-Json).items)).Count -gt 0) { throw 'Default namespace contains workloads or cannot be verified' }
            $null = Assert-P8Owner $p8Owner
            Invoke-P8Native $p8Kind @('delete', 'cluster', '--name', $RunId, '--kubeconfig', $p8Kubeconfig)
            if (@(Get-P8Nodes).Count -ne 0) { throw 'Owned node containers remain after cleanup' }
            [IO.File]::WriteAllText((Join-Path $p8RunDir 'cleanup.json'), (@{ run_id = $RunId; node_containers_remaining = 0; cleaned_at = [DateTime]::UtcNow.ToString('o') } | ConvertTo-Json), $p8Utf8)
        }
    }
} finally { Pop-Location }
