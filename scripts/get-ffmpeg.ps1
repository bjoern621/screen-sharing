<#
  get-ffmpeg.ps1 - download ffmpeg/ffplay into app/build/bin/bin so the built
  app.exe finds them next to itself (FindExe checks .\bin first, PATH second).

  Run once per machine:
    .\get-ffmpeg.ps1
#>
param(
  [string]$Dest = (Join-Path (Split-Path $PSScriptRoot -Parent) "app\build\bin\bin")
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
