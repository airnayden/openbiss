#Requires -Version 5.1
<#
.SYNOPSIS
    OpenBISS Windows installer (source build).

.DESCRIPTION
    Builds OpenBISS from source as a native Windows .exe using
    'fyne package -os windows', then installs it under
    %LOCALAPPDATA%\Programs\OpenBISS together with a Start Menu shortcut.
    No administrator rights are required.

.PARAMETER Uninstall
    Remove the installed binary and Start Menu shortcut. User data under
    %APPDATA%\OpenBISS (TLS cert, logs, settings) is preserved.

.EXAMPLE
    .\scripts\install-windows.ps1
    Build OpenBISS from source and install it for the current user.

.EXAMPLE
    .\scripts\install-windows.ps1 -Uninstall
    Remove OpenBISS while keeping %APPDATA%\OpenBISS user data intact.

.NOTES
    If PowerShell refuses to run unsigned scripts, lower the policy once
    for the current user:

        Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned

    Prerequisites (checked automatically):
      - Go toolchain         (winget install GoLang.Go
                              or  https://go.dev/dl/)
      - MinGW-w64 gcc.exe    (winget install mstorsjo.LLVM-MinGW
                              or  choco install mingw
                              or  scoop install mingw)
      - fyne CLI is auto-installed via 'go install' when missing.

    This script never auto-launches OpenBISS, never code-signs the binary,
    and never deletes %APPDATA%\OpenBISS user data.
#>
[CmdletBinding()]
param(
    [switch]$Uninstall
)

$ErrorActionPreference = "Stop"

# ---------------------------------------------------------------------------
# Constants.
# ---------------------------------------------------------------------------
$AppName     = "OpenBISS"
$AppId       = "com.openbiss.openbiss"
$InstallDir  = Join-Path $env:LOCALAPPDATA "Programs\$AppName"
$StartMenu   = Join-Path $env:APPDATA      "Microsoft\Windows\Start Menu\Programs"
$ShortcutLnk = Join-Path $StartMenu        "$AppName.lnk"

# Resolve project root from script location ($PSScriptRoot is .../scripts).
$Project  = Split-Path -Parent $PSScriptRoot
$IconPath = Join-Path $Project "assets\icon.png"
$ExeName  = "$AppName.exe"

# ---------------------------------------------------------------------------
# Coloured output helpers.
# ---------------------------------------------------------------------------
function Write-Info ($msg) { Write-Host $msg -ForegroundColor Cyan }
function Write-Ok   ($msg) { Write-Host $msg -ForegroundColor Green }
function Write-Warn ($msg) { Write-Host $msg -ForegroundColor Yellow }
function Write-Fail ($msg) { Write-Host $msg -ForegroundColor Red }

# ---------------------------------------------------------------------------
# Uninstall path: remove install dir + Start Menu shortcut, preserve user data.
# Idempotent: Test-Path before Remove-Item so re-runs are safe.
# ---------------------------------------------------------------------------
if ($Uninstall) {
    Write-Info "Uninstalling $AppName..."

    if (Test-Path $InstallDir) {
        Write-Host "   -> Removing $InstallDir"
        Remove-Item -Recurse -Force $InstallDir
    } else {
        Write-Host "   -> $InstallDir not present (skipping)"
    }

    if (Test-Path $ShortcutLnk) {
        Write-Host "   -> Removing $ShortcutLnk"
        Remove-Item -Force $ShortcutLnk
    } else {
        Write-Host "   -> $ShortcutLnk not present (skipping)"
    }

    Write-Ok "Uninstall complete."
    $UserData = Join-Path $env:APPDATA $AppName
    Write-Host "   User data preserved at $UserData (TLS cert, logs, settings)."
    Write-Host "   To remove it manually:  Remove-Item -Recurse -Force `"$UserData`""
    exit 0
}

# ---------------------------------------------------------------------------
# Prerequisite checks.
# Each missing dep prints install hints, calls Write-Error, then exits 1.
# ---------------------------------------------------------------------------
Write-Info "Checking prerequisites..."

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Fail "Go toolchain not found."
    Write-Host "   -> winget install GoLang.Go"
    Write-Host "   -> Or download from https://go.dev/dl/"
    Write-Error -Message "Missing dependency: go" -ErrorAction Continue
    exit 1
}
Write-Host ("   [ok] Go found: {0}" -f (& go version))

if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
    Write-Fail "MinGW-w64 (gcc) not found."
    Write-Host "   Fyne requires a C compiler on Windows (CGO). Install one of:"
    Write-Host "   -> winget install mstorsjo.LLVM-MinGW"
    Write-Host "   -> choco install mingw"
    Write-Host "   -> scoop install mingw"
    Write-Error -Message "Missing dependency: gcc (MinGW-w64)" -ErrorAction Continue
    exit 1
}
$gccFirstLine = (& gcc --version | Select-Object -First 1)
Write-Host "   [ok] gcc found: $gccFirstLine"

# ---------------------------------------------------------------------------
# Ensure fyne CLI is installed; auto-install via 'go install' when missing.
# ---------------------------------------------------------------------------
$GoBin    = Join-Path $env:USERPROFILE "go\bin"
$FynePath = Join-Path $GoBin           "fyne.exe"

if (-not (Test-Path $FynePath)) {
    Write-Info "Installing fyne CLI (go install fyne.io/tools/cmd/fyne@latest)..."
    & go install fyne.io/tools/cmd/fyne@latest
}
if (-not (Test-Path $FynePath)) {
    Write-Fail "fyne CLI install failed; expected at $FynePath"
    Write-Error -Message "fyne CLI missing after install" -ErrorAction Continue
    exit 1
}
Write-Host "   [ok] fyne CLI: $FynePath"

# ---------------------------------------------------------------------------
# Sanity-check fyne package inputs.
# ---------------------------------------------------------------------------
if (-not (Test-Path $IconPath)) {
    Write-Fail "Missing icon: $IconPath"
    Write-Error -Message "Missing asset: assets\icon.png" -ErrorAction Continue
    exit 1
}
$FyneAppToml = Join-Path $Project "FyneApp.toml"
if (-not (Test-Path $FyneAppToml)) {
    Write-Fail "Missing FyneApp.toml at $FyneAppToml"
    Write-Error -Message "Missing FyneApp.toml at project root" -ErrorAction Continue
    exit 1
}

# ---------------------------------------------------------------------------
# Build the Windows .exe via 'fyne package -os windows'.
# Run from project root so the relative icon path resolves correctly.
# ---------------------------------------------------------------------------
Push-Location $Project
try {
    # VERSION is informational here; fyne reads its version from FyneApp.toml.
    $Version = "dev"
    try {
        $gitVer = & git describe --tags --always --dirty 2>$null
        if ($LASTEXITCODE -eq 0 -and $gitVer) { $Version = $gitVer.Trim() }
    } catch {
        # git not on PATH -> keep "dev" fallback
    }

    # Drop any leftover .exe from a previous local build so detection is clean.
    if (Test-Path $ExeName) { Remove-Item -Force $ExeName }

    Write-Info ("Building {0} (version {1}) via fyne package -os windows..." -f $ExeName, $Version)
    & $FynePath package `
        -os windows `
        -icon assets/icon.png `
        -name $AppName `
        -app-id $AppId `
        -release
    if ($LASTEXITCODE -ne 0) {
        Write-Fail "fyne package exited with code $LASTEXITCODE"
        Write-Error -Message "fyne package failed" -ErrorAction Continue
        exit 1
    }

    if (-not (Test-Path $ExeName)) {
        Write-Fail "Build did not produce $ExeName"
        Write-Error -Message "Build artifact missing" -ErrorAction Continue
        exit 1
    }
    Write-Host "   [ok] Built $ExeName"
}
finally {
    Pop-Location
}

# ---------------------------------------------------------------------------
# Install to %LOCALAPPDATA%\Programs\OpenBISS (user-local, no admin needed).
# ---------------------------------------------------------------------------
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

$ExeSrc     = Join-Path $Project    $ExeName
$ExeTarget  = Join-Path $InstallDir $ExeName
$IconTarget = Join-Path $InstallDir "icon.png"

if (Test-Path $ExeTarget) {
    Write-Host "   -> Removing existing $ExeTarget"
    Remove-Item -Force $ExeTarget
}

Write-Info "Installing $ExeTarget..."
Move-Item -Force -Path $ExeSrc -Destination $ExeTarget

# Copy icon alongside the exe (mirrors the .app bundle layout on macOS).
Copy-Item -Force -Path $IconPath -Destination $IconTarget
Write-Host "   [ok] Installed $ExeTarget"

# ---------------------------------------------------------------------------
# Create Start Menu shortcut via WScript.Shell COM object.
# ---------------------------------------------------------------------------
Write-Info "Creating Start Menu shortcut..."
$WshShell = New-Object -ComObject WScript.Shell
$Shortcut = $WshShell.CreateShortcut($ShortcutLnk)
$Shortcut.TargetPath       = $ExeTarget
$Shortcut.IconLocation     = "$ExeTarget,0"
$Shortcut.WorkingDirectory = $InstallDir
$Shortcut.Description      = "Open-source BORICA BISS smart-card signing service"
$Shortcut.Save()
Write-Host "   [ok] Shortcut: $ShortcutLnk"

# ---------------------------------------------------------------------------
# Post-install instructions.
# ---------------------------------------------------------------------------
Write-Host ""
Write-Ok "$AppName installed successfully."
Write-Host ""
Write-Host "To launch:"
Write-Host "   - Start Menu:  press the Windows key, type 'OpenBISS', then Enter"
Write-Host "   - Explorer:    open $InstallDir and double-click $ExeName"
Write-Host "   - PowerShell:  & `"$ExeTarget`""
Write-Host ""
Write-Warn "SmartScreen warning (first launch only):"
Write-Host "   The binary is NOT code-signed. Windows SmartScreen may display"
Write-Host "   'Windows protected your PC' the first time OpenBISS starts."
Write-Host "   Click 'More info' and then 'Run anyway' to launch OpenBISS."
Write-Host "   The warning will not reappear on subsequent launches."
Write-Host ""
Write-Host "To uninstall:"
Write-Host "   .\scripts\install-windows.ps1 -Uninstall"
Write-Host ""
