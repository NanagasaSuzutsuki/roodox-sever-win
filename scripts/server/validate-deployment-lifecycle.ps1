param(
    [string]$ConfigPath = "testdata/deployment-smoke/roodox-smoke.config.json",
    [switch]$Rebuild,
    [switch]$KeepArtifacts
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "common.ps1")

function Get-FileHashOrEmpty {
    param(
        [string]$Path
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        return ""
    }
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash
}

function Remove-PathIfPresent {
    param(
        [string]$Path,
        [int]$TimeoutSeconds = 20
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }

    $deadline = (Get-Date).AddSeconds([Math]::Max($TimeoutSeconds, 1))
    while ($true) {
        try {
            if (Test-Path -LiteralPath $Path -PathType Container) {
                Get-ChildItem -LiteralPath $Path -Force -Recurse -ErrorAction SilentlyContinue | ForEach-Object {
                    try {
                        $_.Attributes = 'Normal'
                    }
                    catch {
                    }
                }
                try {
                    (Get-Item -LiteralPath $Path -Force -ErrorAction Stop).Attributes = 'Directory'
                }
                catch {
                }
                [System.IO.Directory]::Delete([System.IO.Path]::GetFullPath($Path), $true)
            } else {
                try {
                    (Get-Item -LiteralPath $Path -Force -ErrorAction Stop).Attributes = 'Normal'
                }
                catch {
                }
                [System.IO.File]::Delete([System.IO.Path]::GetFullPath($Path))
            }
            return
        }
        catch {
            if (-not (Test-Path -LiteralPath $Path)) {
                return
            }
            if ((Get-Date) -ge $deadline) {
                throw
            }
            Start-Sleep -Milliseconds 250
        }
    }
}

function Test-PathWithinBase {
    param(
        [string]$Path,
        [string]$BaseDir
    )

    if ([string]::IsNullOrWhiteSpace($Path) -or [string]::IsNullOrWhiteSpace($BaseDir)) {
        return $false
    }

    $fullPath = [System.IO.Path]::GetFullPath($Path).TrimEnd('\')
    $fullBase = [System.IO.Path]::GetFullPath($BaseDir).TrimEnd('\')
    return $fullPath.StartsWith($fullBase + "\", [System.StringComparison]::OrdinalIgnoreCase) -or
        $fullPath.Equals($fullBase, [System.StringComparison]::OrdinalIgnoreCase)
}

$layout = Get-RoodoxServerLayout -ConfigPath $ConfigPath
$baseDir = $layout.ConfigDir
$handoffDir = Join-Path $layout.StateDir "handoff"
$exportedCAPath = Join-Path $handoffDir "roodox-ca-cert.pem"

$cleanupTargets = @(
    (Split-Path -Parent $layout.BinaryPath),
    $layout.StateDir,
    (Split-Path -Parent $layout.TLSCertPath),
    $layout.RootDir,
    $layout.DBPath,
    ($layout.DBPath + "-wal"),
    ($layout.DBPath + "-shm"),
    ($layout.DBPath + ".lock")
)

foreach ($target in $cleanupTargets) {
    if (-not (Test-PathWithinBase -Path $target -BaseDir $baseDir)) {
        throw "cleanup target escapes smoke config dir: $target"
    }
    Remove-PathIfPresent -Path $target
}

try {
    $installArgs = @("-ConfigPath", $layout.ConfigPath, "-BuildIfMissing")
    if ($Rebuild) {
        $installArgs += "-Rebuild"
    }
    Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "install-deployment.ps1") -ArgumentList $installArgs -FailureMessage "install-deployment failed"

    $statusArgs = @("-ConfigPath", $layout.ConfigPath, "-RawJson")
    if ($Rebuild) {
        $statusArgs += "-Rebuild"
    }
    $initialStatusJson = Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "certificate-status.ps1") -ArgumentList $statusArgs -FailureMessage "certificate-status failed"
    $initialStatus = $initialStatusJson | ConvertFrom-Json
    if (-not $initialStatus.overall_valid) {
        throw "initial tls status is not valid"
    }

    $initialServerHash = Get-FileHashOrEmpty -Path $layout.TLSCertPath
    $initialRootHash = Get-FileHashOrEmpty -Path $layout.TLSRootCertPath

    $exportArgs = @("-ConfigPath", $layout.ConfigPath, "-DestinationPath", $exportedCAPath)
    if ($Rebuild) {
        $exportArgs += "-Rebuild"
    }
    Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "export-client-ca.ps1") -ArgumentList $exportArgs -FailureMessage "export-client-ca failed"
    $exportedRootHash = Get-FileHashOrEmpty -Path $exportedCAPath
    if ($initialRootHash -eq "" -or $initialRootHash -ne $exportedRootHash) {
        throw "exported client CA does not match current root CA"
    }

    $upgradeArgs = @("-ConfigPath", $layout.ConfigPath, "-RotateServerCert", "-BuildIfMissing")
    if ($Rebuild) {
        $upgradeArgs += "-Rebuild"
    }
    Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "upgrade-deployment.ps1") -ArgumentList $upgradeArgs -FailureMessage "upgrade-deployment failed"

    $rotatedServerHash = Get-FileHashOrEmpty -Path $layout.TLSCertPath
    $rotatedRootHash = Get-FileHashOrEmpty -Path $layout.TLSRootCertPath
    if ($rotatedServerHash -eq "" -or $rotatedServerHash -eq $initialServerHash) {
        throw "server certificate hash did not change after upgrade rotation"
    }
    if ($rotatedRootHash -ne $initialRootHash) {
        throw "root CA changed during leaf-only upgrade rotation"
    }

    Invoke-RoodoxScript -Path (Join-Path $PSScriptRoot "rollback-deployment.ps1") -ArgumentList @("-ConfigPath", $layout.ConfigPath, "-Latest") -FailureMessage "rollback-deployment failed"

    $rolledBackServerHash = Get-FileHashOrEmpty -Path $layout.TLSCertPath
    $rolledBackRootHash = Get-FileHashOrEmpty -Path $layout.TLSRootCertPath
    if ($rolledBackServerHash -ne $initialServerHash) {
        throw "server certificate hash did not return to pre-upgrade value after rollback"
    }
    if ($rolledBackRootHash -ne $initialRootHash) {
        throw "root CA hash changed after rollback"
    }

    $snapshots = @(Get-RoodoxReleaseSnapshots -Layout $layout)
    [pscustomobject]@{
        Success = $true
        ConfigPath = $layout.ConfigPath
        SnapshotCount = $snapshots.Count
        InitialServerHash = $initialServerHash
        RotatedServerHash = $rotatedServerHash
        RolledBackServerHash = $rolledBackServerHash
        RootCAHash = $initialRootHash
        ExportedCAPath = $exportedCAPath
    } | Format-List
}
finally {
    if (-not $KeepArtifacts) {
        foreach ($target in $cleanupTargets) {
            Remove-PathIfPresent -Path $target
        }
    }
}
