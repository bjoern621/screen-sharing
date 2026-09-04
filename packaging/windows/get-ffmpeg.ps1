<#
  get-ffmpeg.ps1 - download ffmpeg and ffplay into build/windows/redist,
  where `task build:windows` copies them from on its way to build/bin.

  Not straight into build/bin: that directory is a build output and is rebuilt,
  while the redistributables are fetched once per machine and kept (docs/packaging.md).
  The app looks for the pair beside its own executable and then on PATH
  (backend/internal/ffmpeg, FindExe), which is what the copy into build/bin serves.

  Run once per machine:
    .\get-ffmpeg.ps1
#>
param(
  [string]$Dest = (Join-Path (Split-Path (Split-Path $PSScriptRoot -Parent) -Parent) "build\windows\redist")
)

$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force $Dest | Out-Null
$zip = Join-Path $Dest "ffmpeg.zip"
$url = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"

Write-Host "downloading $url ..." -ForegroundColor Yellow
Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing
$tmp = Join-Path $Dest "_x"
Expand-Archive -Path $zip -DestinationPath $tmp -Force
$binDir = Get-ChildItem $tmp -Directory | Select-Object -First 1
Copy-Item (Join-Path $binDir.FullName "bin\ffmpeg.exe") $Dest -Force
Copy-Item (Join-Path $binDir.FullName "bin\ffplay.exe") $Dest -Force
Remove-Item $zip -Force
Remove-Item $tmp -Recurse -Force
Write-Host "ffmpeg + ffplay in $Dest" -ForegroundColor Green
