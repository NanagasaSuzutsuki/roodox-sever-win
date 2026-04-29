param(
    [Parameter(Mandatory = $true)]
    [string]$ConfigPath,
    [Parameter(Mandatory = $true)]
    [string]$ConfigTemplatePath,
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath,
    [Parameter(Mandatory = $true)]
    [string]$BootstrapPath,
    [Parameter(Mandatory = $true)]
    [string]$ProjectRoot,
    [Parameter(Mandatory = $true)]
    [string]$DataRoot
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Ensure-ObjectProperty {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Object,
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [Parameter(Mandatory = $true)]
        [object]$DefaultValue
    )

    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) {
        $Object | Add-Member -NotePropertyName $Name -NotePropertyValue $DefaultValue
        $property = $Object.PSObject.Properties[$Name]
    }
    if ($null -eq $property.Value) {
        $property.Value = $DefaultValue
    }
    return $property.Value
}

function Set-ObjectProperty {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Object,
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [Parameter(Mandatory = $true)]
        [AllowNull()]
        [object]$Value
    )

    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) {
        $Object | Add-Member -NotePropertyName $Name -NotePropertyValue $Value
    }
    else {
        $property.Value = $Value
    }
}

function New-RoodoxRandomSecret {
    $bytes = New-Object byte[] 32
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $rng.GetBytes($bytes)
    }
    finally {
        $rng.Dispose()
    }

    return ([Convert]::ToBase64String($bytes)).TrimEnd("=")
}

function Write-Utf8NoBomFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$Text
    )

    $parent = Split-Path -Parent $Path
    if (-not [string]::IsNullOrWhiteSpace($parent)) {
        New-Item -ItemType Directory -Force -Path $parent | Out-Null
    }

    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $Text, $utf8NoBom)
}

if (-not (Test-Path -LiteralPath $ConfigPath)) {
    if (-not (Test-Path -LiteralPath $ConfigTemplatePath)) {
        throw "config template not found: $ConfigTemplatePath"
    }

    $configParent = Split-Path -Parent $ConfigPath
    if (-not [string]::IsNullOrWhiteSpace($configParent)) {
        New-Item -ItemType Directory -Force -Path $configParent | Out-Null
    }
    Copy-Item -LiteralPath $ConfigTemplatePath -Destination $ConfigPath -Force
}

$raw = Get-Content -LiteralPath $ConfigPath -Raw
$config = $raw | ConvertFrom-Json
if ($null -eq $config) {
    $config = [pscustomobject]@{}
}

$runtime = Ensure-ObjectProperty -Object $config -Name "runtime" -DefaultValue ([pscustomobject]@{})
$windowsService = Ensure-ObjectProperty -Object $runtime -Name "windows_service" -DefaultValue ([pscustomobject]@{})

Set-ObjectProperty -Object $config -Name "data_root" -Value $DataRoot
Set-ObjectProperty -Object $runtime -Name "binary_path" -Value $BinaryPath

if ([string]::IsNullOrWhiteSpace([string]$runtime.state_dir)) {
    Set-ObjectProperty -Object $runtime -Name "state_dir" -Value "runtime"
}
if ([string]::IsNullOrWhiteSpace([string]$runtime.pid_file)) {
    Set-ObjectProperty -Object $runtime -Name "pid_file" -Value "runtime/roodox_server.pid"
}
if ([string]::IsNullOrWhiteSpace([string]$runtime.log_dir)) {
    Set-ObjectProperty -Object $runtime -Name "log_dir" -Value "runtime/logs"
}
if ([string]::IsNullOrWhiteSpace([string]$runtime.stdout_log_name)) {
    Set-ObjectProperty -Object $runtime -Name "stdout_log_name" -Value "server.stdout.log"
}
if ([string]::IsNullOrWhiteSpace([string]$runtime.stderr_log_name)) {
    Set-ObjectProperty -Object $runtime -Name "stderr_log_name" -Value "server.stderr.log"
}
if ([string]::IsNullOrWhiteSpace([string]$windowsService.name)) {
    Set-ObjectProperty -Object $windowsService -Name "name" -Value "RoodoxServer"
}
if ([string]::IsNullOrWhiteSpace([string]$windowsService.display_name)) {
    Set-ObjectProperty -Object $windowsService -Name "display_name" -Value "Roodox Server"
}
if ([string]::IsNullOrWhiteSpace([string]$windowsService.description)) {
    Set-ObjectProperty -Object $windowsService -Name "description" -Value "Roodox gRPC server"
}
if ([string]::IsNullOrWhiteSpace([string]$windowsService.start_type)) {
    Set-ObjectProperty -Object $windowsService -Name "start_type" -Value "auto"
}

if ([string]::IsNullOrWhiteSpace([string]$config.root_dir)) {
    Set-ObjectProperty -Object $config -Name "root_dir" -Value "share"
}

$secret = [string]$config.shared_secret
if ([string]::IsNullOrWhiteSpace($secret) -or $secret -eq "replace-with-a-long-random-secret") {
    Set-ObjectProperty -Object $config -Name "shared_secret" -Value (New-RoodoxRandomSecret)
}

New-Item -ItemType Directory -Force -Path $DataRoot | Out-Null
foreach ($dir in @(
    (Join-Path $DataRoot "share"),
    (Join-Path $DataRoot "artifacts"),
    (Join-Path $DataRoot "artifacts/handoff"),
    (Join-Path $DataRoot "runtime"),
    (Join-Path $DataRoot "runtime/logs"),
    (Join-Path $DataRoot "backups"),
    (Join-Path $DataRoot "certs")
)) {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
}

Write-Utf8NoBomFile -Path $ConfigPath -Text ($config | ConvertTo-Json -Depth 12)

$bootstrap = [ordered]@{
    project_root = $ProjectRoot
    config_path  = $ConfigPath
}
Write-Utf8NoBomFile -Path $BootstrapPath -Text ($bootstrap | ConvertTo-Json)
