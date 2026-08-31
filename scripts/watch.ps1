<#
  watch.ps1 - open a friend's stream from the relay.

  Uses ffplay, which ships with ffmpeg.
  Low-latency flags set.

  Examples:
    ./watch.ps1 -Name bjorn
    ./watch.ps1 -Name friendA -Relay 100.x.x.x
#>
param(
  [Parameter(Mandatory = $true)] [string]$Name,
  [string]$Relay = "127.0.0.1",
  [int]   $Port  = 8890,
  [int]   $LatencyMs = 1500,     # SRT receive buffer; higher = smoother, more delay
  [int]   $Width  = 0,           # 0 = native resolution, no downscale; a value shrinks the window
  [int]   $Height = 0            # 0 = native
)

if (-not (Get-Command ffplay -ErrorAction SilentlyContinue)) {
  Write-Error "ffplay not found on PATH (ships with ffmpeg)."
  exit 1
}

# ffmpeg's srt 'latency' option is in microseconds.
# Big rcvbuf and fc, so lossless keyframe bursts survive
# while the player drains at display pace.
$latUs = $LatencyMs * 1000
$srt = "srt://${Relay}:${Port}?streamid=read:${Name}&latency=${latUs}&rcvbuf=150000000&ffs=150000000"
Write-Host "Watching '$Name' from $Relay  (SRT latency ${LatencyMs}ms)" -ForegroundColor Cyan

$size = @()
if ($Width -gt 0 -and $Height -gt 0) { $size = @("-x", "$Width", "-y", "$Height") }

# -loglevel fatal -nostats, and not for tidiness.
# ffplay's status line and per-frame decode errors go to this console,
# and a clicked Windows console blocks the writer,
# which stalls ffplay's decode loop, expires SRT packets and freezes the picture.
ffplay -hide_banner -loglevel fatal -nostats -fflags nobuffer -flags low_delay -framedrop `
  @size `
  -window_title "watch: $Name" `
  $srt
