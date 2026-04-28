param(
    [string]$ConfigPath = "roodox.config.json",
    [switch]$RawJson,
    [switch]$BuildIfMissing,
    [switch]$Rebuild
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
$json = Invoke-RoodoxAdminBinary -Layout $layout -ArgumentList @("-config", $layout.ConfigPath, "-tls-status") -BuildIfMissing:$BuildIfMissing -Rebuild:$Rebuild
if ($LASTEXITCODE -ne 0) {
    throw "tls status command failed with exit code $LASTEXITCODE"
}

if ($RawJson) {
    $json
    exit 0
}

$status = $json | ConvertFrom-Json
$now = Get-Date
$serverExpiry = if ($status.server_not_after_unix) { [DateTimeOffset]::FromUnixTimeSeconds([int64]$status.server_not_after_unix).LocalDateTime } else { $null }
$rootExpiry = if ($status.root_not_after_unix) { [DateTimeOffset]::FromUnixTimeSeconds([int64]$status.root_not_after_unix).LocalDateTime } else { $null }

[pscustomobject]@{
    OverallValid = $status.overall_valid
    ServerValid = $status.server_valid
    RootValid = $status.root_valid
    ServerCertPath = $status.cert_path
    RootCertPath = $status.root_cert_path
    ServerSubject = $status.server_subject
    RootSubject = $status.root_subject
    ServerExpiresAt = $serverExpiry
    RootExpiresAt = $rootExpiry
    ServerDaysRemaining = if ($serverExpiry) { [Math]::Floor(($serverExpiry - $now).TotalDays) } else { $null }
    RootDaysRemaining = if ($rootExpiry) { [Math]::Floor(($rootExpiry - $now).TotalDays) } else { $null }
    ServerDNSNames = (($status.server_dns_names | Where-Object { $_ }) -join ",")
    RootIsCA = $status.root_is_ca
} | Format-List
