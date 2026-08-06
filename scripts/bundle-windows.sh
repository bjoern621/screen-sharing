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
# GStreamer plugins in gstreamer-1.0/, and under share/ the compiled GLib
# schemas together with the icon theme and the font the window's look is pinned
# to (nativegrid/README.md, "One look on both platforms").
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

# The look's inputs travel with the bundle too, for the reason the whole runtime
# does: a bundle that resolves them against the machine it lands on is a bundle
# that looks like that machine, and this window is supposed to look like itself
# on both platforms (nativegrid/internal/gtkenv states the other half).
#
# The icon theme is what libadwaita's own widgets draw from, the window buttons
# and the dropdown chevrons among them; the grid's own icons are vendored SVGs
# and need none of it. hicolor comes along as the fallback every theme inherits
# from and every lookup that misses ends in.
for theme in Adwaita hicolor; do
    if [ ! -d "$prefix/share/icons/$theme" ]; then
        echo "$prefix/share/icons/$theme does not exist: install mingw-w64-x86_64-adwaita-icon-theme first" >&2
        exit 1
    fi
done
mkdir -p "$bin/share/icons"
cp -rf "$prefix/share/icons/Adwaita" "$prefix/share/icons/hicolor" "$bin/share/icons/"

# The font, and the configuration that makes it the one Pango finds. Neither can
# be left to the machine: Windows resolves families through GDI unless told
# otherwise, and its fontconfig scans the system font directory alone, so a
# bundle carrying the file without the configuration ships a font nothing looks
# at. FONTCONFIG_FILE names this file at startup (gtkenv_windows.go).
font=$prefix/share/fonts/cantarell/Cantarell-VF.otf
if [ ! -f "$font" ]; then
    echo "$font does not exist: install mingw-w64-x86_64-cantarell-fonts first" >&2
    exit 1
fi
mkdir -p "$bin/share/fonts/cantarell"
cp -f "$font" "$bin/share/fonts/cantarell/"

# Every path in it is relative to the file itself, so a bundle that is moved or
# renamed still resolves its own fonts. The cache is the exception and goes to
# the user's own directory, because the one the bundle sits in is often not
# writable and a cache that cannot be written is rebuilt on every start.
cat > "$bin/share/fonts/fonts.conf" <<'CONF'
<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "urn:fontconfig:fonts.dtd">
<!-- The native grid's fonts. Written by scripts/bundle-windows.sh; edit it there. -->
<fontconfig>
  <!-- This file's own directory, scanned through: the families that shipped. -->
  <dir prefix="relative">.</dir>
  <!-- Behind them, so a glyph no bundled family carries still resolves. -->
  <dir>WINDOWSFONTDIR</dir>
  <cachedir prefix="xdg">fontconfig</cachedir>

  <!-- How the Linux side turns an outline into pixels, stated so this side
       does the same. Grayscale rather than a subpixel order, because subpixel
       antialiasing rasterizes for one panel and these are two machines. -->
  <match target="font">
    <edit name="antialias" mode="assign"><bool>true</bool></edit>
    <edit name="hinting" mode="assign"><bool>true</bool></edit>
    <edit name="hintstyle" mode="assign"><const>hintslight</const></edit>
    <edit name="rgba" mode="assign"><const>none</const></edit>
  </match>
</fontconfig>
CONF

# The grid links GTK4 and GStreamer, so an empty closure is a broken walk rather than a
# binary that needs nothing, and a bundle shipped without it fails at the user's end.
if [ "$copied" -eq 0 ]; then
    echo "no libraries found under $prefix: ldd resolved nothing for $grid" >&2
    exit 1
fi

echo "bundled $copied libraries, $(find "$bin/gstreamer-1.0" -name '*.dll' | wc -l) GStreamer plugins, the Adwaita icon theme and Cantarell into $bin"
