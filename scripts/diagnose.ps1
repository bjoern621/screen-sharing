<#
  diagnose.ps1 - run this WHILE publish + watch are running and the picture is broken.

  Opens a second, headless reader on the same stream for 10s and counts decode
  errors. Splits the fault:
    headless reader ALSO broken  -> publisher/relay side (stream itself corrupt)
    headless reader clean        -> your ffplay window/env is the problem
  Also dumps relay stats.

  Example:
    .\diagnose.ps1 -Name bjoern
#>
param(
  [Parameter(Mandatory = $true)] [string]$Name,
  [string]$Relay = "127.0.0.1",
  [int]   $Port  = 8890,
  [int]   $ApiPort = 9997
)

$ErrorActionPreference = "Continue"

"=== relay path stats ==="
try {
  $paths = Invoke-RestMethod "http://${Relay}:${ApiPort}/v3/paths/list" -TimeoutSec 5
  $paths.items | ForEach-Object {
    "path=$($_.name)  ready=$($_.ready)  tracks=$($_.tracks -join ',')  readers=$($_.readers.Count)  bytesIn=$([math]::Round($_.bytesReceived/1MB,1))MB"
  }
} catch { "relay API unreachable: $_" }

"=== ffmpeg/ffplay processes right now ==="
Get-Process ffmpeg,ffplay -ErrorAction SilentlyContinue | ForEach-Object {
  "$($_.ProcessName) pid=$($_.Id) started=$($_.StartTime.ToString('HH:mm:ss'))"
}

"=== headless 10s read of '$Name' (no display, decode only) ==="
$url = "srt://${Relay}:${Port}?streamid=read:${Name}&latency=1500000&rcvbuf=150000000&ffs=150000000"
$errLog = Join-Path $env:TEMP "diag_read.log"
& ffmpeg -hide_banner -loglevel warning -i $url -t 10 -f null NUL 2>$errLog
$errs = (Select-String -Path $errLog -Pattern "Could not find ref|undecodable|corrupt" -ErrorAction SilentlyContinue).Count
"decode errors in 10s headless read: $errs"
""
if ($errs -gt 5) {
  "VERDICT: stream itself broken -> publisher or relay side. Paste this output."
} else {
  "VERDICT: stream is CLEAN -> problem is in the ffplay window/session. Paste this output."
}
