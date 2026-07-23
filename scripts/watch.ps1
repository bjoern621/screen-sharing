<#
  watch.ps1 - open a friend's stream from the relay.

  Uses ffplay (ships with ffmpeg). Low-latency flags set.

  Examples:
    ./watch.ps1 -Name bjorn
    ./watch.ps1 -Name friendA -Relay 100.x.x.x
#>
param(
  [Parameter(Mandatory = $true)] [string]$Name,
  [string]$Relay = "127.0.0.1",
  [int]   $Port  = 8890,
  [int]   $LatencyMs = 1500,     # SRT receive buffer; higher = smoother, more delay
  [int]   $Width  = 0,           # 0 = native resolution (crisp, no downscale). Set to shrink window.
  [int]   $Height = 0            # 0 = native
)

if (-not (Get-Command ffplay -ErrorAction SilentlyContinue)) {
  Write-Error "ffplay not found on PATH (ships with ffmpeg)."
  exit 1
}

# ffmpeg's srt 'latency' option is MICROSECONDS. Big rcvbuf+fc so lossless
# keyframe bursts survive while the player drains at display pace.
$latUs = $LatencyMs * 1000
$srt = "srt://${Relay}:${Port}?streamid=read:${Name}&latency=${latUs}&rcvbuf=150000000&ffs=150000000"
Write-Host "Watching '$Name' from $Relay  (SRT latency ${LatencyMs}ms)" -ForegroundColor Cyan

# native by default (crisp); pass -Width/-Height to shrink the window
$size = @()
if ($Width -gt 0 -and $Height -gt 0) { $size = @("-x", "$Width", "-y", "$Height") }

# -loglevel fatal -nostats: CRITICAL. ffplay's status line + per-frame decode
# errors go to this console; a focused/clicked Windows console blocks the
# writer, which stalls ffplay's decode loop, SRT expires packets, and the
# stream death-spirals into a frozen picture. Silence = immune.
ffplay -hide_banner -loglevel fatal -nostats -fflags nobuffer -flags low_delay -framedrop `
  @size `
  -window_title "watch: $Name" `
  $srt
