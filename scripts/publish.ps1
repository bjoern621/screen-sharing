<#
  publish.ps1 - capture a screen and push it to the MediaMTX relay over SRT.

  Needs ffmpeg on PATH. Get a recent build: https://www.gyan.dev/ffmpeg/builds/
  (ddagrab GPU capture needs n6.1+; gdigrab works on any build.)

  Examples:
    ./publish.ps1 -Name bjorn
    ./publish.ps1 -Name bjorn -Fps 144 -Bitrate 120M -Codec hevc_nvenc -Chroma yuv444p
    ./publish.ps1 -Name bjorn -Capture ddagrab -Monitor 0
    ./publish.ps1 -Name bjorn -Relay 100.x.x.x
#>
param(
  [Parameter(Mandatory = $true)] [string]$Name,        # stream path, friends read this
  [string]$Relay   = "127.0.0.1",                      # relay host/IP
  [int]   $Port    = 8890,                             # SRT port
  [int]   $Fps     = 60,
  [string]$Bitrate = "40M",
  [ValidateSet("hevc_nvenc","h264_nvenc","av1_nvenc","hevc_amf","h264_amf","hevc_qsv","libx264")]
  [string]$Codec   = "hevc_nvenc",
  [ValidateSet("yuv420p","yuv444p","p010le","gbrp")]
  [string]$Chroma  = "gbrp",                            # gbrp = true RGB, exact desktop pixels (default); yuv420p = cheapest for weak links
  [ValidateSet("pc","tv")]
  [string]$Range   = "pc",                              # pc = full range 0-255
  [ValidateSet("gdigrab","ddagrab")]
  [string]$Capture = "ddagrab",                         # ddagrab = GPU, per-monitor (default); gdigrab = whole desktop fallback
  [int]   $Monitor = 0,                                 # ddagrab output index (which monitor)
  [string]$Scale   = "",                                # e.g. "2560:-1" downscale; needed for software libx264
  [ValidateSet("quality","latency","lossless")]
  [string]$Mode    = "quality",                         # quality = best look (VBR+cq); latency = min delay (CBR/ll); lossless = perfect, huge bitrate
  [int]   $Cq      = 19                                 # quality mode: lower = better/bigger (16 near-lossless, 23 lean)
)

if (-not (Get-Command ffmpeg -ErrorAction SilentlyContinue)) {
  Write-Error "ffmpeg not found on PATH. Install ffmpeg (n6.1+ for ddagrab)."
  exit 1
}

$mbps = [double]($Bitrate -replace '[^0-9.]','')
Write-Host "Publishing '$Name'  $($Fps)fps  $Codec  $Chroma  range=$Range  capture=$Capture  ~$mbps Mbps UP" -ForegroundColor Cyan
Write-Host "Friends watch:  ./watch.ps1 -Name $Name -Relay <your-ip>" -ForegroundColor DarkGray

# srt 'latency' is MICROSECONDS in ffmpeg; big sndbuf+fc for lossless bursts
$srt = "srt://${Relay}:${Port}?streamid=publish:${Name}&pkt_size=1316&latency=1500000&sndbuf=150000000&ffs=150000000"
$gop = $Fps * 2

# capture args differ per backend
if ($Capture -eq "ddagrab") {
  # GPU capture via filter source, hwdownload so any encoder can consume it
  $inputArgs = @(
    "-filter_complex", "ddagrab=output_idx=${Monitor}:framerate=${Fps},hwdownload,format=bgra"
  )
} else {
  # software capture of whole desktop
  $inputArgs = @(
    "-f", "gdigrab", "-framerate", "$Fps", "-i", "desktop"
  )
}

# optional downscale (software encode needs it to stay real-time)
$vf = @()
if ($Scale) { $vf = @("-vf", "scale=$Scale") }

# encoder args depend on codec family + mode
$isNvenc = $Codec -like "*nvenc"
if ($Codec -eq "libx264") {
  if ($Mode -eq "quality") {
    $encArgs = @("-c:v","libx264","-preset","slow","-crf","$Cq")          # crf drives quality
  } else {
    $encArgs = @("-c:v","libx264","-preset","veryfast","-tune","zerolatency","-b:v",$Bitrate)
  }
} elseif ($isNvenc) {
  if ($Mode -eq "lossless") {
    # true nvenc lossless: no rate control, bitrate is whatever the frame needs (can burst >1 Gbps)
    # -bf 0: B-frames save nothing in lossless, only add reorder complexity for the decoder
    $encArgs = @("-c:v",$Codec,"-preset","p7","-tune","lossless","-bf","0")
  } elseif ($Mode -eq "quality") {
    # VBR + constant-quality: cq drives look, -Bitrate is the burst ceiling. p7/hq/multipass = best per bit.
    $encArgs = @("-c:v",$Codec,"-preset","p7","-tune","hq","-multipass","fullres",
                 "-rc","vbr","-cq","$Cq","-b:v","0","-maxrate",$Bitrate,"-bufsize",$Bitrate)
  } else {
    $encArgs = @("-c:v",$Codec,"-preset","p5","-tune","ll","-rc","cbr","-b:v",$Bitrate)
  }
} else {
  # amf / qsv: generic bitrate path
  $encArgs = @("-c:v",$Codec,"-b:v",$Bitrate)
}

# color_range only applies to YUV; RGB (gbrp) is inherently full-range
$rangeArgs = if ($Chroma -eq "gbrp") { @() } else { @("-color_range", $Range) }

ffmpeg -hide_banner @inputArgs @vf `
  @encArgs `
  -pix_fmt $Chroma @rangeArgs `
  -g $gop `
  -f mpegts $srt
