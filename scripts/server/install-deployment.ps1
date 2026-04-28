param(
    [string]$ConfigPath = "roodox.config.json",
    [switch]$AsService,
    [switch]$StartAfterInstall,
    [switch]$BuildIfMissing,
    [switch]$Rebuild
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
Ensure-RoodoxRuntimeDirectories -Layout $layout
Ensure-RoodoxServerBinary -Layout $layout -BuildIfMissing:$BuildIfMissing -Rebuild:$Rebuild -RebuildIfStale

if (-not (Test-Path -LiteralPath $layout.TLSCertPath) -or -not (Test-Path -LiteralPath $layout.TLSRootCertPath)) {
    Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "rotate-certificates.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath, "-RotateRootCA") -FailureMessage "failed to initialize tls certificates during install"
}

$snapshot = New-RoodoxReleaseSnapshot -Layout $layout -Label ("install-" + (Get-Date).ToString("yyyyMMdd-HHmmss"))

if ($AsService) {
    $installArgs = @("-ConfigPath", $layout.ConfigPath)
    if ($BuildIfMissing) {
        $installArgs += "-BuildIfMissing"
    }
    if ($Rebuild) {
        $installArgs += "-Rebuild"
    }
    if ($StartAfterInstall) {
        $installArgs += "-StartAfterInstall"
    }
    Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "install-windows-service.ps1") -ArgumentList $installArgs -FailureMessage "windows service installation failed"
} elseif ($StartAfterInstall) {
    $startArgs = @("-ConfigPath", $layout.ConfigPath)
    if ($BuildIfMissing) {
        $startArgs += "-BuildIfMissing"
    }
    if ($Rebuild) {
        $startArgs += "-Rebuild"
    }
    Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "start-server.ps1") -ArgumentList $startArgs -FailureMessage "server start failed after install"
}

[pscustomobject]@{
    Installed = $true
    SnapshotDir = $snapshot.SnapshotDir
    Mode = if ($AsService) { "service" } else { "process_or_files_only" }
    Started = $StartAfterInstall
} | Format-List
