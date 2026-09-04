#!/usr/bin/env bash
#
# bundle-windows.sh - put the GStreamer runtime the backend links,
# and the command-line tools it spawns, next to the built binaries,
# so both run on a machine without MSYS2.
#
# Run from the MSYS2 MINGW64 shell, whose ldd resolves the mingw DLLs:
#
#   task bundle:windows
#   sh scripts/bundle-windows.sh build/bin
#
# Produces the layout docs/packaging.md describes:
# DLLs and the launcher beside the binary, and GStreamer plugins in gstreamer-1.0/.
set -euo pipefail

bin=${1:?usage: bundle-windows.sh <bin-directory>}
prefix=${MINGW_PREFIX:-/mingw64}

# Both inputs reach this script in whichever form the caller happened to hold:
# Task interpolates ROOT_DIR as a native Windows path,
# and MSYS2 rewrites MINGW_PREFIX into one on its way to a native task.exe,
# while a MINGW64 shell passes POSIX paths for both.
# ldd reads POSIX alone and answers in the mount form alone,
# so handed "C:\dir\backend.exe" it resolves nothing
# and, with its errors discarded below, reports an empty closure rather than a failure.
# Each input is therefore pinned to one form here:
# the directory to POSIX, because ldd is given it,
# and the prefix to Windows, because ldd's answers are converted to that form to compare against it.
if ! command -v cygpath >/dev/null 2>&1; then
    echo "cygpath not found: run this from the MSYS2 MINGW64 shell, or with MSYS2's usr/bin on PATH as task bundle:windows does" >&2
    exit 1
fi
bin=$(cygpath -u "$bin")
prefix=$(cygpath -m "$prefix")

backend="$bin/mirrorme-backend.exe"

# Both tools, the inspector as much as the launcher.
# The launcher runs a pipeline, and the encoder probe asks the inspector whether an element exists:
# a missing inspector reads as an install carrying no GStreamer tooling,
# which greys the whole GStreamer engine, every codec on it,
# and the Desktop Duplication capture backend with it (backend/internal/encoders, gstAvailable).
# A zip with the launcher alone therefore ships a GStreamer runtime the app refuses to use.
tools=(gst-launch-1.0.exe gst-inspect-1.0.exe)

# GLib carries no TLS of its own and takes it from a GIO module, glib-networking's.
# Without it every rtsps:// leg fails at the connect, on both sides of the app:
# the test streams and every RTSP publish through rtspclientsink, every tile through rtspsrc,
# and nothing on the way names TLS. gst-launch reports "Failed to connect. (Generic error)",
# and the relay logs a connection that opened and closed
# (nix/package.nix carries the same module for the same reason).
#
# The gnutls module alone. glib-networking also ships two proxy modules,
# and GIO loads every module it finds in a directory:
# the libproxy one wants a library nothing here links,
# and the GNOME one aborts the process over the GSettings schemas a bundle carries none of.
#
# Beside the binary under a directory of the bundle's own, named to every process through
# GIO_EXTRA_MODULES the way the plugins are through GST_PLUGIN_PATH
# (backend/internal/gstbundle answers both, for this process and for the children).
# Not the lib/gio/modules GIO scans unaided: on Windows it derives that directory from where its
# own DLL sits and strips a trailing "bin" or "lib" first, so the same layout loads under one
# directory name and silently does not under another, build/bin among them.
gio_source_dir=lib/gio/modules
gio_module_dir=gio-modules
gio_modules=(libgiognutls.dll)

if [ ! -f "$backend" ]; then
    echo "$backend does not exist: build it with 'task build:windows' first" >&2
    exit 1
fi
if [ ! -d "$prefix/lib/gstreamer-1.0" ]; then
    echo "$prefix/lib/gstreamer-1.0 does not exist: install the MSYS2 gstreamer packages first" >&2
    exit 1
fi
for tool in "${tools[@]}"; do
    if [ ! -f "$prefix/bin/$tool" ]; then
        echo "$prefix/bin/$tool does not exist: install the MSYS2 gstreamer packages first" >&2
        exit 1
    fi
done
for module in "${gio_modules[@]}"; do
    if [ ! -f "$prefix/$gio_source_dir/$module" ]; then
        echo "$prefix/$gio_source_dir/$module does not exist: install the MSYS2 glib-networking package first" >&2
        exit 1
    fi
done

# Both sides of the app's GStreamer use ship here.
# The backend links the library for the receive pipelines (backend/internal/receive),
# and spawns the launcher for a GStreamer publish, for the encode probe and for the test streams
# (backend/internal/publish, GstExe).
# The programs go beside the binaries, where the app looks first (ffmpeg.FindExe),
# and GST_PLUGIN_PATH names the plugins both sides load:
# backend/internal/gstbundle answers where they went, for this process and for the children.
for tool in "${tools[@]}"; do
    cp -f "$prefix/bin/$tool" "$bin/"
done

# Every installed plugin rather than a chosen subset.
# Which ones a run needs follows from the transport and the codec being published or watched,
# so trimming the set is a packaging decision that needs the shipped transports in front of it.
mkdir -p "$bin/gstreamer-1.0"
cp -f "$prefix"/lib/gstreamer-1.0/*.dll "$bin/gstreamer-1.0/"

# The TLS module, in the directory the backend names to GIO (gio_module_dir above).
mkdir -p "$bin/$gio_module_dir"
for module in "${gio_modules[@]}"; do
    cp -f "$prefix/$gio_source_dir/$module" "$bin/$gio_module_dir/"
done

# The DLLs come from the closure of the binary, of every plugin and of the TLS module, walked to a fixed point:
# a plugin pulls in libraries the backend itself never links,
# and ldd resolves one level per call on some builds.
#
# Flat beside the binary, where the Windows loader looks first
# for the process and for anything the process loads later, plugins included.
declare -A seen
copied=0
queue=("$backend")
for tool in "${tools[@]}"; do
    queue+=("$bin/$tool")
done
for plugin in "$bin"/gstreamer-1.0/*.dll; do
    queue+=("$plugin")
done
for module in "${gio_modules[@]}"; do
    queue+=("$bin/$gio_module_dir/$module")
done

while [ ${#queue[@]} -gt 0 ]; do
    current=("${queue[@]}")
    queue=()
    for file in "${current[@]}"; do
        while read -r dll; do
            if [ -z "$dll" ] || [ -n "${seen[$dll]:-}" ]; then
                continue
            fi
            seen[$dll]=1
            copied=$((copied + 1))
            cp -f "$dll" "$bin/"
            queue+=("$bin/$(basename "$dll")")
        done < <(ldd "$file" 2>/dev/null | awk '$3 != "" { print $3 }' |
            cygpath -m -f - 2>/dev/null |
            awk -v p="$prefix/" 'index(tolower($0), tolower(p)) == 1')
    done
done

# The backend links GStreamer, so an empty closure is a broken walk
# rather than a binary that needs nothing,
# and a bundle shipped without it fails at the user's end.
if [ "$copied" -eq 0 ]; then
    echo "no libraries found under $prefix: ldd resolved nothing for $backend" >&2
    exit 1
fi

# Counted by the shell rather than by find:
# Windows carries a find.exe of its own, ahead of MSYS2's on the PATH Task hands this,
# and it answers "File not found - *.dll" and a count of zero over a directory holding plugins.
plugins=("$bin"/gstreamer-1.0/*.dll)

echo "bundled $copied libraries, ${tools[*]}, ${#plugins[@]} GStreamer plugins and ${gio_modules[*]} into $bin"
