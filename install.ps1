# rog installer — Windows (PowerShell)
#
# Usage:
#   irm https://raw.githubusercontent.com/Geogboe/rog/main/install.ps1 | iex
#
# Environment variables:
#   $env:ROG_VERSION     Pin a specific version (e.g. v0.2.0). Defaults to latest release.
#   $env:ROG_INSTALL_DIR Override install directory. Defaults to $HOME\.local\bin.
#   $env:ROG_DEBUG       Set to 1 for verbose output.

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

$Repo       = 'Geogboe/rog'
$InstallDir = if ($env:ROG_INSTALL_DIR) { $env:ROG_INSTALL_DIR } else { "$HOME\.local\bin" }
$IsDebug    = $env:ROG_DEBUG -eq '1'

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

function Write-Info  ([string]$Msg) { Write-Host "==> $Msg" -ForegroundColor Cyan }
function Write-Ok    ([string]$Msg) { Write-Host "v $Msg" -ForegroundColor Green }
function Write-Warn  ([string]$Msg) { Write-Warning $Msg }
function Write-Dbg   ([string]$Msg) { if ($IsDebug) { Write-Host "[debug] $Msg" -ForegroundColor DarkGray } }

# ---------------------------------------------------------------------------
# Version resolution
# ---------------------------------------------------------------------------

function Resolve-RogVersion {
    if ($env:ROG_VERSION) {
        Write-Dbg "Using pinned version: $($env:ROG_VERSION)"
        return $env:ROG_VERSION
    }
    Write-Info "Fetching latest release version..."
    # Use /releases?per_page=1 so prereleases are included (/releases/latest skips them)
    $Releases = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases?per_page=1"
    $Version  = $Releases[0].tag_name
    if (-not $Version) {
        throw "Could not determine latest version. Set `$env:ROG_VERSION to install a specific version."
    }
    Write-Dbg "Latest version: $Version"
    return $Version
}

# ---------------------------------------------------------------------------
# Checksum verification
# ---------------------------------------------------------------------------

function Test-Checksum {
    param(
        [string]$File,
        [string]$ChecksumsFile
    )
    $FileName  = [System.IO.Path]::GetFileName($File)
    $Checksums = Get-Content $ChecksumsFile

    # checksums.txt format: "<hash>  <filename>" (two spaces)
    $Line = $Checksums | Where-Object { $_ -match "\s$([regex]::Escape($FileName))$" }
    if (-not $Line) {
        throw "Checksum not found for $FileName in checksums.txt"
    }
    $Expected = ($Line -split '\s+')[0].ToLower()

    # Use .NET directly — more portable than Get-FileHash (avoids profile/module issues)
    $sha256 = [System.Security.Cryptography.SHA256CryptoServiceProvider]::new()
    $bytes  = [System.IO.File]::ReadAllBytes($File)
    $Actual = ([System.BitConverter]::ToString($sha256.ComputeHash($bytes)) -replace '-').ToLower()
    $sha256.Dispose()
    if ($Actual -ne $Expected) {
        throw "Checksum mismatch for ${FileName}!`n  expected: $Expected`n  got:      $Actual"
    }
    Write-Ok "Checksum verified"
}

# ---------------------------------------------------------------------------
# PATH check — prints a copy-pasteable one-liner if the install dir is missing
# ---------------------------------------------------------------------------

function Test-InPath {
    param([string]$Dir)
    $UserPath   = [Environment]::GetEnvironmentVariable('Path', 'User');   if (-not $UserPath)   { $UserPath = '' }
    $SystemPath = [Environment]::GetEnvironmentVariable('Path', 'Machine'); if (-not $SystemPath) { $SystemPath = '' }
    $AllDirs    = ($UserPath + ';' + $SystemPath) -split ';' |
                  ForEach-Object { $_.TrimEnd('\').TrimEnd('/') } |
                  Where-Object   { $_ -ne '' }
    return ($AllDirs -contains $Dir.TrimEnd('\').TrimEnd('/'))
}

function Show-PathInstructions {
    param([string]$Dir)
    Write-Warn "$Dir is not in your PATH"
    Write-Host ""
    Write-Host "  Add it permanently by running (then open a new terminal):" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "    [Environment]::SetEnvironmentVariable('Path', `"$Dir;`" + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')" -ForegroundColor Cyan
    Write-Host ""
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

$TmpDir = $null

try {
    $Version    = Resolve-RogVersion
    $VersionNum = $Version.TrimStart('v')
    $Archive    = "rog-$VersionNum-windows-amd64.zip"
    $ChecksumsUrl = "https://github.com/$Repo/releases/download/$Version/checksums.txt"
    $ArchiveUrl   = "https://github.com/$Repo/releases/download/$Version/$Archive"

    Write-Dbg "Version: $Version ($VersionNum)"
    Write-Dbg "Archive: $Archive"
    Write-Dbg "Install dir: $InstallDir"

    Write-Info "Installing rog $Version (windows/amd64)..."

    # Create temp directory
    $TmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
    New-Item -ItemType Directory -Path $TmpDir | Out-Null

    # Download checksums
    Write-Info "Downloading checksums..."
    $ChecksumsFile = Join-Path $TmpDir "checksums.txt"
    Invoke-WebRequest -Uri $ChecksumsUrl -OutFile $ChecksumsFile -UseBasicParsing
    Write-Dbg "Checksums saved to $ChecksumsFile"

    # Download archive
    Write-Info "Downloading $Archive..."
    $ArchiveFile = Join-Path $TmpDir $Archive
    Invoke-WebRequest -Uri $ArchiveUrl -OutFile $ArchiveFile -UseBasicParsing

    # Verify checksum
    Write-Info "Verifying checksum..."
    Test-Checksum -File $ArchiveFile -ChecksumsFile $ChecksumsFile

    # Create install directory
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    # Extract binary
    Write-Info "Installing to $InstallDir..."
    Expand-Archive -Path $ArchiveFile -DestinationPath $TmpDir -Force
    $BinarySource = Join-Path $TmpDir "rog.exe"
    $BinaryDest   = Join-Path $InstallDir "rog.exe"
    Copy-Item -Path $BinarySource -Destination $BinaryDest -Force

    Write-Ok "rog $Version installed to $BinaryDest"
    Write-Host ""

    # PATH check
    if (-not (Test-InPath $InstallDir)) {
        Show-PathInstructions $InstallDir
    }

    Write-Host "Run 'rog --help' to get started."
}
finally {
    if ($TmpDir -and (Test-Path $TmpDir)) {
        Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
    }
}
