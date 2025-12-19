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

Write-Host "[install] Installed: $Dest"
Write-Host "[install] Add to PATH if needed: $BinDir"

if (-not [string]::IsNullOrWhiteSpace($ProjectPath)) {
  & $Dest install $ProjectPath --preset react-go
}

