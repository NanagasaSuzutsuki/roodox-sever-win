param(
    [string]$ConfigPath = "testdata/deployment-smoke/roodox-smoke.config.json",
    [string]$Duration = "2m",
    [int]$Workers = 4,
    [string]$BuildInterval = "20s",
    [switch]$KeepArtifacts
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)

Push-Location $repoRoot
try {
    $args = @(
        "run", "./cmd/roodox_qa", "soak",
        "-config", $ConfigPath,
        "-duration", $Duration,
        "-workers", "$Workers",
        "-build-interval", $BuildInterval
    )
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
