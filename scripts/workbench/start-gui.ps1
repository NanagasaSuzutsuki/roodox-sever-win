param(
    [string]$ConfigPath,
    [switch]$BuildIfMissing,
    [switch]$Rebuild,
    [switch]$Wait
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxWorkbenchLayout -ConfigPath $ConfigPath
$serverCommonPath = Join-Path $layout.RepoRoot "scripts/server/common.ps1"
. $serverCommonPath

if (-not (Test-Path -LiteralPath $layout.ConfigPath)) {
    throw "config file not found: $($layout.ConfigPath)"
}

$serverLayout = Get-RoodoxServerLayout -ConfigPath $layout.ConfigPath
Ensure-RoodoxRuntimeDirectories -Layout $serverLayout
$runtimeMode = Get-RoodoxRuntimeMode -Layout $serverLayout
if (-not $runtimeMode.ServiceRunning -and -not $runtimeMode.ManagedProcessRunning -and $runtimeMode.UnmanagedProcesses.Count -eq 0) {
    Ensure-RoodoxServerBinary -Layout $serverLayout -BuildIfMissing -RebuildIfStale
}

$needsBuild = $Rebuild -or -not (Test-Path -LiteralPath $layout.ExecutablePath) -or -not (Test-Path -LiteralPath $layout.BuildMarkerPath)
if ($needsBuild) {
    Invoke-RoodoxWorkbenchTauriBuild -Layout $layout -Mode "run"
}
else {
    Write-RoodoxWorkbenchBootstrap -Layout $layout -DestinationPath $layout.BootstrapPath
}

$process = Start-Process -FilePath $layout.ExecutablePath -WorkingDirectory $layout.TargetReleaseDir -PassThru
if ($Wait) {
    Wait-Process -Id $process.Id
    $process.Refresh()
}

[pscustomobject]@{
    ProcessId = $process.Id
    Executable = $layout.ExecutablePath
    ConfigPath = $layout.ConfigPath
    WorkbenchRoot = $layout.RepoRoot
    BuiltViaTauri = $true
    Running = -not $process.HasExited
}
