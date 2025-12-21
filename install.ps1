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

Write-Host "[install] Downloading $Url"
Invoke-WebRequest -Uri $Url -OutFile $ZipPath

Expand-Archive -Path $ZipPath -DestinationPath $Tmp.FullName -Force

$Exe = Join-Path $Tmp.FullName "awkit.exe"
if (-not (Test-Path $Exe)) {
  throw "awkit.exe not found in downloaded archive"
}

$Dest = Join-Path $BinDir "awkit.exe"
Copy-Item -Force $Exe $Dest

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

