# Packaging and runtime dependencies

The app is two binaries: a headless Go backend that owns capture, encode, publish
and decode, and the Avalonia shell that talks to it over a local socket
(`ipc-api.md`). The backend shells out to `ffmpeg` and `ffplay` for capture,
encode, publish and the single-stream viewer, and links GStreamer through go-gst
for the receive pipelines a tile decodes through.
The binaries carry none of ffmpeg or GStreamer themselves, so a build is only
complete once those programs and libraries are reachable at run time.
This document describes what the app needs, how it finds it, and how each
distribution channel is expected to provide it.

## Runtime dependencies

| Program | Required | Purpose | Ships with |
|---------|----------|---------|------------|
| `ffmpeg` | yes | Capture, encode, publish. | The ffmpeg project. |
| `ffplay` | yes | The watch/viewer window. | The same ffmpeg distribution. |
| `gst-launch-1.0` | for the GStreamer publish engine | The engine that runs the portal and `*src` capture backends, the encode-rate probe on that engine, and the synthetic test streams. | GStreamer (`gstreamer` plus the plugin packages below). |
| GStreamer libraries | for the shell's tile grid | Linked into the backend through go-gst; a receive pipeline decodes and converts in-process, and the single-stream player needs none of them. | The same GStreamer packages, plus their development files at build time. |

`ffplay` is part of a full ffmpeg build, so a single ffmpeg install satisfies
both.
The GStreamer launcher is a separate program from the libraries the backend
links: a receive pipeline runs in-process through those, while a publish spawns
`gst-launch-1.0` the way it spawns `ffmpeg`, so a machine carrying the libraries
alone still fails every GStreamer publish at lookup, and one carrying the
launcher alone decodes no tile.
Minimal ffmpeg builds sometimes omit `ffplay` and the capture demuxers; the
build has to include the demuxer for the target platform (`ddagrab` on Windows,
`x11grab` and `kmsgrab` on Linux).
`captureArgs` in the `ffmpeg` package is the source of truth for which backends
the app invokes.

The Nix dev shell in `flake.nix` lists the same set, and is the reference for a
known-good dependency set during development.

Nothing beyond those two is needed to open a viewer, on any platform. A viewer
counts as connected once the relay reports a reader on the path, which the
backend's own relay poll already reports, and no window-system probe takes part in
that signal, so a package declares no dependency for it (`StartWatch` in
`internal/app/watch.go` states the same).

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

## How the app locates the programs it spawns

`FindExe` in the `ffmpeg` package resolves the executable name (`ffmpeg`,
`ffplay` or `gst-launch-1.0`, suffixed with `.exe` on Windows) in this order:

1. A copy sitting next to the app binary, in the same directory.
2. The first match on `PATH`.
3. Otherwise an error naming the missing program and the two places it is looked
   for.

Every GStreamer child goes through the same lookup, through `publish.FindGstExe`:
the publish engine, the encode probe and the test streams spawn one binary, and a
bare name handed to `exec.Command` would search `PATH` alone and pass over the copy
a bundle ships beside the app.
A bundled launcher is also given `GST_PLUGIN_PATH` at spawn
(`publish.GstChildEnv`), for the reason the grid sets it on itself: the prefix it
was built against exists on no machine but the build host.

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

The choice is a licensing one as well as a provisioning one.
A declared dependency is installed by the package manager under its own terms and the
package ships none of it.
A bundled copy is redistribution: ffmpeg is GPL and the GStreamer runtime is LGPL, so an
archive carrying them carries their notices and a source pointer too.
`THIRD-PARTY-NOTICES.md` is that record, per artifact, and the packaging scripts copy it
in beside the binaries.
The app's own code stays under Apache-2.0 either way, because a spawned program and a
loaded library are both boundaries the copyleft stops at.

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

`scripts/package-windows.ps1` assembles this channel, over what `task build:windows` and
`task bundle:windows` produce.
Windows has no dependency manager the installer can rely on, so the app ships
ffmpeg with it.
The Windows build task copies `ffmpeg.exe` and `ffplay.exe` into the output
directory next to the backend binary, where `FindExe` finds them first.
A development run needs no bundle, because `FindExe` falls back to `PATH`.
The GStreamer launcher is the exception: on Windows it comes from MSYS2, whose
prefix nothing puts on a normal `PATH`, so `task dev` appends `mingw64/bin` for the
run. It is appended rather than prepended because MSYS2 ships an ffmpeg of its own,
and a prefix in front would move every capture and encode onto it.
Use a recent third-party build (Gyan or BtbN); `ddagrab` needs a current ffmpeg.
No privilege step: `ddagrab` and `gdigrab` capture without elevation.

The backend links GStreamer through cgo for its receive pipelines, and no cross
toolchain builds that from Linux, so the Windows binary is built on Windows
against the toolchain MSYS2 provides.
Install MSYS2, then from its MINGW64 shell:

```bash
pacman -S mingw-w64-x86_64-{toolchain,pkgconf,go,gstreamer} \
          mingw-w64-x86_64-gst-{plugins-base,plugins-good,plugins-bad,plugins-ugly,plugins-rs,rtsp-server,libav}
```

GTK4, libadwaita and `gobject-introspection` are no longer inputs: they belonged to
the GTK grid, and the window is Avalonia's now.

