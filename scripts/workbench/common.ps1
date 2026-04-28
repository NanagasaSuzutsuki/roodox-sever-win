Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Get-RoodoxWorkbenchRepoRoot {
    return Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
}

function Resolve-RoodoxWorkbenchConfigPath {
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

function Get-RoodoxWorkbenchLayout {
    param(
        [string]$ConfigPath
    )

    $repoRoot = Get-RoodoxWorkbenchRepoRoot
    $workbenchRoot = Join-Path $repoRoot "workbench"
    $tauriRoot = Join-Path $workbenchRoot "src-tauri"
    $targetReleaseDir = Join-Path $tauriRoot "target/release"
    $bundleDir = Join-Path $targetReleaseDir "bundle"
    $bundleMsiDir = Join-Path $bundleDir "msi"
    $artifactRoot = Join-Path $repoRoot "artifacts/workbench"
    $portableDir = Join-Path $artifactRoot "portable"
    $deliveryDir = Join-Path $artifactRoot "delivery"
    $handoffSourceDir = Join-Path $repoRoot "artifacts/handoff"
    $configFullPath = Resolve-RoodoxWorkbenchConfigPath -ConfigPath $ConfigPath -RepoRoot $repoRoot
    $bootstrapFileName = "roodox-workbench.bootstrap.json"

    [pscustomobject]@{
        RepoRoot             = $repoRoot
        WorkbenchRoot        = $workbenchRoot
        TauriRoot            = $tauriRoot
        TargetReleaseDir     = $targetReleaseDir
        BundleDir            = $bundleDir
        BundleMsiDir         = $bundleMsiDir
        ArtifactRoot         = $artifactRoot
        PortableDir          = $portableDir
        DeliveryDir          = $deliveryDir
        DeliveryPortableDir  = Join-Path $deliveryDir "portable"
        DeliveryHandoffDir   = Join-Path $deliveryDir "handoff"
        DeliveryReadmePath   = Join-Path $deliveryDir "README.txt"
        DeliveryZipPath      = Join-Path $artifactRoot "roodox-workbench-delivery.zip"
        HandoffSourceDir     = $handoffSourceDir
        ConfigPath           = $configFullPath
        ExecutablePath       = Join-Path $targetReleaseDir "roodox-workbench.exe"
        BuildMarkerPath      = Join-Path $targetReleaseDir ".roodox-workbench.tauri-build.stamp"
        BootstrapFileName    = $bootstrapFileName
        BootstrapPath        = Join-Path $targetReleaseDir $bootstrapFileName
        PortableExecutable   = Join-Path $portableDir "roodox-workbench.exe"
        PortableBootstrap    = Join-Path $portableDir $bootstrapFileName
        PortableLauncherPath = Join-Path $portableDir "start-roodox-workbench.cmd"
        PortableReadmePath   = Join-Path $portableDir "README.txt"
    }
}

function Write-RoodoxWorkbenchBootstrap {
    param(
        [psobject]$Layout,
        [string]$DestinationPath
    )

    if ([string]::IsNullOrWhiteSpace($DestinationPath)) {
        throw "bootstrap destination path is required"
    }

    $parent = Split-Path -Parent $DestinationPath
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }

    $payload = [ordered]@{
        project_root = $Layout.RepoRoot
        config_path  = $Layout.ConfigPath
    }
    $payload | ConvertTo-Json | Set-Content -LiteralPath $DestinationPath -Encoding utf8
}

function Get-RoodoxWorkbenchLatestMsiPath {
    param(
        [psobject]$Layout
    )

    if (-not (Test-Path -LiteralPath $Layout.BundleMsiDir)) {
        return $null
    }

    $item = Get-ChildItem -LiteralPath $Layout.BundleMsiDir -Filter *.msi -File |
        Sort-Object LastWriteTimeUtc -Descending |
        Select-Object -First 1
    if ($null -eq $item) {
        return $null
    }
    return $item.FullName
}

