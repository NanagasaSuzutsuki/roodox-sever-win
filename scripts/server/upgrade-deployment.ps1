param(
    [string]$ConfigPath = "roodox.config.json",
    [string]$Label = "",
    [switch]$BuildIfMissing,
    [switch]$Rebuild,
    [switch]$RotateServerCert,
    [switch]$RotateRootCA,
    [switch]$StartAfterUpgrade
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
Ensure-RoodoxRuntimeDirectories -Layout $layout
$runtime = Get-RoodoxRuntimeMode -Layout $layout
if ($runtime.UnmanagedProcesses.Count -gt 0) {
    $pids = ($runtime.UnmanagedProcesses | ForEach-Object { $_.Id }) -join ","
    throw "unmanaged server processes are running: $pids"
}

$restartMode = if ($StartAfterUpgrade) {
    if ($runtime.ServiceInstalled) { "service" } else { "process" }
} else {
    $runtime.RestartMode
}

$labelValue = $Label
if ([string]::IsNullOrWhiteSpace($labelValue)) {
    $labelValue = "pre-upgrade-" + (Get-Date).ToString("yyyyMMdd-HHmmss")
}
$snapshot = New-RoodoxReleaseSnapshot -Layout $layout -Label $labelValue

$stopped = $false
try {
    if ($runtime.ServiceRunning) {
        Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "stop-windows-service.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath) -FailureMessage "failed to stop windows service before upgrade"
        $stopped = $true
    } elseif ($runtime.ManagedProcessRunning) {
        Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "stop-server.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath) -FailureMessage "failed to stop managed server process before upgrade"
        $stopped = $true
    }

    Ensure-RoodoxServerBinary -Layout $layout -BuildIfMissing:$BuildIfMissing -Rebuild:$Rebuild -RebuildIfStale

    if ($RotateServerCert -or $RotateRootCA) {
        $rotateArgs = @("-ConfigPath", $layout.ConfigPath)
        if ($RotateRootCA) {
            $rotateArgs += "-RotateRootCA"
        }
        Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "rotate-certificates.ps1") -ArgumentList $rotateArgs -FailureMessage "certificate rotation failed during upgrade"
    }

    if ($restartMode -eq "service") {
        Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "start-windows-service.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath) -FailureMessage "failed to start windows service after upgrade"
    } elseif ($restartMode -eq "process") {
        Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "start-server.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath, "-BuildIfMissing", "-RebuildIfStale") -FailureMessage "failed to start managed server process after upgrade"
    }
}
catch {
    Write-Warning "upgrade failed, restoring snapshot $($snapshot.SnapshotDir)"
    $runtimeAfterFailure = Get-RoodoxRuntimeMode -Layout $layout
    $serviceAfterFailure = Get-RoodoxWindowsService -Layout $layout
    if ($serviceAfterFailure -and $serviceAfterFailure.State -ne "Stopped") {
        Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "stop-windows-service.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath) -FailureMessage "failed to stop windows service before restoring snapshot"
    } elseif ($runtimeAfterFailure.ManagedProcessRunning) {
        Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "stop-server.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath) -FailureMessage "failed to stop managed server process before restoring snapshot"
    }
    Restore-RoodoxReleaseSnapshot -Layout $layout -SnapshotDir $snapshot.SnapshotDir

    if ($restartMode -eq "service") {
        Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "start-windows-service.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath) -FailureMessage "failed to restore windows service after upgrade rollback"
    } elseif ($restartMode -eq "process") {
        Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "start-server.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath, "-BuildIfMissing", "-RebuildIfStale") -FailureMessage "failed to restore managed server process after upgrade rollback"
    }
    throw
}

[pscustomobject]@{
    Upgraded = $true
    SnapshotDir = $snapshot.SnapshotDir
    RestartMode = $restartMode
    RotatedServerCert = ($RotateServerCert -or $RotateRootCA)
    RotatedRootCA = $RotateRootCA
} | Format-List
