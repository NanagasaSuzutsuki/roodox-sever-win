param(
    [string]$Version = "",
    [switch]$BuildMsi,
    [switch]$BuildInstaller
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
        [string]$MsiFileName,
        [string]$SetupFileName
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
    if (-not [string]::IsNullOrWhiteSpace($SetupFileName)) {
        $lines += "- $SetupFileName"
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

function Write-WorkbenchLauncher {
    param([string]$Path)

    $launcher = @'
@echo off
setlocal
start "" "%~dp0roodox-workbench.exe"
endlocal
'@
    Set-Content -LiteralPath $Path -Value $launcher -Encoding ascii
}

function Get-InnoSetupCompilerPath {
    $command = Get-Command iscc.exe -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }

    foreach ($path in @(
        "C:\Program Files (x86)\Inno Setup 6\ISCC.exe",
        "C:\Program Files\Inno Setup 6\ISCC.exe",
        (Join-Path $env:LOCALAPPDATA "Programs\Inno Setup 6\ISCC.exe")
    )) {
        if (Test-Path -LiteralPath $path) {
            return $path
        }
    }

    return $null
}

function New-InstallerStage {
    param(
        [string]$RepoRoot,
        [string]$ServerExe,
        [string]$WorkbenchExe,
        [string]$StageDir
    )

    $appDir = Join-Path $StageDir "app"
    Reset-Directory -Path $StageDir
    New-Item -ItemType Directory -Force -Path (Join-Path $appDir "scripts"), (Join-Path $appDir "docs") | Out-Null

    Copy-Item -LiteralPath $ServerExe -Destination (Join-Path $appDir "roodox_server.exe") -Force
    Copy-Item -LiteralPath $WorkbenchExe -Destination (Join-Path $appDir "roodox-workbench.exe") -Force
    Copy-Item -LiteralPath (Join-Path $RepoRoot "roodox.config.example.json") -Destination (Join-Path $appDir "roodox.config.example.json") -Force
    Copy-Item -LiteralPath (Join-Path $RepoRoot "README.md") -Destination (Join-Path $appDir "README.md") -Force
    Copy-Item -LiteralPath (Join-Path $RepoRoot "SECURITY.md") -Destination (Join-Path $appDir "SECURITY.md") -Force
    Copy-Item -LiteralPath (Join-Path $RepoRoot "LICENSE") -Destination (Join-Path $appDir "LICENSE") -Force

    Copy-Item -LiteralPath (Join-Path $RepoRoot "scripts/server") -Destination (Join-Path $appDir "scripts/server") -Recurse -Force
    Copy-Item -LiteralPath (Join-Path $RepoRoot "scripts/installer") -Destination (Join-Path $appDir "scripts/installer") -Recurse -Force
    Copy-Item -LiteralPath (Join-Path $RepoRoot "docs/README.md") -Destination (Join-Path $appDir "docs/README.md") -Force
    Copy-Item -LiteralPath (Join-Path $RepoRoot "docs/OPERATIONS.md") -Destination (Join-Path $appDir "docs/OPERATIONS.md") -Force
    Copy-Item -LiteralPath (Join-Path $RepoRoot "docs/QA.md") -Destination (Join-Path $appDir "docs/QA.md") -Force
    Copy-Item -LiteralPath (Join-Path $RepoRoot "docs/PRIVACY_AUDIT.md") -Destination (Join-Path $appDir "docs/PRIVACY_AUDIT.md") -Force
    Copy-Item -LiteralPath (Join-Path $RepoRoot "docs/encyclopedia") -Destination (Join-Path $appDir "docs/encyclopedia") -Recurse -Force

    Write-WorkbenchLauncher -Path (Join-Path $appDir "start-roodox-workbench.cmd")

    return [pscustomobject]@{
        StageDir = $StageDir
        AppDir = $appDir
    }
}

function Write-InnoSetupScript {
    param(
        [string]$Path,
        [string]$Version,
        [string]$AppSourceDir,
        [string]$OutputDir,
        [string]$OutputBaseFileName
    )

    $script = @"
#define MyAppName "Roodox Server"
#define MyAppVersion "$Version"
#define AppSourceDir "$AppSourceDir"
#define OutputDir "$OutputDir"
#define OutputBaseName "$OutputBaseFileName"

[Setup]
AppId={{7A8DF66B-59AA-4D1B-8A51-9777084FF57C}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher=NanagasaSuzutsuki
DefaultDirName={autopf}\Roodox Server
DefaultGroupName=Roodox
OutputDir={#OutputDir}
OutputBaseFilename={#OutputBaseName}
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=admin
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
DisableProgramGroupPage=yes
UninstallDisplayIcon={app}\roodox-workbench.exe
CloseApplications=yes
RestartApplications=no
SetupLogging=yes

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Additional icons:"
Name: "installservice"; Description: "Install and start the Roodox Windows service"; GroupDescription: "Server startup:"; Flags: checkedonce

[Dirs]
Name: "{commonappdata}\Roodox"
Name: "{commonappdata}\Roodox\share"; Permissions: users-modify
Name: "{commonappdata}\Roodox\artifacts"
Name: "{commonappdata}\Roodox\artifacts\handoff"; Permissions: users-modify
Name: "{commonappdata}\Roodox\runtime"
Name: "{commonappdata}\Roodox\runtime\logs"
Name: "{commonappdata}\Roodox\backups"
Name: "{commonappdata}\Roodox\certs"

[Files]
Source: "{#AppSourceDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#AppSourceDir}\roodox.config.example.json"; DestDir: "{commonappdata}\Roodox"; DestName: "roodox.config.json"; Flags: onlyifdoesntexist; Permissions: users-modify
Source: "{#AppSourceDir}\roodox.config.example.json"; DestDir: "{commonappdata}\Roodox"; DestName: "roodox.config.example.json"; Flags: ignoreversion

[Icons]
Name: "{group}\Roodox Workbench"; Filename: "{app}\roodox-workbench.exe"
Name: "{group}\Roodox Data Folder"; Filename: "{commonappdata}\Roodox"
Name: "{group}\Roodox Documentation"; Filename: "{app}\docs"
Name: "{commondesktop}\Roodox Workbench"; Filename: "{app}\roodox-workbench.exe"; Tasks: desktopicon

[Run]
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\scripts\installer\initialize-installed-layout.ps1"" -ConfigPath ""{commonappdata}\Roodox\roodox.config.json"" -ConfigTemplatePath ""{app}\roodox.config.example.json"" -BinaryPath ""{app}\roodox_server.exe"" -BootstrapPath ""{app}\roodox-workbench.bootstrap.json"" -ProjectRoot ""{app}"" -DataRoot ""{commonappdata}\Roodox"""; Flags: runhidden waituntilterminated
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\scripts\server\uninstall-windows-service.ps1"" -ConfigPath ""{commonappdata}\Roodox\roodox.config.json"" -Force"; Flags: runhidden waituntilterminated; Check: ShouldRefreshExistingService
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\scripts\server\install-deployment.ps1"" -ConfigPath ""{commonappdata}\Roodox\roodox.config.json"" -AsService"; Flags: runhidden waituntilterminated; Check: ShouldRefreshExistingService
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\scripts\server\start-windows-service.ps1"" -ConfigPath ""{commonappdata}\Roodox\roodox.config.json"""; Flags: runhidden waituntilterminated; Check: ShouldRestartExistingService
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\scripts\server\install-deployment.ps1"" -ConfigPath ""{commonappdata}\Roodox\roodox.config.json"" -AsService -StartAfterInstall"; Flags: runhidden waituntilterminated; Tasks: installservice; Check: ShouldInstallFreshService
Filename: "{app}\roodox-workbench.exe"; Description: "Launch Roodox Workbench"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "powershell.exe"; Parameters: "-NoProfile -ExecutionPolicy Bypass -File ""{app}\scripts\server\uninstall-windows-service.ps1"" -ConfigPath ""{commonappdata}\Roodox\roodox.config.json"" -Force"; Flags: runhidden waituntilterminated skipifdoesntexist

[Code]
var
  ExistingServicePresent: Boolean;
  ExistingServiceWasRunning: Boolean;

function QueryServiceOutput(const ServiceName: string; const TempFile: string): Boolean;
var
  ResultCode: Integer;
begin
  if FileExists(TempFile) then
    DeleteFile(TempFile);
  Result := Exec(
    ExpandConstant('{cmd}'),
    '/C sc.exe query "' + ServiceName + '" > "' + TempFile + '" 2>&1',
    '',
    SW_HIDE,
    ewWaitUntilTerminated,
    ResultCode
  ) and (ResultCode = 0);
end;

function ServiceOutputContains(const ServiceName: string; const Needle: string): Boolean;
var
  TempFile: string;
  Content: AnsiString;
begin
  TempFile := ExpandConstant('{tmp}\roodox-service-query.txt');
  Result := False;
  if not QueryServiceOutput(ServiceName, TempFile) then
    exit;
  if not LoadStringFromFile(TempFile, Content) then
    exit;
  Result := Pos(Uppercase(Needle), Uppercase(Content)) > 0;
end;

function WaitForServiceStopped(const ServiceName: string): Boolean;
var
  I: Integer;
begin
  for I := 0 to 29 do
  begin
    if not ServiceOutputContains(ServiceName, 'RUNNING') then
    begin
      Result := True;
      exit;
    end;
    Sleep(1000);
  end;
  Result := not ServiceOutputContains(ServiceName, 'RUNNING');
end;

function ShouldInstallFreshService(): Boolean;
begin
  Result := WizardIsTaskSelected('installservice') and (not ExistingServicePresent);
end;

function ShouldRefreshExistingService(): Boolean;
begin
  Result := ExistingServicePresent;
end;

function ShouldRestartExistingService(): Boolean;
begin
  Result := ExistingServicePresent and ExistingServiceWasRunning;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  ResultCode: Integer;
begin
  Result := '';
  ExistingServicePresent := ServiceOutputContains('RoodoxServer', 'STATE');
  ExistingServiceWasRunning := False;

  if ExistingServicePresent then
  begin
    ExistingServiceWasRunning := ServiceOutputContains('RoodoxServer', 'RUNNING');
    if ExistingServiceWasRunning then
    begin
      Exec(
        ExpandConstant('{cmd}'),
        '/C sc.exe stop "RoodoxServer"',
        '',
        SW_HIDE,
        ewWaitUntilTerminated,
        ResultCode
      );
      if not WaitForServiceStopped('RoodoxServer') then
        Result := 'Existing Roodox Windows service is still running. Stop it manually and rerun setup.';
    end;
  end;
end;
"@

    Set-Content -LiteralPath $Path -Value $script -Encoding ascii
}

function Invoke-AllInOneInstallerBuild {
    param(
        [string]$RepoRoot,
        [string]$Version,
        [string]$ReleaseRoot,
        [string]$ServerExe,
        [string]$WorkbenchExe
    )

    $isccPath = Get-InnoSetupCompilerPath
    if ([string]::IsNullOrWhiteSpace($isccPath)) {
        throw "Inno Setup compiler not found. Install JRSoftware.InnoSetup or add iscc.exe to PATH."
    }

    $installerRoot = Join-Path $ReleaseRoot "installer"
    $stage = New-InstallerStage -RepoRoot $RepoRoot -ServerExe $ServerExe -WorkbenchExe $WorkbenchExe -StageDir (Join-Path $installerRoot "stage")
    $setupBaseName = "roodox-server-win-v$Version-setup"
    $issPath = Join-Path $installerRoot "roodox-server-all-in-one.iss"
    Write-InnoSetupScript -Path $issPath -Version $Version -AppSourceDir $stage.AppDir -OutputDir $ReleaseRoot -OutputBaseFileName $setupBaseName

    & $isccPath "/Qp" $issPath | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "iscc.exe failed with exit code $LASTEXITCODE"
    }

    $setupPath = Join-Path $ReleaseRoot ($setupBaseName + ".exe")
    if (-not (Test-Path -LiteralPath $setupPath)) {
        throw "all-in-one installer not found: $setupPath"
    }

    return [pscustomobject]@{
        SetupPath = $setupPath
        IssPath = $issPath
        StageDir = $stage.StageDir
    }
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
Write-WorkbenchLauncher -Path (Join-Path $bundleDir "start-roodox-workbench.cmd")

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

$setupPath = $null
$setupName = ""
if ($BuildInstaller) {
    $installer = Invoke-AllInOneInstallerBuild -RepoRoot $repoRoot -Version $resolvedVersion -ReleaseRoot $releaseRoot -ServerExe $serverExe -WorkbenchExe $layout.ExecutablePath
    $setupPath = $installer.SetupPath
    $setupName = [System.IO.Path]::GetFileName($setupPath)
}

Write-ReleaseReadme -Path (Join-Path $bundleDir "RELEASE.txt") -Version $resolvedVersion -MsiFileName $msiName -SetupFileName $setupName

if (Test-Path -LiteralPath $zipPath) {
    Remove-Item -LiteralPath $zipPath -Force
}
Compress-Archive -Path (Join-Path $bundleDir "*") -DestinationPath $zipPath -Force

[pscustomobject]@{
    Version = $resolvedVersion
    BundleDir = $bundleDir
    ZipPath = $zipPath
    MsiPath = $msiPath
    SetupPath = $setupPath
    ServerExe = Join-Path $bundleDir "roodox_server.exe"
    WorkbenchExe = Join-Path $bundleDir "roodox-workbench.exe"
    ConfigPath = Join-Path $bundleDir "roodox.config.json"
}
