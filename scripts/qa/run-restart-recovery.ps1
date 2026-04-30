param(
    [string]$ConfigPath = "testdata/deployment-smoke/roodox-smoke.config.json",
    [int]$PreSeconds = 5,
    [int]$DownSeconds = 7,
    [int]$PostSeconds = 14,
    [switch]$KeepLogs,
    [switch]$CaptureRestartServerLogs
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
. (Join-Path $repoRoot "scripts/server/common.ps1")
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$logRoot = Join-Path $env:TEMP "roodox-qa"
New-Item -ItemType Directory -Force -Path $logRoot | Out-Null

function Remove-QAPaths {
    param(
        [string[]]$Paths
    )

    foreach ($path in $Paths) {
        if ([string]::IsNullOrWhiteSpace($path)) {
            continue
        }
        if (-not (Test-Path -LiteralPath $path)) {
            continue
        }
        try {
            Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction Stop
        }
        catch {
            Write-Warning "skip cleanup for ${path}: $($_.Exception.Message)"
        }
    }
}

function Clear-RestartLogs {
    param(
        [string]$RootPath,
        [string[]]$ExcludePaths = @()
    )

    if (-not (Test-Path -LiteralPath $RootPath)) {
        return
    }

    $excluded = @{}
    foreach ($excludePath in $ExcludePaths) {
        if ([string]::IsNullOrWhiteSpace($excludePath)) {
            continue
        }
        $excluded[$excludePath] = $true
    }

    $targets = Get-ChildItem -LiteralPath $RootPath -File -ErrorAction SilentlyContinue | Where-Object {
        $_.Name -like "restart-*.log" -and -not $excluded.ContainsKey($_.FullName)
    }
    foreach ($target in $targets) {
        try {
            Remove-Item -LiteralPath $target.FullName -Force -ErrorAction Stop
        }
        catch {
            Write-Warning "skip stale log cleanup for $($target.FullName): $($_.Exception.Message)"
        }
    }
}

if (-not $KeepLogs) {
    Clear-RestartLogs -RootPath $logRoot
}

$probeStdout = Join-Path $logRoot "restart-probe-$timestamp.stdout.log"
$probeStderr = Join-Path $logRoot "restart-probe-$timestamp.stderr.log"
$serverStdout = Join-Path $logRoot "restart-server-$timestamp.stdout.log"
$serverStderr = Join-Path $logRoot "restart-server-$timestamp.stderr.log"

$probeProc = $null
$newSvc = $null
$probeSucceeded = $false
$layout = $null

Push-Location $repoRoot
try {
    $layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
    Ensure-RoodoxRuntimeDirectories -Layout $layout
    Ensure-RoodoxServerBinary -Layout $layout -BuildIfMissing -RebuildIfStale

    $probeArgs = @(
        "run", "./cmd/roodox_qa", "probe",
        "-config", $layout.ConfigPath,
        "-pre", "$($PreSeconds)s",
        "-down", "$($DownSeconds)s",
        "-post", "$($PostSeconds)s"
    )
    $probeProc = Start-Process -FilePath "go" -ArgumentList $probeArgs -WorkingDirectory $repoRoot -RedirectStandardOutput $probeStdout -RedirectStandardError $probeStderr -PassThru

    Start-Sleep -Seconds $PreSeconds

    $svc = Get-RoodoxManagedProcess -Layout $layout -CleanStalePID
    if (-not $svc) {
        $unmanaged = @(Get-RoodoxBinaryProcesses -Layout $layout)
        if ($unmanaged.Count -gt 1) {
            $pids = ($unmanaged | ForEach-Object { $_.Id }) -join ","
            throw "multiple server processes match $($layout.BinaryPath): $pids"
        }
        if ($unmanaged.Count -eq 1) {
            $svc = $unmanaged[0]
        }
    }
    if (-not $svc) {
        throw "server process not found before restart test for config=$($layout.ConfigPath)"
    }

    Stop-Process -Id $svc.Id -Force
    Remove-RoodoxPidFile -PIDFile $layout.PIDFile
    Start-Sleep -Seconds $DownSeconds
    if (-not $KeepLogs) {
        Clear-RestartLogs -RootPath $logRoot -ExcludePaths @($probeStdout, $probeStderr)
    }

    $serverStart = @{
        FilePath = $layout.BinaryPath
        ArgumentList = @("-config", $layout.ConfigPath)
        WorkingDirectory = $layout.ConfigDir
        PassThru = $true
    }
    if ($CaptureRestartServerLogs) {
        $serverStart.RedirectStandardOutput = $serverStdout
        $serverStart.RedirectStandardError = $serverStderr
    }
    else {
        $serverStart.WindowStyle = "Hidden"
    }

    $newSvc = Start-Process @serverStart
    Write-RoodoxPidFile -Layout $layout -Process $newSvc
    $probeProc | Wait-Process -Timeout ([Math]::Max(($PreSeconds + $DownSeconds + $PostSeconds + 30), 60))

    Get-Content -Path $probeStdout
    if (Test-Path $probeStderr) {
        Get-Content -Path $probeStderr
    }

    if ($probeProc.ExitCode -ne 0) {
        if (Test-Path $serverStderr) {
            Get-Content -Path $serverStderr
        }
        exit $probeProc.ExitCode
    }

    $probeSucceeded = $true
    Get-Process -Id $newSvc.Id | Select-Object Id, ProcessName, StartTime | Format-List
}
finally {
    $cleanupTargets = @($probeStdout, $probeStderr)
    if ($CaptureRestartServerLogs) {
        $cleanupTargets += @($serverStdout, $serverStderr)
    }

    if (-not $KeepLogs -and $probeSucceeded) {
        Remove-QAPaths -Paths $cleanupTargets
        try {
            if ((Test-Path -LiteralPath $logRoot) -and -not (Get-ChildItem -LiteralPath $logRoot -Force -ErrorAction SilentlyContinue)) {
                Remove-Item -LiteralPath $logRoot -Force -ErrorAction Stop
            }
        }
        catch {
            Write-Warning "skip cleanup for ${logRoot}: $($_.Exception.Message)"
        }
    }

    Pop-Location
}
