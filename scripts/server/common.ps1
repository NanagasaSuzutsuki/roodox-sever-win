Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Get-RoodoxServerRepoRoot {
    return Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
}

function Resolve-RoodoxPath {
    param(
        [string]$PathValue,
        [string]$BaseDir
    )

    if ([string]::IsNullOrWhiteSpace($PathValue)) {
        return ""
    }
    if ([System.IO.Path]::IsPathRooted($PathValue)) {
        return [System.IO.Path]::GetFullPath($PathValue)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $BaseDir $PathValue))
}

function Resolve-RoodoxConfigPath {
    param(
        [string]$ConfigPath,
        [string]$RepoRoot
    )

    if ([string]::IsNullOrWhiteSpace($ConfigPath)) {
        return Join-Path $RepoRoot "roodox.config.json"
    }
    if ([System.IO.Path]::IsPathRooted($ConfigPath)) {
        return [System.IO.Path]::GetFullPath($ConfigPath)
    }

    $cwdCandidate = Join-Path (Get-Location).Path $ConfigPath
    if (Test-Path -LiteralPath $cwdCandidate) {
        return [System.IO.Path]::GetFullPath($cwdCandidate)
    }
    return [System.IO.Path]::GetFullPath((Join-Path $RepoRoot $ConfigPath))
}

function Get-RoodoxOptionalProperty {
    param(
        [object]$Object,
        [string]$Name
    )

    if ($null -eq $Object) {
        return $null
    }
    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) {
        return $null
    }
    return $property.Value
}

