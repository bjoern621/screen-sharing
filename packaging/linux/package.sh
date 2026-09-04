#!/usr/bin/env bash
#
# package.sh - assemble the Linux release directory and its tarball.
#
#   task package:linux
#   VERSION=0.6.1 sh packaging/linux/package.sh [output-directory]
#
# One directory holds both binaries, the layout the shell's own lookup expects:
# it starts the backend when nothing answers on the control endpoint,
# and finds it beside itself (avalonia/ScreenShare.App/Backend/BackendProcess.cs).
#
# The shell is published self-contained, so the machine needs no .NET install.
# ffmpeg and GStreamer are not bundled:
# this artifact is for distributions with no package of their own here,
# and both come from theirs (docs/packaging.md, docs/install.md).
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
# The run names the version and no file in the tree does
# (.github/workflows/version.yml).
# Unset, the archive and the binary inside it both call themselves dev,
# which is the mark of a build nobody released (backend/cmd/backend/main.go).
version=${VERSION:-dev}
out=${1:-$root/build/dist}
name=mirrorme-$version-linux-x86_64-portable
stage=$out/$name

# A stale directory would ship whatever a previous version left in it,
# and publish overwrites rather than clears.
rm -rf "$stage"
mkdir -p "$stage"

# backend/internal/receive links GStreamer through cgo.
# Go turns cgo off by itself when it finds no C compiler,
# and the failure then names a package that looks unrelated to it.
CGO_ENABLED=1 go build -C "$root/backend" \
    -ldflags "-X main.version=$version" \
    -o "$stage/mirrorme-backend" ./cmd/backend

dotnet publish "$root/avalonia/ScreenShare.App/ScreenShare.App.csproj" \
    --configuration Release \
    --runtime linux-x64 \
    --self-contained true \
    --output "$stage"

# The one file a reader of this archive sees before running anything.
# It names the two programs and points at the document listing what the distribution provides,
# rather than restating that list here, where it would drift.
cat > "$stage/README.txt" <<EOF
MirrorMe $version

Run ./mirrorme. It starts the backend beside it, so there is nothing
else to launch.

ffmpeg (with ffplay) and GStreamer come from the distribution and have to be
installed first. What to install, and how to reach a relay:
https://github.com/bjoern621/screen-sharing/blob/main/docs/install.md
EOF

# The license travels with the binaries,
# and the notices name what the .NET publish output above put beside them.
# ffmpeg and GStreamer are not in this archive,
# so nothing here carries an obligation past attribution.
cp "$root/LICENSE" "$root/THIRD-PARTY-NOTICES.md" "$stage/"

tar -czf "$out/$name.tar.gz" -C "$out" "$name"

echo "packaged $out/$name.tar.gz"
