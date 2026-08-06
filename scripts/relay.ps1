<#
  relay.ps1 - run the MediaMTX relay natively on Windows.

  Why native and not Docker on Windows:
    Docker Desktop's UDP port-proxy rewrites the source port and breaks SRT's
    handshake, so host->container SRT fails with "I/O error". A native binary
    binds :8890 directly on the host - no NAT, SRT just works.
    (On a Linux relay box/VPS, `docker compose up -d` is fine - UDP forwards there.)

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
Write-Host "Friends point their -Relay at this machine's IP." -ForegroundColor DarkGray

if ($Background) {
  $p = Start-Process -FilePath $exe -ArgumentList $conf -PassThru -WindowStyle Hidden `
        -RedirectStandardOutput (Join-Path $bin "relay.out.log") `
        -RedirectStandardError  (Join-Path $bin "relay.err.log")
  Write-Host "Relay running in background, pid $($p.Id). Logs in bin\relay.out.log" -ForegroundColor Green
} else {
  & $exe $conf
}
