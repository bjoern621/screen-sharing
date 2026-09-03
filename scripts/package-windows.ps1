# package-windows.ps1 - assemble the Windows release directory and its zip.
#
#   task package:windows
#
# Windows has no dependency manager an installer can lean on, so this artifact carries everything:
# the backend, the shell, ffmpeg and ffplay,
# the GStreamer command-line tools and the plugin set (docs/packaging.md).
# One directory holds all of it,
# both what the Windows loader searches first for a DLL and where the app's own lookups start:
# the shell finds the backend beside itself,
# and the backend finds ffmpeg and gst-launch-1.0 the same way (backend/internal/ffmpeg/exe.go).
#
# Before this, `task build:windows` builds the backend and copies ffmpeg and ffplay in beside it,
# and `task bundle:windows` copies the GStreamer runtime,
# the Task task naming both as dependencies.
# This script publishes the shell into the same directory and zips the result.
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$version = (Get-Content (Join-Path $root 'VERSION')).Trim()
$bin = Join-Path $root 'build/bin'
$dist = Join-Path $root 'build/dist'
$name = "screen-sharing-$version-windows-x86_64"
$zip = Join-Path $dist "$name.zip"

# Each of these comes from a task this one depends on,
# and each is a program the app spawns or loads at run time.
# Checking them here names the missing piece;
# a zip shipped without one fails at the user's end, where the same absence reads as a broken app.
# The inspector is checked beside the launcher because its absence is the quiet one:
# a zip missing it starts and runs, and greys the whole GStreamer engine instead,
# the probe reading one missing program as an install with no GStreamer tooling at all
# (backend/internal/encoders, gstAvailable).
$required = @(
    'screenshare-backend.exe',
    'ffmpeg.exe',
    'ffplay.exe',
    'gst-launch-1.0.exe',
    'gst-inspect-1.0.exe'
)
foreach ($file in $required) {
    $path = Join-Path $bin $file
    if (-not (Test-Path $path)) {
        throw "$path is missing: run 'task bundle:windows' first"
    }
}

# Self-contained, so the machine needs no .NET install.
# Into the same directory as the backend rather than beside it:
# the loader and every lookup here start in the directory the program was started from.
dotnet publish (Join-Path $root 'avalonia/ScreenShare.App/ScreenShare.App.csproj') `
    --configuration Release `
    --runtime win-x64 `
    --self-contained true `
    --output $bin
if ($LASTEXITCODE -ne 0) {
    throw "dotnet publish failed with exit code $LASTEXITCODE"
}

# The archive carries one directory rather than loose files,
# so an extract lands in a folder named after the version
# instead of scattering several hundred files into whatever directory the reader unpacked it in.
$stage = Join-Path $dist $name
if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
New-Item -ItemType Directory -Force -Path $stage | Out-Null
Copy-Item -Recurse -Force (Join-Path $bin '*') $stage

# The one file a reader of this archive sees before running anything.
# Everything the app needs is in the archive, so this names which program to start and nothing else.
$readme = @"
screen-sharing $version

Run screenshare-avalonia.exe. It starts the backend beside it, so there is
nothing else to launch, and nothing to install first.

To reach a relay, or to run one:
https://github.com/bjoern621/screen-sharing/blob/main/docs/install.md
"@
Set-Content -Path (Join-Path $stage 'README.txt') -Value $readme -Encoding UTF8

# This archive is the one redistributing ffmpeg and the GStreamer runtime, GPL and LGPL,
# so their notice travels with them rather than living in the repository alone
# (THIRD-PARTY-NOTICES.md states what each one is and where its source is).
# ffmpeg's own license text is copied where the build that supplied the executables carried one,
# which the release workflow's download does.
Copy-Item (Join-Path $root 'LICENSE') $stage
Copy-Item (Join-Path $root 'THIRD-PARTY-NOTICES.md') $stage
$ffmpegLicense = Join-Path $root 'build/windows/redist/ffmpeg-LICENSE.txt'
if (Test-Path $ffmpegLicense) {
    Copy-Item $ffmpegLicense $stage
}

# Compress-Archive refuses to overwrite,
# and a rebuilt package replaces the one built before it rather than failing here.
if (Test-Path $zip) { Remove-Item $zip }
Compress-Archive -Path $stage -DestinationPath $zip

Write-Host "packaged $zip"
