param(
    [string]$ConfigPath = "roodox.config.json",
    [string]$BackupPath = "",
    [switch]$Latest,
    [switch]$NoSafetyBackup
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath

$service = Get-CimInstance Win32_Service -Filter "Name = '$($layout.ServiceName)'" -ErrorAction SilentlyContinue
if ($service -and $service.State -eq "Running") {
    throw "windows service $($layout.ServiceName) is running; stop it before restoring the database"
}

$managed = Get-RoodoxManagedProcess -Layout $layout -CleanStalePID
if ($managed) {
    throw "managed server process is running (pid=$($managed.Id)); stop it before restoring the database"
}

$unmanaged = @(Get-RoodoxBinaryProcesses -Layout $layout)
if ($unmanaged.Count -gt 0) {
    $pids = ($unmanaged | ForEach-Object { $_.Id }) -join ","
    throw "server binary is still running outside runtime management (pid=$pids); stop it before restoring the database"
}

if ($Latest) {
    $dbBase = [System.IO.Path]::GetFileNameWithoutExtension($layout.DBPath)
    if ([string]::IsNullOrWhiteSpace($dbBase)) {
        $dbBase = "roodox"
    }
    $pattern = "$dbBase-backup-*.db"
    $candidate = Get-ChildItem -LiteralPath $layout.BackupDir -Filter $pattern -File -ErrorAction SilentlyContinue |
        Sort-Object LastWriteTimeUtc -Descending |
        Select-Object -First 1
    if (-not $candidate) {
        throw "no backup files found under $($layout.BackupDir) matching $pattern"
    }
    $BackupPath = $candidate.FullName
}

if ([string]::IsNullOrWhiteSpace($BackupPath)) {
    throw "specify -BackupPath or use -Latest"
}

$resolvedBackupPath = Resolve-RoodoxPath -PathValue $BackupPath -BaseDir $layout.ConfigDir
if (-not (Test-Path -LiteralPath $resolvedBackupPath)) {
    throw "backup file not found: $resolvedBackupPath"
}

Ensure-RoodoxServerBinary -Layout $layout -BuildIfMissing -RebuildIfStale

$args = @(
    "-config", $layout.ConfigPath,
    "-restore-db-from", $resolvedBackupPath
)
if ($NoSafetyBackup) {
    $args += "-restore-db-no-safety-backup"
}

& $layout.BinaryPath @args
if ($LASTEXITCODE -ne 0) {
    throw "database restore failed with exit code $LASTEXITCODE"
}

[pscustomobject]@{
    Restored = $true
    DBPath = $layout.DBPath
    BackupPath = $resolvedBackupPath
    SafetyBackup = (-not $NoSafetyBackup)
} | Format-List
