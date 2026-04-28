param(
    [string]$ConfigPath = "roodox.config.json",
    [switch]$RotateRootCA,
    [switch]$RestartAfter,
    [string]$BackupDir = "",
    [switch]$BuildIfMissing,
    [switch]$Rebuild
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
$runtime = Get-RoodoxRuntimeMode -Layout $layout

$args = @(
    "-config", $layout.ConfigPath,
    "-rotate-tls"
)

if ($RotateRootCA) {
    $args += "-rotate-tls-root-ca"
}

$resolvedBackupDir = ""
if (-not [string]::IsNullOrWhiteSpace($BackupDir)) {
    $resolvedBackupDir = Resolve-RoodoxPath -PathValue $BackupDir -BaseDir $layout.ConfigDir
    $args += @("-tls-backup-dir", $resolvedBackupDir)
}

$json = Invoke-RoodoxAdminBinary -Layout $layout -ArgumentList $args -BuildIfMissing:$BuildIfMissing -Rebuild:$Rebuild
if ($LASTEXITCODE -ne 0) {
    throw "rotate tls command failed with exit code $LASTEXITCODE"
}

$result = $json | ConvertFrom-Json

if ($RotateRootCA) {
    Write-Warning "root CA was rotated; clients must refresh their trusted roodox-ca-cert.pem before reconnecting after restart"
}

if ($RestartAfter) {
    if ($runtime.ServiceRunning) {
        Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "restart-windows-service.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath) -FailureMessage "failed to restart windows service after certificate rotation"
    } elseif ($runtime.ManagedProcessRunning) {
        Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "restart-server.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath, "-BuildIfMissing") -FailureMessage "failed to restart managed server process after certificate rotation"
    } elseif ($runtime.UnmanagedProcesses.Count -gt 0) {
        $pids = ($runtime.UnmanagedProcesses | ForEach-Object { $_.Id }) -join ","
        Write-Warning "certificates rotated on disk, but unmanaged processes are still running: $pids"
    }
} elseif ($runtime.ServiceRunning -or $runtime.ManagedProcessRunning -or $runtime.UnmanagedProcesses.Count -gt 0) {
    Write-Warning "certificates were rotated on disk; restart the running server process/service to load them"
}

[pscustomobject]@{
    RotatedRootCA = $result.rotated_root_ca
    BackupDir = if ($result.backup_dir) { $result.backup_dir } else { $resolvedBackupDir }
    ServerCertPath = $result.status.cert_path
    RootCertPath = $result.status.root_cert_path
    ServerExpiresAt = if ($result.status.server_not_after_unix) { [DateTimeOffset]::FromUnixTimeSeconds([int64]$result.status.server_not_after_unix).LocalDateTime } else { $null }
    RootExpiresAt = if ($result.status.root_not_after_unix) { [DateTimeOffset]::FromUnixTimeSeconds([int64]$result.status.root_not_after_unix).LocalDateTime } else { $null }
    RestartAfter = $RestartAfter
} | Format-List
