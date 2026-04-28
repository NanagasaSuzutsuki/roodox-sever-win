param(
    [string]$ConfigPath = "roodox.config.json",
    [int]$TimeoutSeconds = 10,
    [switch]$Force,
    [switch]$StopUnmanaged
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
$proc = Get-RoodoxManagedProcess -Layout $layout -CleanStalePID
if (-not $proc) {
    if (-not $StopUnmanaged) {
        Write-Output "Roodox server is not running for config=$($layout.ConfigPath)"
        exit 0
    }

    $unmanaged = @(Get-RoodoxBinaryProcesses -Layout $layout)
    if ($unmanaged.Count -eq 0) {
        Write-Output "Roodox server is not running for config=$($layout.ConfigPath)"
        exit 0
    }
    if ($unmanaged.Count -gt 1) {
        $pids = ($unmanaged | ForEach-Object { $_.Id }) -join ","
        throw "multiple unmanaged server processes match $($layout.BinaryPath): $pids"
    }
    $proc = $unmanaged[0]
}

$shutdownRequested = $false
if (-not $Force) {
    try {
        & $layout.BinaryPath -config $layout.ConfigPath -request-shutdown -shutdown-reason "scripts/server/stop-server.ps1"
        if ($LASTEXITCODE -eq 0) {
            $shutdownRequested = $true
        }
    }
    catch {
        Write-Warning "graceful shutdown request failed, falling back to process stop: $($_.Exception.Message)"
    }
}

if (-not $shutdownRequested) {
    if ($Force) {
        Stop-Process -Id $proc.Id -Force
    } else {
        Stop-Process -Id $proc.Id
    }
}

$deadline = (Get-Date).AddSeconds([Math]::Max($TimeoutSeconds, 1))
while ((Get-Date) -lt $deadline) {
    try {
        $null = Get-Process -Id $proc.Id -ErrorAction Stop
        Start-Sleep -Milliseconds 250
    }
    catch {
        Remove-RoodoxPidFile -PIDFile $layout.PIDFile
        Write-Output "Roodox server stopped. pid=$($proc.Id)"
        exit 0
    }
}

throw "server process $($proc.Id) did not exit within $TimeoutSeconds seconds"
