# Packaging and runtime dependencies

The desktop app is a Wails binary that shells out to `ffmpeg` and `ffplay` for
every capture, encode, publish and watch operation.
The binary carries none of that itself, so a build is only complete once those
programs are reachable at run time.
This document describes what the app needs, how it finds it, and how each
distribution channel is expected to provide it.

## Runtime dependencies

| Program | Required | Purpose | Ships with |
|---------|----------|---------|------------|
| `ffmpeg` | yes | Capture, encode, publish. | The ffmpeg project. |
| `ffplay` | yes | The watch/viewer window. | The same ffmpeg distribution. |

`ffplay` is part of a full ffmpeg build, so a single ffmpeg install satisfies
both.
Minimal ffmpeg builds sometimes omit `ffplay` and the capture demuxers; the
build has to include the demuxer for the target platform (`ddagrab` on Windows,
`x11grab` and `kmsgrab` on Linux).
`captureArgs` in the `ffmpeg` package is the source of truth for which backends
the app invokes.

The Nix dev shell in `flake.nix` lists the same set, and is the reference for a
known-good dependency set during development.

Nothing beyond those two is needed to open a viewer, on any platform. A viewer
counts as connected once the relay reports a reader on the path, which `Live()`
already polls, and no window-system probe takes part in that signal, so a package
declares no dependency for it (`StartWatch` in `desktop/app_watch.go` states the
same).

### The AMD AMF runtime

The `*_amf` encoders are the one encoder family that needs more than ffmpeg
itself.
They are compiled into the build unconditionally and load AMD's closed-source
`libamfrt64.so.1` at run time, by soname, so the library has to be on the loader
path rather than merely installed somewhere.
AMD ships it in the proprietary part of its driver package
(`amf-amdgpu-pro`), for x86_64 only, and AMF reaches the encoder hardware through
Vulkan, so the card also needs a Vulkan driver.
A missing runtime is not a failure the app has to handle: the encoder refuses to
open, `encoders.Detect` sees that, and the settings form greys the family exactly
as it does on a machine with no AMD card.
The dev shell puts the package on `LD_LIBRARY_PATH` for the same reason a
packaged build has to.

## How the app locates ffmpeg

`FindExe` in the `ffmpeg` package resolves the executable name (`ffmpeg` or
`ffplay`, suffixed with `.exe` on Windows) in this order:

1. A copy sitting next to the app binary, in the same directory.
2. The first match on `PATH`.
3. Otherwise an error instructing the user to install ffmpeg or drop it beside
   the app.

Two provisioning models follow from that rule.
A channel either bundles ffmpeg next to the binary for a self-contained install,
or declares a dependency on a system ffmpeg and relies on `PATH`.
The bundled copy always wins, so a packaged system ffmpeg and a bundled one never
conflict: the bundle decides.

The rest of this document picks one model per channel.
The convention is: bundle where the platform has no package manager to lean on
(Windows, AppImage, Flatpak), declare a dependency where it does (Arch, NixOS,
Debian, Fedora).
Bundling a private ffmpeg into a distro package is the opposite of what packagers
expect and is a common reason for rejection.

## Screen-capture privileges on Linux

The capture backend, not the app, decides whether elevated privileges are
required.

| Backend | Session | Privilege |
|---------|---------|-----------|
| `x11grab` | X11, or XWayland windows only | none |
| `kmsgrab` | Wayland or a bare KMS console | `CAP_SYS_ADMIN` |
| `ddagrab`, `gdigrab` | Windows | none |

`x11grab` captures the whole X screen with no special rights and is the simplest
Linux path.
On a Wayland session it only sees XWayland client windows, not the Wayland
desktop, so it is not a general capture backend there.

`kmsgrab` reads framebuffers straight from the DRM subsystem and requires
`CAP_SYS_ADMIN`.
There is no unprivileged variant of that API.
It is the only backend in a stock ffmpeg build that captures a full Wayland
desktop.

### Granting the capability

`CAP_SYS_ADMIN` is close to root: a process holding it can perform a wide range
of kernel operations, not just read framebuffers.
Do not grant it to a shared or system-wide ffmpeg.
Any local user who can run that binary would inherit the capability through
ffmpeg's many input handlers.
When the capability is needed, it goes on the app's own bundled copy of ffmpeg,
or on a dedicated wrapper, never on `/usr/bin/ffmpeg`.

Manual grant, on the bundled copy only:

