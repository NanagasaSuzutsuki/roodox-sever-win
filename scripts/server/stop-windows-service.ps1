param(
    [string]$ConfigPath = "roodox.config.json",
    [int]$TimeoutSeconds = 30,
    [switch]$Force
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
$service = Get-Service -Name $layout.ServiceName -ErrorAction SilentlyContinue
if (-not $service) {
    Write-Output "Windows service is not installed. name=$($layout.ServiceName)"
    exit 0
}
$serviceInfo = Get-RoodoxWindowsService -Layout $layout
$servicePid = 0
if ($serviceInfo) {
    $servicePid = [int]$serviceInfo.ProcessId
}

if ($service.Status -ne "Stopped") {
    if ($service.Status -ne "StopPending") {
        if ($Force) {
            Stop-Service -Name $layout.ServiceName -Force
        } else {
            Stop-Service -Name $layout.ServiceName
        }
    }
    Wait-RoodoxServiceStatus -ServiceName $layout.ServiceName -Status "Stopped" -TimeoutSeconds $TimeoutSeconds
}

if ($servicePid -gt 0) {
    Wait-RoodoxProcessExit -ProcessId $servicePid -TimeoutSeconds $TimeoutSeconds
}

Write-Output "Windows service stopped. name=$($layout.ServiceName)"
