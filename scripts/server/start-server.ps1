param(
    [string]$ConfigPath = "roodox.config.json",
    [switch]$Foreground,
    [switch]$BuildIfMissing,
    [switch]$Rebuild,
    [switch]$RebuildIfStale,
    [int]$StartupSeconds = 3
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
Ensure-RoodoxRuntimeDirectories -Layout $layout

$service = Get-RoodoxWindowsService -Layout $layout
if ($service -and $service.State -eq "Running") {
    Write-Output "Roodox windows service already running. name=$($layout.ServiceName) config=$($layout.ConfigPath)"
    exit 1
}

$existing = Get-RoodoxManagedProcess -Layout $layout -CleanStalePID
if ($existing) {
    Write-Output "Roodox server already running. pid=$($existing.Id) config=$($layout.ConfigPath)"
    exit 1
}

$unmanaged = @(Get-RoodoxBinaryProcesses -Layout $layout)
if ($unmanaged.Count -gt 0) {
    $pids = ($unmanaged | ForEach-Object { $_.Id }) -join ","
    Write-Output "Roodox server already running without PID management. pid=$pids binary=$($layout.BinaryPath)"
    exit 1
}

Ensure-RoodoxServerBinary -Layout $layout -BuildIfMissing:$BuildIfMissing -Rebuild:$Rebuild -RebuildIfStale:$RebuildIfStale

if ($Foreground) {
    Push-Location $layout.ConfigDir
    try {
        & $layout.BinaryPath -config $layout.ConfigPath
        exit $LASTEXITCODE
    }
    finally {
        Pop-Location
    }
}

$previousPathLower = [System.Environment]::GetEnvironmentVariable("Path", "Process")
$previousPathUpper = [System.Environment]::GetEnvironmentVariable("PATH", "Process")
$effectivePath = if (-not [string]::IsNullOrWhiteSpace($previousPathUpper)) { $previousPathUpper } else { $previousPathLower }
if ([string]::IsNullOrWhiteSpace($effectivePath)) {
    $effectivePath = [System.Environment]::GetEnvironmentVariable("Path", "Machine")
}
[System.Environment]::SetEnvironmentVariable("Path", $null, "Process")
[System.Environment]::SetEnvironmentVariable("PATH", $effectivePath, "Process")

try {
    $proc = Start-Process `
        -FilePath $layout.BinaryPath `
        -ArgumentList @("-config", $layout.ConfigPath) `
        -WorkingDirectory $layout.ConfigDir `
        -RedirectStandardOutput $layout.StdoutLogPath `
        -RedirectStandardError $layout.StderrLogPath `
        -WindowStyle Hidden `
        -PassThru
}
finally {
    [System.Environment]::SetEnvironmentVariable("Path", $previousPathLower, "Process")
    [System.Environment]::SetEnvironmentVariable("PATH", $previousPathUpper, "Process")
}

$deadline = (Get-Date).AddSeconds([Math]::Max($StartupSeconds, 1))
while ((Get-Date) -lt $deadline) {
    Start-Sleep -Milliseconds 250
    try {
        $proc = Get-Process -Id $proc.Id -ErrorAction Stop
    }
    catch {
        $stderrTail = ""
        if (Test-Path -LiteralPath $layout.StderrLogPath) {
            $stderrTail = ((Get-Content -LiteralPath $layout.StderrLogPath -Tail 20) -join [Environment]::NewLine)
        }
        throw "server exited during startup. stderr:`n$stderrTail"
    }
}

Write-RoodoxPidFile -Layout $layout -Process $proc
Write-Output "Roodox server started. pid=$($proc.Id) config=$($layout.ConfigPath)"
Write-Output "stdout=$($layout.StdoutLogPath)"
Write-Output "stderr=$($layout.StderrLogPath)"
