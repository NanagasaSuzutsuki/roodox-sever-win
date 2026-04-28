param(
    [string]$ConfigPath = "roodox.config.json"
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
$snapshots = @(Get-RoodoxReleaseSnapshots -Layout $layout)
if ($snapshots.Count -eq 0) {
    Write-Output "No release snapshots found under $($layout.ReleaseDir)"
    exit 0
}

$snapshots | Select-Object Name,FullName,LastWriteTime | Format-Table -AutoSize
