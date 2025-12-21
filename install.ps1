Param(
  [Parameter(Position=0)]
  [string]$ProjectPath = "",

  [string]$Repo = $env:AWKIT_REPO,
  [string]$Version = $env:AWKIT_VERSION,
  [string]$Prefix = $env:AWKIT_PREFIX
)

if ([string]::IsNullOrWhiteSpace($Repo)) { $Repo = "silver2dream/ai-workflow-kit" }
if ([string]::IsNullOrWhiteSpace($Version)) { $Version = "latest" }
if ([string]::IsNullOrWhiteSpace($Prefix)) { $Prefix = "$HOME\.local" }

$BinDir = Join-Path $Prefix "bin"
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

$Asset = "awkit_windows_amd64.zip"
if ($Version -eq "latest") {
  $Url = "https://github.com/$Repo/releases/latest/download/$Asset"
} else {
  $Url = "https://github.com/$Repo/releases/download/$Version/$Asset"
}

$Tmp = New-Item -ItemType Directory -Force -Path (Join-Path $env:TEMP ("awkit-" + [guid]::NewGuid().ToString()))
$ZipPath = Join-Path $Tmp.FullName $Asset

Write-Host ""
Write-Host "Installing awkit..."
Write-Host "  Platform: windows/amd64"
Write-Host "  Version:  $Version"
Write-Host ""

Write-Host "[1/3] Downloading..."
try {
  Invoke-WebRequest -Uri $Url -OutFile $ZipPath -ErrorAction Stop
} catch {
  Write-Host ""
  Write-Host "✗ Download failed" -ForegroundColor Red
  Write-Host ""
  Write-Host "Troubleshooting:" -ForegroundColor Yellow
  Write-Host "  - Check your internet connection"
  Write-Host "  - Verify the release exists: $Url"
  Write-Host "  - Try setting `$env:AWKIT_VERSION to a specific version"
  Write-Host ""
  Write-Host "Error: $_" -ForegroundColor Red
  exit 1
}

Write-Host "[2/3] Extracting..."
try {
  Expand-Archive -Path $ZipPath -DestinationPath $Tmp.FullName -Force -ErrorAction Stop
} catch {
  Write-Host ""
  Write-Host "✗ Extraction failed" -ForegroundColor Red
  Write-Host ""
  Write-Host "The downloaded file may be corrupted. Try again."
  Write-Host "Error: $_" -ForegroundColor Red
  exit 1
}

$Exe = Join-Path $Tmp.FullName "awkit.exe"
if (-not (Test-Path $Exe)) {
  Write-Host ""
  Write-Host "✗ awkit.exe not found in archive" -ForegroundColor Red
  exit 1
}

Write-Host "[3/3] Installing..."
try {
  $Dest = Join-Path $BinDir "awkit.exe"
  Copy-Item -Force $Exe $Dest -ErrorAction Stop
} catch {
  Write-Host ""
  Write-Host "✗ Cannot install to: $Dest" -ForegroundColor Red
  Write-Host ""
  Write-Host "Check write permissions or try running as Administrator."
  Write-Host "Error: $_" -ForegroundColor Red
  exit 1
}

Remove-Item -Recurse -Force $Tmp.FullName -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "✓ awkit installed to $Dest" -ForegroundColor Green

# Check if BinDir is in PATH
$CurrentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($CurrentPath -split ";" | Where-Object { $_ -eq $BinDir }) {
  Write-Host "✓ $BinDir is already in PATH" -ForegroundColor Green
  Write-Host ""
  Write-Host "Run 'awkit version' to verify installation."
} else {
  Write-Host ""
  Write-Host "To use awkit, add it to your PATH:" -ForegroundColor Yellow
  Write-Host ""
  Write-Host "  # Add to User PATH (permanent):" -ForegroundColor Cyan
  Write-Host "  `$path = [Environment]::GetEnvironmentVariable('PATH', 'User')"
  Write-Host "  [Environment]::SetEnvironmentVariable('PATH', `"`$path;$BinDir`", 'User')"
  Write-Host ""
  Write-Host "  # Or for current session only:"
  Write-Host "  `$env:PATH += `";$BinDir`""
  Write-Host ""
  Write-Host "Then restart your terminal and run 'awkit version' to verify."
}
Write-Host ""

if (-not [string]::IsNullOrWhiteSpace($ProjectPath)) {
  & $Dest install $ProjectPath --preset react-go
}

