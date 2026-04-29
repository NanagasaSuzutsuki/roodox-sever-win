param(
    [switch]$AutoInstall
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Find-ToolPath {
    param([Parameter(Mandatory = $true)][string]$Name)

    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if ($command -and $command.Source) {
        return $command.Source
    }

    $fileName = if ($Name.EndsWith(".exe")) { $Name } else { "$Name.exe" }
    foreach ($dir in @(
        "C:\Program Files\CMake\bin",
        "C:\Program Files (x86)\GnuWin32\bin"
    )) {
        $candidate = Join-Path $dir $fileName
        if (Test-Path -LiteralPath $candidate) {
            return $candidate
        }
    }

    return $null
}

function Test-WingetInstalled {
    return $null -ne (Get-Command winget.exe -ErrorAction SilentlyContinue)
}

$missing = @()
foreach ($tool in @(
    @{ Name = "cmake"; PackageId = "Kitware.CMake" },
    @{ Name = "make"; PackageId = "GnuWin32.Make" }
)) {
    if (-not (Find-ToolPath -Name $tool.Name)) {
        $missing += [pscustomobject]$tool
    }
}

if ($missing.Count -eq 0) {
    Write-Host "build tools already available"
    exit 0
}

if (-not $AutoInstall) {
    Write-Host ("missing build tools: " + (($missing | ForEach-Object Name) -join ", "))
    exit 0
}

if (-not (Test-WingetInstalled)) {
    Write-Warning "winget not found; skipping automatic build-tool installation"
    exit 0
}

foreach ($tool in $missing) {
    try {
        & winget install --id $tool.PackageId -e --accept-package-agreements --accept-source-agreements
        if ($LASTEXITCODE -ne 0) {
            Write-Warning "winget install failed for $($tool.PackageId) with exit code $LASTEXITCODE"
        }
    }
    catch {
        Write-Warning "winget install failed for $($tool.PackageId): $($_.Exception.Message)"
    }
}

exit 0
