<#
  whoislive.ps1 - list active streams on the relay via MediaMTX HTTP API.
  This is your account-free discovery: shows who is publishing + their bitrate.

  Examples:
    ./whoislive.ps1
    ./whoislive.ps1 -Relay 100.x.x.x
#>
param(
  [string]$Relay = "127.0.0.1",
  [int]   $ApiPort = 9997
)

$url = "http://${Relay}:${ApiPort}/v3/paths/list"

try {
  $data = Invoke-RestMethod -Uri $url -TimeoutSec 5
} catch {
  Write-Error "Cannot reach relay API at $url . Is MediaMTX up? (task relay)"
  exit 1
}

if (-not $data.items -or $data.items.Count -eq 0) {
  Write-Host "No one live." -ForegroundColor DarkGray
  exit 0
}

Write-Host "LIVE NOW on $Relay" -ForegroundColor Cyan
$data.items | ForEach-Object {
  $ready = if ($_.ready) { "●" } else { "○" }
  $mbps  = if ($_.bytesReceived) { "" } else { "" }
  $kbps  = [math]::Round(($_.tracks.Count), 0)
  [PSCustomObject]@{
    Live    = $ready
    Stream  = $_.name
    Tracks  = ($_.tracks -join ",")
    Readers = $_.readers.Count
    Source  = $_.source.type
  }
} | Format-Table -AutoSize

Write-Host "Watch one:  ./watch.ps1 -Name <Stream> -Relay $Relay" -ForegroundColor DarkGray
