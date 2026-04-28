param(
    [string]$ConfigPath = "roodox.config.json",
    [int]$TimeoutSeconds = 30
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
$service = Get-Service -Name $layout.ServiceName -ErrorAction SilentlyContinue
if (-not $service) {
    throw "windows service is not installed: $($layout.ServiceName)"
}

if ($service.Status -ne "Running") {
    if ($service.Status -ne "StartPending") {
        Start-Service -Name $layout.ServiceName
    }
    Wait-RoodoxServiceStatus -ServiceName $layout.ServiceName -Status "Running" -TimeoutSeconds $TimeoutSeconds
}

$serviceInfo = Get-RoodoxWindowsService -Layout $layout
$proc = Get-RoodoxServiceProcess -Layout $layout -Service $serviceInfo
if (-not $proc) {
    throw "windows service reported running, but no matching server process was found"
}

Write-Output "Windows service started. name=$($layout.ServiceName) pid=$($proc.Id)"
