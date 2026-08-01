#!/usr/bin/env bash
#
# bundle-windows.sh - put the GTK4 and GStreamer runtime the native grid links
# next to the built binaries, so the app runs on a machine without MSYS2.
#
# Run from the MSYS2 MINGW64 shell, whose ldd resolves the mingw DLLs:
#
#   task bundle:windows
#   sh scripts/bundle-windows.sh desktop/build/bin
#
# It produces the layout docs/packaging.md describes: DLLs beside the binary,
# GStreamer plugins in gstreamer-1.0/, GLib's compiled schemas under share/.
set -euo pipefail

bin=${1:?usage: bundle-windows.sh <bin-directory>}
prefix=${MINGW_PREFIX:-/mingw64}

# Both inputs reach this script in whichever form the caller happened to hold: Task
# interpolates ROOT_DIR as a native Windows path, and MSYS2 rewrites MINGW_PREFIX into
# one on its way to a native task.exe, while a MINGW64 shell passes POSIX paths for
# both. ldd only reads POSIX, and only ever answers in the mount form, so handed
# "C:\dir\grid.exe" it resolves nothing and, with its errors discarded below, reports an
# empty closure rather than a failure. Each input is therefore pinned to one form here:
# the directory to POSIX, because ldd is given it, and the prefix to Windows, because
# ldd's answers are converted to that form to be compared against it.
if ! command -v cygpath >/dev/null 2>&1; then
    echo "cygpath not found: run this from the MSYS2 MINGW64 shell" >&2
    exit 1
fi
bin=$(cygpath -u "$bin")
prefix=$(cygpath -m "$prefix")

grid="$bin/screenshare-nativegrid.exe"

if [ ! -f "$grid" ]; then
    echo "$grid does not exist: build it with 'task nativegrid' first" >&2
    exit 1
fi
if [ ! -d "$prefix/lib/gstreamer-1.0" ]; then
    echo "$prefix/lib/gstreamer-1.0 does not exist: install the MSYS2 gstreamer packages first" >&2
    exit 1
fi

# Every installed plugin rather than a chosen subset. Which ones a run needs
# follows from the transport and the codec of whatever is being watched, so
# trimming the set is a packaging decision that needs the shipped transports in
# front of it.
mkdir -p "$bin/gstreamer-1.0"
cp -f "$prefix"/lib/gstreamer-1.0/*.dll "$bin/gstreamer-1.0/"

# The DLLs come from the closure of the binary and of every plugin, walked to a
# fixed point: a plugin pulls in libraries the grid itself never links, and ldd
# resolves one level per call on some builds.
#
# Flat beside the binary, which is where the Windows loader looks first for the
# process and for anything the process loads later, plugins included.
declare -A seen
copied=0
queue=("$grid")
for plugin in "$bin"/gstreamer-1.0/*.dll; do
    queue+=("$plugin")
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

# GTK reads its settings out of GSettings and aborts when the schemas are
# missing, so the compiled schema file travels with the bundle. GLib resolves
# share/ relative to the directory its own DLL sits in, which is why this layout
# needs no variable pointing at it.
mkdir -p "$bin/share/glib-2.0/schemas"
cp -f "$prefix/share/glib-2.0/schemas/gschemas.compiled" "$bin/share/glib-2.0/schemas/"

# The grid links GTK4 and GStreamer, so an empty closure is a broken walk rather than a
# binary that needs nothing, and a bundle shipped without it fails at the user's end.
if [ "$copied" -eq 0 ]; then
    echo "no libraries found under $prefix: ldd resolved nothing for $grid" >&2
    exit 1
fi

echo "bundled $copied libraries and $(find "$bin/gstreamer-1.0" -name '*.dll' | wc -l) GStreamer plugins into $bin"
