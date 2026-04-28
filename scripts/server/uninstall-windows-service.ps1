param(
    [string]$ConfigPath = "roodox.config.json",
    [switch]$Force
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

if (-not (Test-RoodoxAdministrator)) {
    throw "administrator privileges are required to uninstall the Windows service"
}

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
$service = Get-Service -Name $layout.ServiceName -ErrorAction SilentlyContinue
if (-not $service) {
    Write-Output "Windows service is not installed. name=$($layout.ServiceName)"
    exit 0
}

if ($service.Status -ne "Stopped") {
    if ($Force) {
        Stop-Service -Name $layout.ServiceName -Force
    } else {
        Stop-Service -Name $layout.ServiceName
    }
    $service.WaitForStatus("Stopped", [TimeSpan]::FromSeconds(20))
}

sc.exe delete $layout.ServiceName | Out-Null
Write-Output "Windows service removed. name=$($layout.ServiceName)"
