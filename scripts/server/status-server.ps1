param(
    [string]$ConfigPath = "roodox.config.json"
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
$runtime = Get-RoodoxRuntimeMode -Layout $layout

$status = [pscustomobject]@{
    Status        = if ($runtime.ServiceRunning) { "running_service" } elseif ($runtime.ManagedProcessRunning) { "running" } elseif ($runtime.UnmanagedProcesses.Count -gt 0) { "running_unmanaged" } else { "stopped" }
    PID           = if ($runtime.ServiceProcessRunning) { $runtime.ServiceProcess.Id } elseif ($runtime.ManagedProcessRunning) { $runtime.ManagedProcess.Id } elseif ($runtime.UnmanagedProcesses.Count -gt 0) { ($runtime.UnmanagedProcesses | ForEach-Object { $_.Id }) -join "," } else { $null }
    ConfigPath    = $layout.ConfigPath
    BinaryPath    = $layout.BinaryPath
    StateDir      = $layout.StateDir
    PIDFile       = $layout.PIDFile
    StdoutLogPath = $layout.StdoutLogPath
    StderrLogPath = $layout.StderrLogPath
    ServiceName   = if ($runtime.ServiceInstalled) { $layout.ServiceName } else { $null }
}

$status | Format-List
