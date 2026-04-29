param(
    [string]$ConfigPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxWorkbenchLayout -ConfigPath $ConfigPath
$serverCommonPath = Join-Path $layout.RepoRoot "scripts/server/common.ps1"
. $serverCommonPath

function Sync-RoodoxWorkbenchHandoffArtifacts {
    param(
        [psobject]$WorkbenchLayout,
        [psobject]$ServerLayout
    )

    $handoffDir = $WorkbenchLayout.HandoffSourceDir
    $accessDir = Join-Path $handoffDir "client-access"
    New-Item -ItemType Directory -Force -Path $handoffDir, $accessDir | Out-Null

    $caPath = Join-Path $handoffDir "roodox-ca-cert.pem"
    Invoke-RoodoxAdminBinary -Layout $ServerLayout -ArgumentList @(
        "-config", $ServerLayout.ConfigPath,
        "-export-client-ca", $caPath
    ) -BuildIfMissing | Out-Null

    $joinBundleRaw = Invoke-RoodoxAdminBinary -Layout $ServerLayout -ArgumentList @(
        "-config", $ServerLayout.ConfigPath,
        "-issue-join-bundle-json"
    ) -BuildIfMissing
    $joinBundleResult = (($joinBundleRaw | Out-String).Trim()) | ConvertFrom-Json
    $bundlePath = Join-Path $accessDir "roodox-client-access.json"
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($bundlePath, [string]$joinBundleResult.bundle_json, $utf8NoBom)
}

if (-not (Test-Path -LiteralPath $layout.ConfigPath)) {
    throw "config file not found: $($layout.ConfigPath)"
}

$serverLayout = Get-RoodoxServerLayout -ConfigPath $layout.ConfigPath
Ensure-RoodoxRuntimeDirectories -Layout $serverLayout
$runtimeMode = Get-RoodoxRuntimeMode -Layout $serverLayout
if (-not $runtimeMode.ServiceRunning -and -not $runtimeMode.ManagedProcessRunning -and $runtimeMode.UnmanagedProcesses.Count -eq 0) {
    Ensure-RoodoxServerBinary -Layout $serverLayout -BuildIfMissing -RebuildIfStale
}
Sync-RoodoxWorkbenchHandoffArtifacts -WorkbenchLayout $layout -ServerLayout $serverLayout

Invoke-RoodoxWorkbenchTauriBuild -Layout $layout -Mode "run"
$published = Publish-RoodoxWorkbenchArtifacts -Layout $layout -HandoffSourceDir $layout.HandoffSourceDir

[pscustomobject]@{
    ArtifactRoot = $published.ArtifactRoot
    PortableDir = $published.PortableDir
    PortableExecutable = $published.PortableExecutable
    PortableLauncher = $published.PortableLauncher
    PortableBootstrap = $published.PortableBootstrap
    MsiPath = $published.MsiPath
    DeliveryDir = $published.DeliveryDir
    DeliveryZip = $published.DeliveryZip
    DeliveryHandoffDir = $published.DeliveryHandoffDir
    ConfigPath = $layout.ConfigPath
}