function Join-RoodoxConfigPath {
    param(
        [string]$Base,
        [string]$Relative
    )

    $baseValue = [string]$Base
    $relativeValue = [string]$Relative
    if ([string]::IsNullOrWhiteSpace($baseValue)) {
        return $relativeValue
    }
    if ([string]::IsNullOrWhiteSpace($relativeValue)) {
        return $baseValue
    }
    return ([System.IO.Path]::Combine($baseValue, $relativeValue)).Replace("\", "/")
}

function Test-RoodoxConfigPathEquals {
    param(
        [string]$Left,
        [string]$Right
    )

    $normalize = {
        param([string]$Value)
        if ([string]::IsNullOrWhiteSpace($Value)) {
            return ""
        }
        return ([string]$Value).Replace("\", "/").Trim()
    }

    return [string]::Equals((& $normalize $Left), (& $normalize $Right), [System.StringComparison]::OrdinalIgnoreCase)
}

function Get-RoodoxDataPathDefault {
    param(
        [string]$DataRoot,
        [string]$Fallback
    )

    if ([string]::IsNullOrWhiteSpace($DataRoot)) {
        return $Fallback
    }
    return Join-RoodoxConfigPath -Base $DataRoot -Relative $Fallback
}

function Get-RoodoxServerLayout {
    param(
        [string]$ConfigPath
    )

    $repoRoot = Get-RoodoxServerRepoRoot
    $resolvedConfigPath = Resolve-RoodoxConfigPath -ConfigPath $ConfigPath -RepoRoot $repoRoot
    if (-not (Test-Path -LiteralPath $resolvedConfigPath)) {
        throw "config file not found: $resolvedConfigPath"
    }

    $configDir = Split-Path -Parent $resolvedConfigPath
    $config = Get-Content -LiteralPath $resolvedConfigPath -Raw | ConvertFrom-Json
    $runtime = Get-RoodoxOptionalProperty -Object $config -Name "runtime"
    $dataRootValue = [string](Get-RoodoxOptionalProperty -Object $config -Name "data_root")

    $binaryPathValue = if ($runtime) { Get-RoodoxOptionalProperty -Object $runtime -Name "binary_path" } else { $null }
    if ([string]::IsNullOrWhiteSpace([string]$binaryPathValue)) { $binaryPathValue = "roodox_server.exe" }
    $binaryPath = Resolve-RoodoxPath -PathValue $binaryPathValue -BaseDir $configDir

    $stateDirValue = if ($runtime) { Get-RoodoxOptionalProperty -Object $runtime -Name "state_dir" } else { $null }
    if ([string]::IsNullOrWhiteSpace([string]$stateDirValue)) { $stateDirValue = Get-RoodoxDataPathDefault -DataRoot $dataRootValue -Fallback "runtime" }
    $stateDir = Resolve-RoodoxPath -PathValue $stateDirValue -BaseDir $configDir

    $pidFileValue = if ($runtime) { Get-RoodoxOptionalProperty -Object $runtime -Name "pid_file" } else { $null }
    if ([string]::IsNullOrWhiteSpace([string]$pidFileValue) -or (Test-RoodoxConfigPathEquals -Left ([string]$pidFileValue) -Right "runtime/roodox_server.pid")) {
        $pidFileValue = Join-RoodoxConfigPath -Base ([string]$stateDirValue) -Relative "roodox_server.pid"
    }
    $pidFile = Resolve-RoodoxPath -PathValue $pidFileValue -BaseDir $configDir

    $logDirValue = if ($runtime) { Get-RoodoxOptionalProperty -Object $runtime -Name "log_dir" } else { $null }
    if ([string]::IsNullOrWhiteSpace([string]$logDirValue) -or (Test-RoodoxConfigPathEquals -Left ([string]$logDirValue) -Right "runtime/logs")) {
        $logDirValue = Join-RoodoxConfigPath -Base ([string]$stateDirValue) -Relative "logs"
    }
    $logDir = Resolve-RoodoxPath -PathValue $logDirValue -BaseDir $configDir

    $stdoutLogName = if ($runtime) { [string](Get-RoodoxOptionalProperty -Object $runtime -Name "stdout_log_name") } else { "" }
    if ([string]::IsNullOrWhiteSpace($stdoutLogName)) { $stdoutLogName = "server.stdout.log" }

    $stderrLogName = if ($runtime) { [string](Get-RoodoxOptionalProperty -Object $runtime -Name "stderr_log_name") } else { "" }
    if ([string]::IsNullOrWhiteSpace($stderrLogName)) { $stderrLogName = "server.stderr.log" }

    $serviceConfig = if ($runtime) { Get-RoodoxOptionalProperty -Object $runtime -Name "windows_service" } else { $null }
    $serviceName = if ($serviceConfig) { [string](Get-RoodoxOptionalProperty -Object $serviceConfig -Name "name") } else { "" }
    if ([string]::IsNullOrWhiteSpace($serviceName)) { $serviceName = "RoodoxServer" }
    $serviceDisplayName = if ($serviceConfig) { [string](Get-RoodoxOptionalProperty -Object $serviceConfig -Name "display_name") } else { "" }
    if ([string]::IsNullOrWhiteSpace($serviceDisplayName)) { $serviceDisplayName = "Roodox Server" }
    $serviceDescription = if ($serviceConfig) { [string](Get-RoodoxOptionalProperty -Object $serviceConfig -Name "description") } else { "" }
    if ([string]::IsNullOrWhiteSpace($serviceDescription)) { $serviceDescription = "Roodox gRPC server" }
    $serviceStartType = if ($serviceConfig) { [string](Get-RoodoxOptionalProperty -Object $serviceConfig -Name "start_type") } else { "" }
    if ([string]::IsNullOrWhiteSpace($serviceStartType)) { $serviceStartType = "auto" }
    $dbPathValue = Get-RoodoxOptionalProperty -Object $config -Name "db_path"
    if ([string]::IsNullOrWhiteSpace([string]$dbPathValue) -or ((-not [string]::IsNullOrWhiteSpace($dataRootValue)) -and (Test-RoodoxConfigPathEquals -Left ([string]$dbPathValue) -Right "roodox.db"))) { $dbPathValue = Get-RoodoxDataPathDefault -DataRoot $dataRootValue -Fallback "roodox.db" }
    $dbPath = Resolve-RoodoxPath -PathValue $dbPathValue -BaseDir $configDir
    $rootDirValue = [string](Get-RoodoxOptionalProperty -Object $config -Name "root_dir")
    if ([string]::IsNullOrWhiteSpace($rootDirValue)) { $rootDirValue = "share" }
    $rootDir = Resolve-RoodoxPath -PathValue $rootDirValue -BaseDir $configDir
    $databaseConfig = Get-RoodoxOptionalProperty -Object $config -Name "database"
    $backupDirValue = if ($databaseConfig) { Get-RoodoxOptionalProperty -Object $databaseConfig -Name "backup_dir" } else { $null }
    if ([string]::IsNullOrWhiteSpace([string]$backupDirValue) -or ((-not [string]::IsNullOrWhiteSpace($dataRootValue)) -and (Test-RoodoxConfigPathEquals -Left ([string]$backupDirValue) -Right "backups"))) { $backupDirValue = Get-RoodoxDataPathDefault -DataRoot $dataRootValue -Fallback "backups" }
    $backupDir = Resolve-RoodoxPath -PathValue $backupDirValue -BaseDir $configDir
    $tlsCertPathValue = [string](Get-RoodoxOptionalProperty -Object $config -Name "tls_cert_path")
    if ([string]::IsNullOrWhiteSpace($tlsCertPathValue) -or ((-not [string]::IsNullOrWhiteSpace($dataRootValue)) -and (Test-RoodoxConfigPathEquals -Left $tlsCertPathValue -Right "certs/roodox-server-cert.pem"))) { $tlsCertPathValue = Get-RoodoxDataPathDefault -DataRoot $dataRootValue -Fallback "certs/roodox-server-cert.pem" }
    $tlsKeyPathValue = [string](Get-RoodoxOptionalProperty -Object $config -Name "tls_key_path")
    if ([string]::IsNullOrWhiteSpace($tlsKeyPathValue) -or ((-not [string]::IsNullOrWhiteSpace($dataRootValue)) -and (Test-RoodoxConfigPathEquals -Left $tlsKeyPathValue -Right "certs/roodox-server-key.pem"))) { $tlsKeyPathValue = Get-RoodoxDataPathDefault -DataRoot $dataRootValue -Fallback "certs/roodox-server-key.pem" }
    $tlsCertPath = Resolve-RoodoxPath -PathValue $tlsCertPathValue -BaseDir $configDir
    $tlsKeyPath = Resolve-RoodoxPath -PathValue $tlsKeyPathValue -BaseDir $configDir
    $tlsRootCertPath = Join-Path (Split-Path -Parent $tlsCertPath) "roodox-ca-cert.pem"
    $tlsRootKeyPath = Join-Path (Split-Path -Parent $tlsCertPath) "roodox-ca-key.pem"
    $releaseDir = Join-Path $stateDir "releases"

    [pscustomobject]@{
        RepoRoot           = $repoRoot
        ConfigPath         = $resolvedConfigPath
        ConfigDir          = $configDir
        DataRoot           = $dataRootValue
        BinaryPath         = $binaryPath
        StateDir           = $stateDir
        PIDFile            = $pidFile
        LogDir             = $logDir
        StdoutLogPath      = Join-Path $logDir $stdoutLogName
        StderrLogPath      = Join-Path $logDir $stderrLogName
        RootDir            = $rootDir
        DBPath             = $dbPath
        BackupDir          = $backupDir
        TLSCertPath        = $tlsCertPath
        TLSKeyPath         = $tlsKeyPath
        TLSRootCertPath    = $tlsRootCertPath
        TLSRootKeyPath     = $tlsRootKeyPath
        ReleaseDir         = $releaseDir
        ServiceName        = $serviceName
        ServiceDisplayName = $serviceDisplayName
        ServiceDescription = $serviceDescription
        ServiceStartType   = $serviceStartType
    }
}

function Ensure-RoodoxRuntimeDirectories {
    param(
        [psobject]$Layout
    )

    $dirs = @(
        $Layout.StateDir,
        (Split-Path -Parent $Layout.PIDFile),
        $Layout.LogDir,
        $Layout.ReleaseDir
    )
    foreach ($dir in $dirs) {
        if ([string]::IsNullOrWhiteSpace($dir)) {
            continue
        }
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
}

function Read-RoodoxPid {
    param(
        [string]$PIDFile
    )

    if (-not (Test-Path -LiteralPath $PIDFile)) {
        return $null
    }

    $raw = (Get-Content -LiteralPath $PIDFile -Raw).Trim()
    if ([string]::IsNullOrWhiteSpace($raw)) {
        return $null
    }

    $pidValue = 0
    if (-not [int]::TryParse($raw, [ref]$pidValue)) {
        return $null
    }
    return $pidValue
}

function Remove-RoodoxPidFile {
    param(
        [string]$PIDFile
    )

    if (Test-Path -LiteralPath $PIDFile) {
        Remove-Item -LiteralPath $PIDFile -Force
    }
}

function Write-RoodoxPidFile {
    param(
        [psobject]$Layout,
        [System.Diagnostics.Process]$Process
    )

    Set-Content -LiteralPath $Layout.PIDFile -Value ([string]$Process.Id) -Encoding ascii
}

function Get-RoodoxManagedProcess {
    param(
        [psobject]$Layout,
        [switch]$CleanStalePID
    )

    $pidValue = Read-RoodoxPid -PIDFile $Layout.PIDFile
    if (-not $pidValue) {
        return $null
    }

    try {
        $proc = Get-Process -Id $pidValue -ErrorAction Stop
    }
    catch {
        if ($CleanStalePID) {
            Remove-RoodoxPidFile -PIDFile $Layout.PIDFile
        }
        return $null
    }

    $procPath = ""
    try {
        $procPath = [string]$proc.Path
    }
    catch {
        $procPath = ""
    }
    if ($procPath -and ([System.IO.Path]::GetFullPath($procPath) -ne $Layout.BinaryPath)) {
        if ($CleanStalePID) {
            Remove-RoodoxPidFile -PIDFile $Layout.PIDFile
        }
        return $null
    }
    return $proc
}

function Get-RoodoxServiceProcess {
    param(
        [psobject]$Layout,
        $Service
    )

    if ($null -eq $Service) {
        $Service = Get-RoodoxWindowsService -Layout $Layout
    }
    if (-not $Service -or $Service.State -ne "Running") {
        return $null
    }

    $pidValue = 0
    try {
        $pidValue = [int]$Service.ProcessId
    }
    catch {
        $pidValue = 0
    }
    if ($pidValue -le 0) {
        return $null
    }

    try {
        $proc = Get-Process -Id $pidValue -ErrorAction Stop
    }
    catch {
        return $null
    }

    $procPath = ""
    try {
        $procPath = [string]$proc.Path
    }
    catch {
        $procPath = ""
    }
    if ($procPath -and ([System.IO.Path]::GetFullPath($procPath) -ne $Layout.BinaryPath)) {
        return $null
    }
    return $proc
}

function Get-RoodoxBinaryProcesses {
    param(
        [psobject]$Layout,
        [int[]]$ExcludeProcessIds = @()
    )

    $processName = [System.IO.Path]::GetFileNameWithoutExtension($Layout.BinaryPath)
    $candidates = @(Get-Process -Name $processName -ErrorAction SilentlyContinue)
    $matches = New-Object System.Collections.Generic.List[System.Diagnostics.Process]
    foreach ($candidate in $candidates) {
        if ($ExcludeProcessIds -contains $candidate.Id) {
            continue
        }
        $candidatePath = ""
        try {
            $candidatePath = [string]$candidate.Path
        }
        catch {
            $candidatePath = ""
        }
        if (-not $candidatePath) {
            $matches.Add($candidate)
            continue
        }
        if ([System.IO.Path]::GetFullPath($candidatePath) -ne $Layout.BinaryPath) {
            continue
        }
        $matches.Add($candidate)
    }
    return @($matches)
}

function Get-RoodoxWindowsService {
    param(
        [psobject]$Layout
    )

    return Get-CimInstance Win32_Service -Filter "Name = '$($Layout.ServiceName)'" -ErrorAction SilentlyContinue
}

function Wait-RoodoxServiceStatus {
    param(
        [string]$ServiceName,
        [ValidateSet("Running", "Stopped")]
        [string]$Status,
        [int]$TimeoutSeconds = 30
    )

    $service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if (-not $service) {
        throw "windows service not found: $ServiceName"
    }
    if ($service.Status.ToString() -ne $Status) {
        $service.WaitForStatus($Status, [TimeSpan]::FromSeconds([Math]::Max($TimeoutSeconds, 1)))
    }
}

function Wait-RoodoxProcessExit {
    param(
        [int]$ProcessId,
        [int]$TimeoutSeconds = 30
    )

    if ($ProcessId -le 0) {
        return
    }

    $deadline = (Get-Date).AddSeconds([Math]::Max($TimeoutSeconds, 1))
    while ((Get-Date) -lt $deadline) {
        if ($null -eq (Get-Process -Id $ProcessId -ErrorAction SilentlyContinue)) {
            return
        }
        Start-Sleep -Milliseconds 250
    }

    throw "process $ProcessId did not exit within $TimeoutSeconds seconds"
}

function Get-RoodoxRuntimeMode {
    param(
        [psobject]$Layout
    )

    $service = Get-RoodoxWindowsService -Layout $Layout
    $serviceProcess = Get-RoodoxServiceProcess -Layout $Layout -Service $service
    $managed = Get-RoodoxManagedProcess -Layout $Layout -CleanStalePID
    $excludeProcessIds = @()
    if ($serviceProcess) {
        $excludeProcessIds += $serviceProcess.Id
    }
    if ($managed -and ($excludeProcessIds -notcontains $managed.Id)) {
        $excludeProcessIds += $managed.Id
    }
    $unmanaged = @(Get-RoodoxBinaryProcesses -Layout $Layout -ExcludeProcessIds $excludeProcessIds)

    [pscustomobject]@{
        ServiceInstalled = ($null -ne $service)
        ServiceRunning = ($service -and $service.State -eq "Running")
        ServiceProcessRunning = ($null -ne $serviceProcess)
        ServiceProcess = $serviceProcess
        ManagedProcessRunning = ($null -ne $managed)
        ManagedProcess = $managed
        UnmanagedProcesses = @($unmanaged)
        RestartMode = if ($service -and $service.State -eq "Running") { "service" } elseif ($managed) { "process" } else { "none" }
    }
}

function Get-RoodoxDeployableFiles {
    param(
        [psobject]$Layout
    )

    return @(
        $Layout.BinaryPath,
        $Layout.ConfigPath,
        $Layout.TLSCertPath,
        $Layout.TLSKeyPath,
        $Layout.TLSRootCertPath,
        $Layout.TLSRootKeyPath
    )
}

function New-RoodoxReleaseSnapshot {
    param(
        [psobject]$Layout,
        [string]$Label
    )

    Ensure-RoodoxRuntimeDirectories -Layout $Layout
    if ([string]::IsNullOrWhiteSpace($Label)) {
        $Label = (Get-Date).ToString("yyyyMMdd-HHmmss")
    }

    $snapshotDir = Join-Path $Layout.ReleaseDir $Label
    if (Test-Path -LiteralPath $snapshotDir) {
        throw "release snapshot already exists: $snapshotDir"
    }

    New-Item -ItemType Directory -Force -Path $snapshotDir | Out-Null
    $manifest = [ordered]@{
        label = $Label
        created_at_utc = (Get-Date).ToUniversalTime().ToString("o")
        config_path = $Layout.ConfigPath
        binary_path = $Layout.BinaryPath
        files = @()
    }

    foreach ($path in Get-RoodoxDeployableFiles -Layout $Layout) {
        if (-not (Test-Path -LiteralPath $path)) {
            continue
        }
        $target = Join-Path $snapshotDir ([System.IO.Path]::GetFileName($path))
        Copy-Item -LiteralPath $path -Destination $target -Force
        $hash = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash
        $manifest.files += [ordered]@{
            source = $path
            snapshot = $target
            sha256 = $hash
        }
    }

    $manifestPath = Join-Path $snapshotDir "manifest.json"
    $manifest | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $manifestPath -Encoding utf8
    return [pscustomobject]@{
        Label = $Label
        SnapshotDir = $snapshotDir
        ManifestPath = $manifestPath
    }
}

function Get-RoodoxReleaseSnapshots {
    param(
        [psobject]$Layout
    )

    if (-not (Test-Path -LiteralPath $Layout.ReleaseDir)) {
        return @()
    }

    return @(Get-ChildItem -LiteralPath $Layout.ReleaseDir -Directory | Sort-Object Name -Descending)
}

function Restore-RoodoxReleaseSnapshot {
    param(
        [psobject]$Layout,
        [string]$SnapshotDir
    )

    if ([string]::IsNullOrWhiteSpace($SnapshotDir)) {
        throw "snapshot directory is required"
    }
    if (-not (Test-Path -LiteralPath $SnapshotDir)) {
        throw "snapshot directory not found: $SnapshotDir"
    }

    $map = [ordered]@{
        "roodox_server.exe" = $Layout.BinaryPath
        ([System.IO.Path]::GetFileName($Layout.ConfigPath)) = $Layout.ConfigPath
        ([System.IO.Path]::GetFileName($Layout.TLSCertPath)) = $Layout.TLSCertPath
        ([System.IO.Path]::GetFileName($Layout.TLSKeyPath)) = $Layout.TLSKeyPath
        ([System.IO.Path]::GetFileName($Layout.TLSRootCertPath)) = $Layout.TLSRootCertPath
        ([System.IO.Path]::GetFileName($Layout.TLSRootKeyPath)) = $Layout.TLSRootKeyPath
    }

    foreach ($name in $map.Keys) {
        $source = Join-Path $SnapshotDir $name
        if (-not (Test-Path -LiteralPath $source)) {
            continue
        }
        $target = $map[$name]
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target) | Out-Null
        Copy-RoodoxFileWithRetry -SourcePath $source -DestinationPath $target
    }
}

function Copy-RoodoxFileWithRetry {
    param(
        [string]$SourcePath,
        [string]$DestinationPath,
        [int]$TimeoutSeconds = 30
    )

    $deadline = (Get-Date).AddSeconds([Math]::Max($TimeoutSeconds, 1))
    while ($true) {
        try {
            Copy-Item -LiteralPath $SourcePath -Destination $DestinationPath -Force
            return
        }
        catch [System.IO.IOException] {
            if ((Get-Date) -ge $deadline) {
                throw
            }
            Start-Sleep -Milliseconds 250
        }
    }
}

function Get-RoodoxServerSourceWriteTimeUtc {
    param(
        [psobject]$Layout
    )

    $candidates = New-Object System.Collections.Generic.List[System.DateTime]
    $markerFiles = @(
        (Join-Path $Layout.RepoRoot "go.mod"),
        (Join-Path $Layout.RepoRoot "go.sum")
    )
    foreach ($path in $markerFiles) {
        if (Test-Path -LiteralPath $path) {
            $candidates.Add((Get-Item -LiteralPath $path).LastWriteTimeUtc)
        }
    }

    foreach ($dirName in @("cmd", "internal", "proto")) {
        $dir = Join-Path $Layout.RepoRoot $dirName
        if (-not (Test-Path -LiteralPath $dir)) {
            continue
        }
        $latest = Get-ChildItem -LiteralPath $dir -Recurse -File -ErrorAction SilentlyContinue |
            Where-Object { $_.Extension -in @(".go", ".proto") } |
            Sort-Object LastWriteTimeUtc -Descending |
            Select-Object -First 1
        if ($latest) {
            $candidates.Add($latest.LastWriteTimeUtc)
        }
    }

    if ($candidates.Count -eq 0) {
        return $null
    }
    return ($candidates | Sort-Object -Descending | Select-Object -First 1)
}

function Test-RoodoxServerBinaryStale {
    param(
        [psobject]$Layout
    )

    if (-not (Test-Path -LiteralPath $Layout.BinaryPath)) {
        return $true
    }

    $sourceWriteTime = Get-RoodoxServerSourceWriteTimeUtc -Layout $Layout
    if ($null -eq $sourceWriteTime) {
        return $false
    }

    $binaryWriteTime = (Get-Item -LiteralPath $Layout.BinaryPath).LastWriteTimeUtc
    return ($binaryWriteTime -lt $sourceWriteTime)
}

function Ensure-RoodoxServerBinary {
    param(
        [psobject]$Layout,
        [switch]$BuildIfMissing,
        [switch]$Rebuild,
        [switch]$RebuildIfStale
    )

    $binaryExists = (Test-Path -LiteralPath $Layout.BinaryPath)
    $shouldRebuild = $Rebuild
    if (-not $shouldRebuild -and $binaryExists -and $RebuildIfStale) {
        $shouldRebuild = Test-RoodoxServerBinaryStale -Layout $Layout
    }

    if ($binaryExists -and -not $shouldRebuild) {
        return
    }
    if (-not $binaryExists -and -not $BuildIfMissing -and -not $shouldRebuild) {
        throw "server binary not found: $($Layout.BinaryPath)"
    }

    Ensure-RoodoxRuntimeDirectories -Layout $Layout
    $tempBinary = Join-Path $Layout.StateDir ("roodox_server.build." + [System.Guid]::NewGuid().ToString("N") + ".exe")
    Push-Location $Layout.RepoRoot
    try {
        $goCache = Join-Path $Layout.StateDir ".gocache"
        $goModCache = Join-Path $Layout.StateDir ".gomodcache"
        New-Item -ItemType Directory -Force -Path $goCache, $goModCache | Out-Null
        $previousGoCache = $env:GOCACHE
        $previousGoModCache = $env:GOMODCACHE
        $env:GOCACHE = $goCache
        $env:GOMODCACHE = $goModCache
        & go build -o $tempBinary ./cmd/roodox_server
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit code $LASTEXITCODE"
        }
        Copy-RoodoxFileWithRetry -SourcePath $tempBinary -DestinationPath $Layout.BinaryPath
    }
    finally {
        if (Test-Path -LiteralPath $tempBinary) {
            Remove-Item -LiteralPath $tempBinary -Force -ErrorAction SilentlyContinue
        }
        if ($null -eq $previousGoCache) {
            Remove-Item Env:GOCACHE -ErrorAction SilentlyContinue
        } else {
            $env:GOCACHE = $previousGoCache
        }
        if ($null -eq $previousGoModCache) {
            Remove-Item Env:GOMODCACHE -ErrorAction SilentlyContinue
        } else {
            $env:GOMODCACHE = $previousGoModCache
        }
        Pop-Location
    }
}

function Invoke-RoodoxAdminBinary {
    param(
        [psobject]$Layout,
        [string[]]$ArgumentList,
        [switch]$BuildIfMissing,
        [switch]$Rebuild
    )

    $binaryPath = $Layout.BinaryPath
    $tempBinary = $null

    if ($Rebuild -or (-not (Test-Path -LiteralPath $binaryPath) -and $BuildIfMissing)) {
        Ensure-RoodoxRuntimeDirectories -Layout $Layout
        $tempBinary = Join-Path $Layout.StateDir ("roodox_server.admin." + [System.Guid]::NewGuid().ToString("N") + ".exe")
        Push-Location $Layout.RepoRoot
        try {
            $goCache = Join-Path $Layout.StateDir ".gocache"
            $goModCache = Join-Path $Layout.StateDir ".gomodcache"
            New-Item -ItemType Directory -Force -Path $goCache, $goModCache | Out-Null
            $previousGoCache = $env:GOCACHE
            $previousGoModCache = $env:GOMODCACHE
            $env:GOCACHE = $goCache
            $env:GOMODCACHE = $goModCache
            & go build -o $tempBinary ./cmd/roodox_server
            if ($LASTEXITCODE -ne 0) {
                throw "go build failed with exit code $LASTEXITCODE"
            }
        }
        finally {
            if ($null -eq $previousGoCache) {
                Remove-Item Env:GOCACHE -ErrorAction SilentlyContinue
            } else {
                $env:GOCACHE = $previousGoCache
            }
            if ($null -eq $previousGoModCache) {
                Remove-Item Env:GOMODCACHE -ErrorAction SilentlyContinue
            } else {
                $env:GOMODCACHE = $previousGoModCache
            }
            Pop-Location
        }
        $binaryPath = $tempBinary
    } elseif (-not (Test-Path -LiteralPath $binaryPath)) {
        throw "server binary not found: $binaryPath"
    }

    try {
        return & $binaryPath @ArgumentList
    }
    finally {
        if ($tempBinary -and (Test-Path -LiteralPath $tempBinary)) {
            try {
                Remove-Item -LiteralPath $tempBinary -Force -ErrorAction Stop
            }
            catch {
            }
        }
    }
}

function Invoke-RoodoxScript {
    param(
        [string]$Path,
        [object[]]$ArgumentList = @(),
        [string]$FailureMessage = ""
    )

    $global:LASTEXITCODE = 0
    $output = & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $Path @ArgumentList
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        if ([string]::IsNullOrWhiteSpace($FailureMessage)) {
            $FailureMessage = "script failed: $Path"
        }
        throw "$FailureMessage (exit code $exitCode)"
    }
    return $output
}

function Test-RoodoxAdministrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

