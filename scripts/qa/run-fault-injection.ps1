param(
    [string]$ConfigPath = "roodox.config.json",
    [switch]$KeepArtifacts
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)

Push-Location $repoRoot
try {
    $args = @("run", "./cmd/roodox_qa", "faults", "-config", $ConfigPath)
    if ($KeepArtifacts) {
        $args += "-keep-artifacts"
    }
    & go @args
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}
finally {
    Pop-Location
}
