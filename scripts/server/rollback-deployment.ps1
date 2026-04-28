param(
    [string]$ConfigPath = "roodox.config.json",
    [string]$SnapshotLabel = "",
    [switch]$Latest,
    [switch]$StartAfterRollback
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
$runtime = Get-RoodoxRuntimeMode -Layout $layout
if ($runtime.UnmanagedProcesses.Count -gt 0) {
    $pids = ($runtime.UnmanagedProcesses | ForEach-Object { $_.Id }) -join ","
    throw "unmanaged server processes are running: $pids"
}

$restartMode = if ($StartAfterRollback) {
    if ($runtime.ServiceInstalled) { "service" } else { "process" }
} else {
    $runtime.RestartMode
}

$snapshotDir = ""
if (-not [string]::IsNullOrWhiteSpace($SnapshotLabel)) {
    $snapshotDir = Join-Path $layout.ReleaseDir $SnapshotLabel
} else {
    $snapshots = @(Get-RoodoxReleaseSnapshots -Layout $layout)
    if ($snapshots.Count -eq 0) {
        throw "no release snapshots found under $($layout.ReleaseDir)"
    }
    $snapshotDir = $snapshots[0].FullName
}

if (-not (Test-Path -LiteralPath $snapshotDir)) {
    throw "snapshot directory not found: $snapshotDir"
}

if ($runtime.ServiceRunning) {
    Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "stop-windows-service.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath) -FailureMessage "failed to stop windows service before rollback"
} elseif ($runtime.ManagedProcessRunning) {
    Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "stop-server.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath) -FailureMessage "failed to stop managed server process before rollback"
}

Restore-RoodoxReleaseSnapshot -Layout $layout -SnapshotDir $snapshotDir

if ($restartMode -eq "service") {
    Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "start-windows-service.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath) -FailureMessage "failed to start windows service after rollback"
} elseif ($restartMode -eq "process") {
    Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "start-server.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath, "-BuildIfMissing") -FailureMessage "failed to start managed server process after rollback"
}

[pscustomobject]@{
    RolledBack = $true
    SnapshotDir = $snapshotDir
    RestartMode = $restartMode
} | Format-List