```bash
sudo setcap cap_sys_admin+ep /path/to/app/ffmpeg
```

On NixOS the store is read-only, so `setcap` cannot target the store path.
A `security.wrappers` entry in the system configuration produces a setcap wrapper
under `/run/wrappers/bin`:

```nix
security.wrappers.screenshare-ffmpeg = {
  owner = "root";
  group = "root";
  capabilities = "cap_sys_admin+ep";
  source = "${pkgs.ffmpeg-full}/bin/ffmpeg";
};
```

The app then invokes `/run/wrappers/bin/screenshare-ffmpeg` for capture.

### The portal alternative

The standards-track way to capture a Wayland desktop is the
`org.freedesktop.portal.ScreenCast` portal backed by PipeWire, the same path OBS
and browsers use.
It grants no static capability: the user picks the surface in a portal dialog and
the compositor hands back a PipeWire stream.
ffmpeg consumes it through the `pipewiregrab` filter, which is present only in an
ffmpeg built with `--enable-libpipewire`.
The stock `ffmpeg-full` in nixpkgs is not built with it, so this path needs a
custom ffmpeg build before it can replace `kmsgrab`.

### Capture device selection

`captureArgs` passes a fixed DRM node to `kmsgrab`.
That node is not stable across machines: the boot `simple-framebuffer` can occupy
`card0` while the real GPU lands on `card1`, and the numbering depends on driver
load order.
Portable capture picks the active card node at run time (the DRM node whose
driver is a real GPU rather than `simple-framebuffer`) instead of hard-coding one.

## Per-channel packaging

### Windows (self-contained)

Windows has no dependency manager the installer can rely on, so the app ships
ffmpeg with it.
`task build:windows` copies `ffmpeg.exe` and `ffplay.exe` into the Wails output
directory next to `screen-sharing.exe`, where `FindExe` finds them first.
The copy belongs to that task rather than to a Wails post-build hook: Wails execs
a hook instead of running it through a shell, so a hook is bound to the tools its
host resolves, and one written for a Linux cross-build aborts every build on a
Windows host, `wails dev` included.
A development run on either host needs no bundle, because `FindExe` falls back to
`PATH`.
Use a recent third-party build (Gyan or BtbN); `ddagrab` needs a current ffmpeg.
No privilege step: `ddagrab` and `gdigrab` capture without elevation.

The native grid is a second binary with a second dependency set: GTK4, libadwaita
and GStreamer, all linked through cgo.
No cross toolchain builds those from Linux, so `task build:windows` produces the
app alone and the grid is built on Windows, against the toolchain MSYS2 provides.
Install MSYS2, then from its MINGW64 shell:

```bash
pacman -S mingw-w64-x86_64-{toolchain,pkgconf,go,gtk4,libadwaita,gobject-introspection} \
          mingw-w64-x86_64-gst-{plugins-base,plugins-good,plugins-bad,plugins-ugly,plugins-rs,libav}
```

`gobject-introspection` is a build input rather than a runtime one: gotk4 resolves it
through cgo's `pkg-config`, the same as the nativegrid dev shell in `flake.nix` does.

Building needs no particular shell. `task nativegrid` and `task bundle:windows` reach the
toolchain, `MINGW_PREFIX` and MSYS2's own `sh` by path, from the `MSYS2_ROOT` variable in
`Taskfile.yml`, so they behave the same from Git Bash, PowerShell or cmd; set
`MSYS2_ROOT` for an install somewhere other than `C:/msys64`. Two details that variable
exists to absorb: the toolchain loads its own DLLs out of its prefix and cgo looks `gcc`
up by name, so the prefix has to be on `PATH`, and the Go MSYS2 ships is trimmed and
needs `GOROOT` named for it.

Reaching MSYS2 by path rather than through the shell is what makes Git for Windows' Git
Bash safe to build from, which is the trap otherwise worth knowing: it is built on MSYS2,
reports `MSYSTEM=MINGW64` and prints the same `MINGW64` prompt, but its `/mingw64` is
Git's own prefix and carries neither the toolchain nor GStreamer. Its `ldd`, `cygpath` and
`MINGW_PREFIX` are Git's too, which is why `bundle:windows` names MSYS2's `sh` instead of
taking whichever is on `PATH`.

