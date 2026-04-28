param(
    [string]$ConfigPath = "roodox.config.json",
    [string]$DestinationPath = "roodox-ca-cert.pem",
    [switch]$BuildIfMissing,
    [switch]$Rebuild
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath

$resolvedDestination = Resolve-RoodoxPath -PathValue $DestinationPath -BaseDir (Get-Location).Path
$json = Invoke-RoodoxAdminBinary -Layout $layout -ArgumentList @("-config", $layout.ConfigPath, "-export-client-ca", $resolvedDestination) -BuildIfMissing:$BuildIfMissing -Rebuild:$Rebuild
if ($LASTEXITCODE -ne 0) {
    throw "export client ca command failed with exit code $LASTEXITCODE"
}

$result = $json | ConvertFrom-Json
[pscustomobject]@{
    SourcePath = $result.root_cert_path
    ExportedPath = $result.exported_path
} | Format-List
