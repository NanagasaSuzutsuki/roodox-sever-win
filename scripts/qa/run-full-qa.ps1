param(
    [string]$ConfigPath = "roodox.config.json",
    [string]$SoakDuration = "2m",
    [int]$SoakWorkers = 4,
    [string]$BuildInterval = "20s"
)

$ErrorActionPreference = "Stop"

& (Join-Path $PSScriptRoot "run-live-regression.ps1") -ConfigPath $ConfigPath
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

& (Join-Path $PSScriptRoot "run-fault-injection.ps1") -ConfigPath $ConfigPath
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

& (Join-Path $PSScriptRoot "run-soak.ps1") -ConfigPath $ConfigPath -Duration $SoakDuration -Workers $SoakWorkers -BuildInterval $BuildInterval
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

& (Join-Path $PSScriptRoot "run-restart-recovery.ps1") -ConfigPath $ConfigPath
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