function Reset-RoodoxWorkbenchDirectory {
    param(
        [string]$Path
    )

    if ([string]::IsNullOrWhiteSpace($Path)) {
        throw "directory path is required"
    }
    if (Test-Path -LiteralPath $Path) {
        Remove-Item -LiteralPath $Path -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $Path | Out-Null
}

function Copy-RoodoxWorkbenchDirectoryContents {
    param(
        [string]$SourceDir,
        [string]$DestinationDir
    )

    if (-not (Test-Path -LiteralPath $SourceDir)) {
        throw "source directory not found: $SourceDir"
    }

    Reset-RoodoxWorkbenchDirectory -Path $DestinationDir
    Get-ChildItem -LiteralPath $SourceDir -Force | ForEach-Object {
        Copy-Item -LiteralPath $_.FullName -Destination $DestinationDir -Recurse -Force
    }
}

function Write-RoodoxWorkbenchDeliveryReadme {
    param(
        [psobject]$Layout,
        [string]$MsiFileName
    )

    $readme = @"
This delivery package contains the current GUI installer and the current client handoff materials.

Contents:
- $MsiFileName
- portable\
- handoff\

portable\ contains a repo-bound portable GUI build.
handoff\ contains the current CA certificate, access bundle, and client handoff documents.
"@
    Set-Content -LiteralPath $Layout.DeliveryReadmePath -Value $readme -Encoding ascii
}

function Invoke-RoodoxWorkbenchTauriBuild {
    param(
        [psobject]$Layout,
        [ValidateSet("run", "msi")]
        [string]$Mode = "run"
    )

    Push-Location $Layout.WorkbenchRoot
    try {
        if ($Mode -eq "msi") {
            & npm.cmd run tauri build -- --bundles msi
            if ($LASTEXITCODE -ne 0) {
                throw "npm.cmd run tauri build -- --bundles msi failed with exit code $LASTEXITCODE"
            }
        }
        else {
            & npm.cmd run tauri build -- --no-bundle
            if ($LASTEXITCODE -ne 0) {
                throw "npm.cmd run tauri build -- --no-bundle failed with exit code $LASTEXITCODE"
            }
        }
    }
    finally {
        Pop-Location
    }

    Write-RoodoxWorkbenchBootstrap -Layout $Layout -DestinationPath $Layout.BootstrapPath
    Set-Content -LiteralPath $Layout.BuildMarkerPath -Value ((Get-Date).ToUniversalTime().ToString("o")) -Encoding ascii
}

function Publish-RoodoxWorkbenchArtifacts {
    param(
        [psobject]$Layout,
        [string]$HandoffSourceDir = "",
        [switch]$IncludeMsi
    )

    if (-not (Test-Path -LiteralPath $Layout.ExecutablePath)) {
        throw "GUI executable not found: $($Layout.ExecutablePath)"
    }

    New-Item -ItemType Directory -Force -Path $Layout.ArtifactRoot, $Layout.PortableDir | Out-Null
    Copy-Item -LiteralPath $Layout.ExecutablePath -Destination $Layout.PortableExecutable -Force
    Write-RoodoxWorkbenchBootstrap -Layout $Layout -DestinationPath $Layout.PortableBootstrap

    $launcher = @'
@echo off
setlocal
start "" "%~dp0roodox-workbench.exe"
endlocal
'@
    Set-Content -LiteralPath $Layout.PortableLauncherPath -Value $launcher -Encoding ascii

    $readme = @'
This portable GUI build is bound to the current server checkout through roodox-workbench.bootstrap.json.

Use start-roodox-workbench.cmd to launch the GUI.
If the server config path changes, rebuild this portable package so the bootstrap file stays in sync.
'@
    Set-Content -LiteralPath $Layout.PortableReadmePath -Value $readme -Encoding ascii

    $msiOutputPath = $null
    if ($IncludeMsi) {
        $msiPath = Get-RoodoxWorkbenchLatestMsiPath -Layout $Layout
        if ($null -eq $msiPath) {
            throw "MSI bundle not found under $($Layout.BundleMsiDir)"
        }
        $msiOutputPath = Join-Path $Layout.ArtifactRoot ([System.IO.Path]::GetFileName($msiPath))
        Copy-Item -LiteralPath $msiPath -Destination $msiOutputPath -Force
    }

    Reset-RoodoxWorkbenchDirectory -Path $Layout.DeliveryDir
    Copy-RoodoxWorkbenchDirectoryContents -SourceDir $Layout.PortableDir -DestinationDir $Layout.DeliveryPortableDir
    if (-not [string]::IsNullOrWhiteSpace($HandoffSourceDir)) {
        Copy-RoodoxWorkbenchDirectoryContents -SourceDir $HandoffSourceDir -DestinationDir $Layout.DeliveryHandoffDir
    }
    if ($msiOutputPath) {
        Copy-Item -LiteralPath $msiOutputPath -Destination (Join-Path $Layout.DeliveryDir ([System.IO.Path]::GetFileName($msiOutputPath))) -Force
    }
    $msiFileName = ""
    if ($msiOutputPath) {
        $msiFileName = [System.IO.Path]::GetFileName($msiOutputPath)
    }
    Write-RoodoxWorkbenchDeliveryReadme -Layout $Layout -MsiFileName $msiFileName

    if (Test-Path -LiteralPath $Layout.DeliveryZipPath) {
        Remove-Item -LiteralPath $Layout.DeliveryZipPath -Force
    }
    Compress-Archive -Path (Join-Path $Layout.DeliveryDir "*") -DestinationPath $Layout.DeliveryZipPath -Force

    return [pscustomobject]@{
        ArtifactRoot       = $Layout.ArtifactRoot
        PortableDir        = $Layout.PortableDir
        PortableExecutable = $Layout.PortableExecutable
        PortableLauncher   = $Layout.PortableLauncherPath
        PortableBootstrap  = $Layout.PortableBootstrap
        MsiPath            = $msiOutputPath
        DeliveryDir        = $Layout.DeliveryDir
        DeliveryZip        = $Layout.DeliveryZipPath
        DeliveryHandoffDir = if ([string]::IsNullOrWhiteSpace($HandoffSourceDir)) { $null } else { $Layout.DeliveryHandoffDir }
    }
}
