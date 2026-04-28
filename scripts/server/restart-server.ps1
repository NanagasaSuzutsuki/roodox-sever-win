param(
    [string]$ConfigPath = "roodox.config.json",
    [switch]$BuildIfMissing,
    [switch]$Rebuild,
    [int]$StartupSeconds = 3,
    [int]$StopTimeoutSeconds = 10,
    [switch]$Force,
    [switch]$StopUnmanaged
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$stopArgs = @(
    "-ConfigPath", $ConfigPath,
    "-TimeoutSeconds", $StopTimeoutSeconds
)
if ($Force) {
    $stopArgs += "-Force"
}
if ($StopUnmanaged) {
    $stopArgs += "-StopUnmanaged"
}
Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "stop-server.ps1") -ArgumentList $stopArgs -FailureMessage "stop-server failed during restart"

$startArgs = @(
    "-ConfigPath", $ConfigPath,
    "-StartupSeconds", $StartupSeconds
)
if ($BuildIfMissing) {
    $startArgs += "-BuildIfMissing"
}
if ($Rebuild) {
    $startArgs += "-Rebuild"
}
Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "start-server.ps1") -ArgumentList $startArgs -FailureMessage "start-server failed during restart"
