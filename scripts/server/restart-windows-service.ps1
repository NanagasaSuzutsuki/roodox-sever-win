param(
    [string]$ConfigPath = "roodox.config.json",
    [int]$TimeoutSeconds = 30,
    [switch]$Force
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$stopArgs = @("-ConfigPath", $ConfigPath, "-TimeoutSeconds", $TimeoutSeconds)
if ($Force) {
    $stopArgs += "-Force"
}
Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "stop-windows-service.ps1") -ArgumentList $stopArgs -FailureMessage "failed to stop windows service during restart"
Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "start-windows-service.ps1") -ArgumentList @("-ConfigPath", $ConfigPath, "-TimeoutSeconds", $TimeoutSeconds) -FailureMessage "failed to start windows service during restart"