One Windows runtime quirk belongs to the grid rather than to its build. libsrt names its
threads by raising the debugger's thread-naming exception, which the Go runtime has no
owner for and ends the process over, so every pipeline carrying an `srtsrc` or `srtsink`
died as it was built, reported as `Exception 0x406d1388 ... signal arrived during external
code execution`. `internal/threadname` disarms it for the grid; the same exception reaches
any Go process that builds one of those elements, the app's GStreamer publish engine
included. A C program never sees it, which is why `gst-launch-1.0` plays the same
pipeline, and RTSP is unaffected because no libsrt is loaded.

A build that reports `build constraints exclude all Go files` for a gotk4 or go-gst
package ran against a `go` that found no C compiler and disabled cgo, which excludes
every file in a binding whose files are all cgo. The extra tell is a `go: downloading
go1.26.4` line, which a `go` newer than `go.mod` would never print, betraying the Windows
Go rather than MSYS2's. `task nativegrid` asks for cgo outright so this surfaces as the
missing compiler instead, and `cmd //c "where go gcc"` shows which toolchain a native
child of the current shell resolves.

`gtk4paintablesink`, the element every tile renders into, comes from
`gst-plugins-rs`, as does `whepsrc`; RTSP comes from `gst-plugins-good` and SRT
and RTMP from `gst-plugins-bad`, so a bundle missing one of those packages is a
bundle missing a transport.
`x264enc` comes from `gst-plugins-ugly`, which the grid's own demo run needs as well as
the GStreamer publish engine, so its absence shows up as `no element "x264enc"` on the
first tile rather than at publish time.

A machine that runs the app has no MSYS2, so `bundle:windows`
(`scripts/bundle-windows.sh`) copies the runtime beside the binaries: the DLL
closure of the grid and of every installed plugin flat next to the executables,
where the Windows loader looks first; the plugins under `gstreamer-1.0`; and
GLib's `gschemas.compiled` under `share/glib-2.0/schemas`, which GTK aborts
without.
GStreamer looks for plugins in the prefix it was built against, which is a path
that exists on no machine but the one that built it, so the grid prepends its own
`gstreamer-1.0` directory to `GST_PLUGIN_PATH` before it initializes the library.
A directory that is not there is an ordinary installation, which is left to find
its plugins itself.

### Arch Linux (AUR)

Declare ffmpeg as a runtime dependency and let pacman provide it.
Do not bundle.

```bash
depends=('ffmpeg')
```

`ffmpeg` pulls in `ffplay`.
The `kmsgrab` capability is not something a package should grant silently; note
it in the package description and leave the `setcap` step, or the portal, to the
user.

### NixOS / nixpkgs

The derivation wraps the app binary so ffmpeg and ffplay are on its `PATH`, using
`makeWrapper`:

```nix
nativeBuildInputs = [ makeWrapper ];

postInstall = ''
  wrapProgram $out/bin/screen-sharing \
    --prefix PATH : ${lib.makeBinPath [ ffmpeg-full ]}
'';
```

The runtime dependency is expressed in the closure, not bundled by hand, which is
what nixpkgs expects.
The full derivation also builds the React frontend (`buildNpmPackage`) and the Go
backend (`buildGoModule` with the `webkit2_41` tag), matching `Taskfile.yml`.
`kmsgrab` capability is a system concern, handled by the `security.wrappers` entry
above, not by the package.

### Debian, Fedora, and other package managers

Same model as Arch: declare the dependency, do not bundle.

- Debian: `Depends: ffmpeg`.
- Fedora: `Requires: ffmpeg`.

### AppImage and Flatpak

These formats are self-contained, so ffmpeg goes inside the image next to the
binary.
A Flatpak additionally captures through the portal, so it needs no capability and
runs in the sandbox unmodified; the manifest requests the `ScreenCast` portal
rather than raw DRM access.

## Verifying a build

```bash
# The app resolves ffmpeg (bundled copy or PATH).
ffmpeg -version

# The encoder probe the app runs at startup, ffmpeg engine: a codec must encode one frame.
ffmpeg -hide_banner -f lavfi -i color=c=black:s=64x64 -frames:v 1 -c:v libx264 -f null -

# The same probe on the GStreamer publish engine (the portal capture backend): the encoder
# element must be in the plugin registry.
gst-inspect-1.0 --exists x264enc

# kmsgrab reaches the framebuffer (fails without CAP_SYS_ADMIN):
ffmpeg -hide_banner -device /dev/dri/card1 -f kmsgrab -i - -frames:v 1 -f null -
```

A successful `kmsgrab` frame confirms both the device node and the capability are
correct; `No handle set on framebuffer` means the capability is missing.
