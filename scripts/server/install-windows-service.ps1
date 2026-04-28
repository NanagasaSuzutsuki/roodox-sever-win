param(
    [string]$ConfigPath = "roodox.config.json",
    [switch]$BuildIfMissing,
    [switch]$Rebuild,
    [switch]$StartAfterInstall
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

if (-not (Test-RoodoxAdministrator)) {
    throw "administrator privileges are required to install the Windows service"
}

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
Ensure-RoodoxRuntimeDirectories -Layout $layout
Ensure-RoodoxServerBinary -Layout $layout -BuildIfMissing:$BuildIfMissing -Rebuild:$Rebuild -RebuildIfStale

$serviceName = $layout.ServiceName
$existing = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
if ($existing) {
    throw "windows service already exists: $serviceName"
}

$startType = switch ($layout.ServiceStartType.ToLowerInvariant()) {
    "manual" { "Manual" }
    "disabled" { "Disabled" }
    default { "Automatic" }
}

$binPath = "`"$($layout.BinaryPath)`" -config `"$($layout.ConfigPath)`" -service-name `"$serviceName`""
New-Service -Name $serviceName -BinaryPathName $binPath -DisplayName $layout.ServiceDisplayName -Description $layout.ServiceDescription -StartupType $startType | Out-Null

if ($StartAfterInstall -and $startType -ne "Disabled") {
    Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "start-windows-service.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath) -FailureMessage "failed to start windows service after install"
}

Write-Output "Windows service installed. name=$serviceName"
