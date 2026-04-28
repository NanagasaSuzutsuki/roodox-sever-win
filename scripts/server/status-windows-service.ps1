param(
    [string]$ConfigPath = "roodox.config.json"
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
$service = Get-CimInstance Win32_Service -Filter "Name = '$($layout.ServiceName)'" -ErrorAction SilentlyContinue
if (-not $service) {
    [pscustomobject]@{
        Name        = $layout.ServiceName
        DisplayName = $layout.ServiceDisplayName
        Installed   = $false
        Status      = "not_installed"
        StartType   = $layout.ServiceStartType
    } | Format-List
    exit 0
}

[pscustomobject]@{
    Name        = $service.Name
    DisplayName = $service.DisplayName
    Installed   = $true
    Status      = $service.State
    StartType   = $service.StartMode
    PathName    = $service.PathName
} | Format-List