Building needs no particular shell. The Windows build and bundle tasks reach the
toolchain, `MINGW_PREFIX` and MSYS2's own `sh` by path, from the `MSYS2_ROOT` variable in
`Taskfile.yml`, so they behave the same from Git Bash, PowerShell or cmd; set
`MSYS2_ROOT` for an install somewhere other than `C:/msys64`. What that variable exists
to absorb is the prefix on `PATH`: cgo looks `gcc` and `pkg-config` up by name, and the
GStreamer the built binary then loads is in the same directory. The Go is the machine's
own rather than MSYS2's, which ships a trimmed one that would need `GOROOT` named for it.
The prefix is appended for the same reason `task dev` appends it, and appending still
finds `gcc` and `pkg-config`, which nothing else on a Windows `PATH` provides.

Reaching MSYS2 by path rather than through the shell is what makes Git for Windows' Git
Bash safe to build from, which is the trap otherwise worth knowing: it is built on MSYS2,
reports `MSYSTEM=MINGW64` and prints the same `MINGW64` prompt, but its `/mingw64` is
Git's own prefix and carries neither the toolchain nor GStreamer. Its `ldd`, `cygpath` and
`MINGW_PREFIX` are Git's too, which is why `bundle:windows` names MSYS2's `sh` instead of
taking whichever is on `PATH`.

One Windows runtime quirk belongs to the running process rather than to the build. libsrt
names its threads by raising the debugger's thread-naming exception, which the Go runtime
has no owner for and ends the process over, so every pipeline carrying an `srtsrc` or
`srtsink` died as it was built, reported as `Exception 0x406d1388 ... signal arrived during
external code execution`. `internal/threadname` disarms it in the backend, which is now the
process that builds those elements in-process; a spawned `gst-launch-1.0` is a C program and
never sees it, which is why the same pipeline plays there. RTSP is unaffected because no
libsrt is loaded.

A build that reports `build constraints exclude all Go files` for a go-gst package ran
against a `go` that found no C compiler and disabled cgo, which excludes every file in a
binding whose files are all cgo. The extra tell is a `go: downloading go1.26.4` line, which
a `go` newer than `go.mod` would never print, betraying the Windows Go rather than MSYS2's.
The build task asks for cgo outright so this surfaces as the missing compiler instead, and
`cmd //c "where go gcc"` shows which toolchain a native child of the current shell resolves.

A receive pipeline ends in `appsink`, which is core, so the tile path needs no plugin of
its own; what it needs is the source element of the leg it watches on.
`whepsrc` comes from `gst-plugins-rs`; SRT and RTMP come from `gst-plugins-bad`, so a
bundle missing one of those packages is a bundle missing a transport.
RTSP is split across two packages by direction: `rtspsrc`, which a receive pipeline and
the test streams watch with, is in `gst-plugins-good`, while `rtspclientsink`, which
every RTSP publish and every test stream sends with, is in `gst-rtsp-server`.
That package is not pulled in by any of the `gst-plugins-*` ones, and its absence
shows up as `no element "rtspclientsink"` the first time a test stream or an RTSP
publish starts, long after the build succeeded.
`x264enc` comes from `gst-plugins-ugly`, which the GStreamer publish engine and the
encode-rate probe both need.
The render chains name elements of their own: `glupload`, `glcolorconvert` and
`glcolorscale` come from `gst-plugins-base`, and a machine registering none of them
falls back to the CPU chain rather than failing (`viewer-architecture.md`).

A machine that runs the app has no MSYS2, so the bundle task
(`scripts/bundle-windows.sh`) copies the runtime beside the binaries: the DLL
closure of the backend, of `gst-launch-1.0.exe` and of every installed plugin flat
next to the executables, where the Windows loader looks first; `gst-launch-1.0.exe`
itself, which the backend spawns for every GStreamer publish; and the plugins under
`gstreamer-1.0`.

The GTK-era inputs are gone with the window that needed them: no `gschemas.compiled`,
no Adwaita icon theme, no Cantarell and no generated `fonts.conf`, because the shell
bundles its own font and icon set (`avalonia/README.md`) and no GLib schema is read.

GStreamer looks for plugins in the prefix it was built against, which is a path
that exists on no machine but the one that built it, so the backend prepends its own
`gstreamer-1.0` directory to `GST_PLUGIN_PATH` before it initializes the library, and
passes the same to every `gst-launch-1.0` child (`publish.GstChildEnv`).
A directory that is not there is an ordinary installation, which is left to find
its plugins itself.

### Arch Linux (AUR)

`packaging/arch/PKGBUILD` is this channel.
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

`nix/package.nix` is this channel, exported from the flake as `packages.default`.
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
The full derivation builds the Go backend with `buildGoModule` and the Avalonia
shell with the .NET SDK, matching `Taskfile.yml`; the GStreamer libraries the
backend links are ordinary build inputs, and the plugin path is set by the wrapper
the same way ffmpeg's is.
`kmsgrab` capability is a system concern, handled by the `security.wrappers` entry
above, not by the package.

### Debian, Fedora, and other package managers

Same model as Arch: declare the dependency, do not bundle.

- Debian: `Depends: ffmpeg`, in a `.deb` no recipe here builds. A Debian install takes
  the tarball below instead, which is why `docs/install.md` names the apt packages it
  has to be given.
- Fedora: `Requires: ffmpeg`. `packaging/fedora/screen-sharing.spec` is that channel,
  and it requires the two paths rather than the name, because `ffmpeg-free` and RPM
  Fusion's `ffmpeg` both provide them and either serves.

Distributions with no package here take the tarball `scripts/package-linux.sh` builds,
which carries both binaries and the .NET runtime and takes ffmpeg and GStreamer from the
distribution.

### AppImage and Flatpak

Neither format has a recipe here, so what follows is the model one would take rather
than something to run.
Both are self-contained, so ffmpeg goes inside the image next to the binary, which puts
them on the bundling side of the rule above and hands them the obligations the Windows
archive carries (`THIRD-PARTY-NOTICES.md`).
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
