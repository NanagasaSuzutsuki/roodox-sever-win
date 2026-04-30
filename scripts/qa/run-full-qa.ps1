param(
    [string]$ConfigPath = "testdata/deployment-smoke/roodox-smoke.config.json",
    [string]$SoakDuration = "2m",
    [int]$SoakWorkers = 4,
    [string]$BuildInterval = "20s"
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
. (Join-Path $repoRoot "scripts/server/common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
$runtime = Get-RoodoxRuntimeMode -Layout $layout
$startedForQA = $false

if (-not $runtime.ServiceRunning -and -not $runtime.ManagedProcessRunning -and $runtime.UnmanagedProcesses.Count -eq 0) {
    & (Join-Path $repoRoot "scripts/server/start-server.ps1") -ConfigPath $layout.ConfigPath -BuildIfMissing
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    $startedForQA = $true
}

try {
    & (Join-Path $PSScriptRoot "run-live-regression.ps1") -ConfigPath $layout.ConfigPath
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    & (Join-Path $PSScriptRoot "run-fault-injection.ps1") -ConfigPath $layout.ConfigPath
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    & (Join-Path $PSScriptRoot "run-soak.ps1") -ConfigPath $layout.ConfigPath -Duration $SoakDuration -Workers $SoakWorkers -BuildInterval $BuildInterval
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    & (Join-Path $PSScriptRoot "run-restart-recovery.ps1") -ConfigPath $layout.ConfigPath
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
finally {
    if ($startedForQA) {
        & (Join-Path $repoRoot "scripts/server/stop-server.ps1") -ConfigPath $layout.ConfigPath
    }
}
