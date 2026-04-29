param(
    [string]$Version = "",
    [switch]$BuildMsi
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$workbenchCommonPath = Join-Path $repoRoot "scripts/workbench/common.ps1"
. $workbenchCommonPath

function Get-ReleaseVersion {
    param([string]$RepoRoot)

    $package = Get-Content -LiteralPath (Join-Path $RepoRoot "workbench/package.json") -Raw | ConvertFrom-Json
    return [string]$package.version
}

function Reset-Directory {
    param([string]$Path)

    if (Test-Path -LiteralPath $Path) {
        Remove-Item -LiteralPath $Path -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $Path | Out-Null
}

function Write-PortableBootstrap {
    param(
        [string]$Path,
        [string]$ProjectRoot,
        [string]$ConfigPath
    )

    $payload = [ordered]@{
        project_root = $ProjectRoot
        config_path  = $ConfigPath
    }
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, ($payload | ConvertTo-Json), $utf8NoBom)
}

function Write-ReleaseReadme {
    param(
        [string]$Path,
        [string]$Version,
        [string]$MsiFileName
    )

    $lines = @(
        "Roodox Windows portable release $Version",
        "",
        "Contents:",
        "- roodox_server.exe",
        "- roodox-workbench.exe",
        "- roodox.config.json",
        "- roodox.config.example.json",
        "- scripts\\server\\",
        "- docs\\",
        "- README.md / SECURITY.md / LICENSE"
    )
    if (-not [string]::IsNullOrWhiteSpace($MsiFileName)) {
        $lines += "- $MsiFileName"
    }
    $lines += @(
        "",
        "First-run checklist:",
        "1. Edit roodox.config.json.",
        "2. Replace shared_secret with a real random value.",
        "3. Run scripts\\server\\install-deployment.ps1 -StartAfterInstall or start the server manually.",
        "4. Use start-roodox-workbench.cmd to open the GUI.",
        "",
        "This bundle intentionally excludes live certs, runtime data, backups, and client handoff artifacts."
    )
    Set-Content -LiteralPath $Path -Value $lines -Encoding ascii
}

$resolvedVersion = $Version
if ([string]::IsNullOrWhiteSpace($resolvedVersion)) {
    $resolvedVersion = Get-ReleaseVersion -RepoRoot $repoRoot
}
if ([string]::IsNullOrWhiteSpace($resolvedVersion)) {
    throw "release version is empty"
}

$layout = Get-RoodoxWorkbenchLayout -ConfigPath (Join-Path $repoRoot "roodox.config.json")
$serverExe = Join-Path $repoRoot "roodox_server.exe"
$releaseRoot = Join-Path $repoRoot "artifacts/release"
$bundleName = "roodox-server-win-v$resolvedVersion-portable"
$bundleDir = Join-Path $releaseRoot $bundleName
$zipPath = Join-Path $releaseRoot ($bundleName + ".zip")

New-Item -ItemType Directory -Force -Path $releaseRoot | Out-Null

Push-Location $repoRoot
try {
    & go build -o $serverExe ./cmd/roodox_server
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

Invoke-RoodoxWorkbenchTauriBuild -Layout $layout -Mode $(if ($BuildMsi) { "msi" } else { "run" })

Reset-Directory -Path $bundleDir
New-Item -ItemType Directory -Force -Path (Join-Path $bundleDir "scripts"), (Join-Path $bundleDir "docs") | Out-Null

Copy-Item -LiteralPath $serverExe -Destination (Join-Path $bundleDir "roodox_server.exe") -Force
Copy-Item -LiteralPath $layout.ExecutablePath -Destination (Join-Path $bundleDir "roodox-workbench.exe") -Force
Copy-Item -LiteralPath (Join-Path $repoRoot "roodox.config.example.json") -Destination (Join-Path $bundleDir "roodox.config.example.json") -Force
Copy-Item -LiteralPath (Join-Path $repoRoot "roodox.config.example.json") -Destination (Join-Path $bundleDir "roodox.config.json") -Force
Copy-Item -LiteralPath (Join-Path $repoRoot "README.md") -Destination (Join-Path $bundleDir "README.md") -Force
Copy-Item -LiteralPath (Join-Path $repoRoot "SECURITY.md") -Destination (Join-Path $bundleDir "SECURITY.md") -Force
Copy-Item -LiteralPath (Join-Path $repoRoot "LICENSE") -Destination (Join-Path $bundleDir "LICENSE") -Force

Copy-Item -LiteralPath (Join-Path $repoRoot "scripts/server") -Destination (Join-Path $bundleDir "scripts/server") -Recurse -Force
Copy-Item -LiteralPath (Join-Path $repoRoot "docs/README.md") -Destination (Join-Path $bundleDir "docs/README.md") -Force
Copy-Item -LiteralPath (Join-Path $repoRoot "docs/OPERATIONS.md") -Destination (Join-Path $bundleDir "docs/OPERATIONS.md") -Force
Copy-Item -LiteralPath (Join-Path $repoRoot "docs/QA.md") -Destination (Join-Path $bundleDir "docs/QA.md") -Force
Copy-Item -LiteralPath (Join-Path $repoRoot "docs/PRIVACY_AUDIT.md") -Destination (Join-Path $bundleDir "docs/PRIVACY_AUDIT.md") -Force
Copy-Item -LiteralPath (Join-Path $repoRoot "docs/encyclopedia") -Destination (Join-Path $bundleDir "docs/encyclopedia") -Recurse -Force

Write-PortableBootstrap -Path (Join-Path $bundleDir "roodox-workbench.bootstrap.json") -ProjectRoot "." -ConfigPath "roodox.config.json"

$launcher = @'
@echo off
setlocal
start "" "%~dp0roodox-workbench.exe"
endlocal
'@
Set-Content -LiteralPath (Join-Path $bundleDir "start-roodox-workbench.cmd") -Value $launcher -Encoding ascii

$msiPath = $null
$msiName = ""
if ($BuildMsi) {
    $msiPath = Get-RoodoxWorkbenchLatestMsiPath -Layout $layout
    if ($null -eq $msiPath) {
        throw "MSI bundle not found under $($layout.BundleMsiDir)"
    }
    $msiName = [System.IO.Path]::GetFileName($msiPath)
    Copy-Item -LiteralPath $msiPath -Destination (Join-Path $bundleDir $msiName) -Force
}

Write-ReleaseReadme -Path (Join-Path $bundleDir "RELEASE.txt") -Version $resolvedVersion -MsiFileName $msiName

if (Test-Path -LiteralPath $zipPath) {
    Remove-Item -LiteralPath $zipPath -Force
}
Compress-Archive -Path (Join-Path $bundleDir "*") -DestinationPath $zipPath -Force

[pscustomobject]@{
    Version = $resolvedVersion
    BundleDir = $bundleDir
    ZipPath = $zipPath
    MsiPath = $msiPath
    ServerExe = Join-Path $bundleDir "roodox_server.exe"
    WorkbenchExe = Join-Path $bundleDir "roodox-workbench.exe"
    ConfigPath = Join-Path $bundleDir "roodox.config.json"
}
