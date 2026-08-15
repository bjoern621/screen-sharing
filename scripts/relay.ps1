<#
  relay.ps1 - run the MediaMTX relay natively on Windows.

  Why this exists: Linux and macOS take the mediamtx binary from the flake's dev
  shell, which Windows has none of, so this fetches the same version instead.
  Anything that puts a NAT between the host and the relay is worth avoiding here:
  a UDP proxy that rewrites the source port breaks SRT's handshake with "I/O
  error", where a native binary binds :8890 on the host and SRT just works.

  Downloads mediamtx.exe into ./bin on first run, then launches it with our
  mediamtx.yml. Runs in the foreground; Ctrl+C to stop.

  Examples:
    ./relay.ps1
    ./relay.ps1 -Background      # launch hidden, return control
#>
param(
  [string]$Version = "1.20.0",
  [switch]$Background
)

$ErrorActionPreference = "Stop"
$root = Split-Path $PSScriptRoot -Parent
$bin  = Join-Path $root "bin"
$exe  = Join-Path $bin "mediamtx.exe"
$conf = Join-Path $root "mediamtx.yml"

if (-not (Test-Path $exe)) {
  Write-Host "mediamtx.exe not found - downloading v$Version ..." -ForegroundColor Yellow
  New-Item -ItemType Directory -Force $bin | Out-Null
  $zip = Join-Path $bin "mediamtx.zip"
  $url = "https://github.com/bluenviron/mediamtx/releases/download/v$Version/mediamtx_v${Version}_windows_amd64.zip"
  Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing
  Expand-Archive -Path $zip -DestinationPath $bin -Force
  Remove-Item $zip
  # drop the bundled default config so ours is the only one in play
  Remove-Item (Join-Path $bin "mediamtx.yml") -ErrorAction SilentlyContinue
}

Write-Host "Relay starting on this host." -ForegroundColor Cyan
Write-Host "  SRT   udp  :8890   (publish/watch)"       -ForegroundColor DarkGray
Write-Host "  API   tcp  :9997   (whoislive discovery)" -ForegroundColor DarkGray
Write-Host "  HLS   tcp  :8888   (browser watch)"       -ForegroundColor DarkGray
Write-Host "  MoQ   both :8892   (browser watch)"       -ForegroundColor DarkGray
Write-Host "Friends point their -Relay at this machine's IP." -ForegroundColor DarkGray

if ($Background) {
  $p = Start-Process -FilePath $exe -ArgumentList $conf -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput (Join-Path $bin "relay.out.log") `
        -RedirectStandardError  (Join-Path $bin "relay.err.log")
  Write-Host "Relay running in background, pid $($p.Id). Logs in bin\relay.out.log" -ForegroundColor Green
} else {
  & $exe $conf
}
