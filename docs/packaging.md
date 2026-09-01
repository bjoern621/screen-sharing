# Packaging and runtime dependencies

Two binaries: a headless Go backend owning capture, encode, publish and decode, and the Avalonia shell talking to it over a local socket (`ipc-api.md`).
The backend shells out to `ffmpeg` and `ffplay`, and links GStreamer through go-gst for the receive pipelines a tile decodes through.
Neither binary carries ffmpeg or GStreamer, so a build is complete only once those are reachable at run time.

## Runtime dependencies

| Program | Required | Purpose | Ships with |
|---------|----------|---------|------------|
| `ffmpeg` | yes | Capture, encode, publish. | The ffmpeg project. |
| `ffplay` | yes | The watch/viewer window. | The same ffmpeg distribution. |
| `gst-launch-1.0` | for the GStreamer publish engine | The portal and `*src` capture backends, the encode-rate probe on that engine, the synthetic test streams. | GStreamer, plus the plugin packages below. |
| GStreamer libraries | for the shell's tile grid | Linked into the backend through go-gst; a receive pipeline decodes and converts in-process. The single-stream player needs none of them. | The same packages, plus development files at build time. |

`ffplay` is part of a full ffmpeg build, so one install satisfies both.

The launcher and the libraries the backend links are separate programs.
A receive pipeline runs in-process through the libraries.
A publish spawns `gst-launch-1.0` the way it spawns `ffmpeg`.
So the libraries alone fail every GStreamer publish at lookup, and the launcher alone decodes no tile.

Minimal ffmpeg builds sometimes omit `ffplay` and the capture demuxers.
The build has to include the demuxer for the target platform: `ddagrab` on Windows, `x11grab` and `kmsgrab` on Linux.
Which backends the app invokes: `captureArgs` in the `ffmpeg` package.
`flake.nix`'s dev shell lists the same set.

Opening a viewer needs nothing beyond those two, on any platform.
A viewer counts as connected once the relay reports a reader on the path, which the backend's relay poll already reports.
No window-system probe takes part, so a package declares no dependency for one (`backend/internal/app/watch.go`).

### The AMD AMF runtime

The `*_amf` encoders are the one family needing more than ffmpeg itself.
Compiled in unconditionally, they load AMD's closed-source `libamfrt64.so.1` at run time by soname, so it has to be on the loader path rather than merely installed.
AMD ships it in the proprietary part of its driver package (`amf-amdgpu-pro`), x86_64 only.
AMF reaches the encoder hardware through Vulkan, so the card also needs a Vulkan driver.
A missing runtime is not a failure the app handles: the encoder refuses to open, `encoders.Detect` sees that, and the form greys the family exactly as on a machine with no AMD card.

`LD_LIBRARY_PATH` reaches an unprivileged `ffmpeg` alone.
The kmsgrab wrapper carries file capabilities, which puts it in glibc's secure-execution mode where that variable is ignored, so a runtime delivered through the environment never reaches its loader.
Untreated that is a wrong answer rather than a missing encoder: `encoders.Detect` probes the unprivileged binary, finds the runtime, and the form offers a family that dies at launch under kmsgrab.
The `amf` option of `nix/screen-share.nix` records the runtime on `libavutil`'s `RUNPATH` instead, which the loader does honour.
Ordinary variables survive, so the oneVPL runtime behind QSV, located through `ONEVPL_SEARCH_PATH`, is unaffected.

## Version pinning

Every dependency this repository resolves is pinned, so the tree decides which toolchain a build uses rather than the day it runs.

| What | Pinned by | Moved by |
|------|-----------|----------|
| Nix package set: ffmpeg, GStreamer, the .NET SDK, Go, AMF, MediaMTX | input revisions in `flake.nix`, recorded in `flake.lock` | editing the revision, then `nix flake lock` |
| Go modules | `backend/go.sum` | `go get`, then `go mod tidy` |
| NuGet packages | exact versions in each `.csproj`, hashed in `nix/deps.json` | editing the version, then regenerating `nix/deps.json` |
| CI actions | commit SHAs in `.github/workflows` | replacing the SHA and the version comment beside it |

The Nix inputs name revisions rather than branches, which leaves `nix flake update` with nothing to do.
A branch ref moves the whole package set at once.
Every row of `backend/internal/capabilities` is measured against ffmpeg and GStreamer, so that move is a change to the app's declared capabilities with no commit behind it.

The revision tracks nixos-unstable rather than a release channel: a release channel trails GStreamer by a minor series, and the plugin set is the thing under test.

Moving it is followed by re-measuring rather than by assuming.
A package set that moved can change which codecs probe usable, which elements a receive pipeline autoplugs, and which pixel formats an element accepts.
"Verifying a build" answers the first two.

## How the app locates the programs it spawns

`FindExe` in the `ffmpeg` package resolves the name (`ffmpeg`, `ffplay` or `gst-launch-1.0`, `.exe` on Windows) in this order:

1. A copy next to the app binary.
2. The first match on `PATH`.
3. Otherwise an error naming the missing program and both places it looked.

Every GStreamer child goes through the same lookup via `publish.FindGstExe`: the publish engine, the encode probe and the test streams.
A bare name handed to `exec.Command` would search `PATH` alone and pass over the copy a bundle ships.
A bundled launcher is also given `GST_PLUGIN_PATH` at spawn (`publish.GstChildEnv`), the prefix it was built against existing on no machine but the build host.

Two provisioning models follow: bundle ffmpeg next to the binary, or declare a dependency and rely on `PATH`.
The bundled copy always wins, so a packaged system ffmpeg and a bundled one never conflict.

The convention: bundle where the platform has no package manager to lean on (Windows, AppImage, Flatpak), declare a dependency where it does (Arch, NixOS, Debian, Fedora).
Bundling a private ffmpeg into a distro package is the opposite of what packagers expect and a common reason for rejection.

Also a licensing choice.
A declared dependency is installed by the package manager under its own terms and the package ships none of it.
A bundled copy is redistribution: ffmpeg is GPL and the GStreamer runtime LGPL, so an archive carrying them carries their notices and a source pointer.
`THIRD-PARTY-NOTICES.md` is that record, per artifact, copied in beside the binaries by the packaging scripts.
The app's own code stays Apache-2.0 either way, a spawned program and a loaded library both being boundaries the copyleft stops at.

## Screen-capture privileges on Linux

The capture backend decides whether elevated privileges are required.

| Backend | Session | Privilege |
|---------|---------|-----------|
| `x11grab` | X11, or XWayland windows only | none |
| `kmsgrab` | Wayland or a bare KMS console | `CAP_SYS_ADMIN` |
| `ddagrab`, `gdigrab` | Windows | none |

`x11grab` captures the whole X screen with no special rights and is the simplest Linux path.
On Wayland it sees XWayland client windows alone, so it is not a general capture backend there.

`kmsgrab` reads framebuffers straight from DRM and requires `CAP_SYS_ADMIN`, that API having no unprivileged variant.
It is the only backend in a stock ffmpeg build that captures a full Wayland desktop.

### Granting the capability

`CAP_SYS_ADMIN` is close to root: a process holding it can perform a wide range of kernel operations.
Never on a shared or system-wide ffmpeg: any local user who can run that binary inherits the capability through ffmpeg's many input handlers.
It goes on the app's own bundled copy, or on a dedicated wrapper, never on `/usr/bin/ffmpeg`.

```bash
sudo setcap cap_sys_admin+ep /path/to/app/ffmpeg
```

On NixOS the store is read-only, so `setcap` cannot target the store path.
A `security.wrappers` entry produces a setcap wrapper under `/run/wrappers/bin`:

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

The standards-track way to capture a Wayland desktop is the `org.freedesktop.portal.ScreenCast` portal backed by PipeWire, the path OBS and browsers use.
It grants no static capability: the user picks the surface in a portal dialog and the compositor hands back a PipeWire stream.
ffmpeg consumes it through the `pipewiregrab` filter, present only in a build with `--enable-libpipewire`.
The stock `ffmpeg-full` in nixpkgs is not, so this path takes a custom ffmpeg build.

### Capture device selection

`captureArgs` passes a fixed DRM node to `kmsgrab`.
That node is not stable across machines: the boot `simple-framebuffer` can occupy `card0` while the real GPU lands on `card1`, and the numbering depends on driver load order.
Portable capture picks the active card node at run time, the DRM node whose driver is a real GPU rather than `simple-framebuffer`, instead of hard-coding one.

## Per-channel packaging

### Windows (self-contained)

`scripts/package-windows.ps1` assembles this channel over what `task build:windows` and `task bundle:windows` produce.
Windows has no dependency manager the installer can rely on, so the app ships ffmpeg: the build task copies `ffmpeg.exe` and `ffplay.exe` next to the backend binary, where `FindExe` finds them first.
A development run needs no bundle, `FindExe` falling back to `PATH`.
The bundle takes a third-party build (Gyan or BtbN) carrying `ddagrab`.
No privilege step: `ddagrab` and `gdigrab` capture without elevation.

The launcher is the exception: on Windows it comes from MSYS2, whose prefix nothing puts on a normal `PATH`, so `task dev` appends `mingw64/bin` for the run.
Appended rather than prepended, MSYS2 shipping an ffmpeg of its own that a prefix in front would move every capture and encode onto.

The backend links GStreamer through cgo and no cross toolchain builds that from Linux, so the Windows binary is built on Windows against MSYS2's toolchain.
Install MSYS2, then from its MINGW64 shell:

```bash
pacman -S mingw-w64-x86_64-{toolchain,pkgconf,gstreamer} \
          mingw-w64-x86_64-gst-{plugins-base,plugins-good,plugins-bad,plugins-ugly,plugins-rs,rtsp-server,libav}
```

Go is not in that list, and the omission is the point: it comes from a Windows install of Go, MSYS2 shipping a trimmed one that would need `GOROOT` named for it.
`.github/workflows` installs the same set.

Building needs no particular shell.
The build and bundle tasks reach the toolchain, `MINGW_PREFIX` and MSYS2's own `sh` by path, from `MSYS2_ROOT` in `Taskfile.yml`, so they behave the same from Git Bash, PowerShell or cmd.
Set `MSYS2_ROOT` for an install other than `C:/msys64`.
What that variable absorbs is the prefix on `PATH`, where the GStreamer the built binary loads sits, and the `CC` and `PKG_CONFIG` cgo compiles through.
Both are named by path rather than looked up: the prefix is appended, as in `task dev`, so a machine carrying another `gcc` and `pkg-config` ahead of it would decide the toolchain instead.
Strawberry Perl ships that pair, and its `pkg-config` resolves against a prefix holding no GStreamer, which is what a `Can't find gobject-2.0.pc` reports.

Reaching MSYS2 by path rather than through the shell is what makes Git for Windows' Git Bash safe to build from.
The trap otherwise: Git Bash is built on MSYS2, reports `MSYSTEM=MINGW64` and prints the same prompt, but its `/mingw64` is Git's own prefix and carries neither the toolchain nor GStreamer.
Its `ldd`, `cygpath` and `MINGW_PREFIX` are Git's too, which is why `bundle:windows` names MSYS2's `sh` instead of taking whichever is on `PATH`.

One Windows runtime quirk belongs to the running process rather than the build.
libsrt names its threads by raising the debugger's thread-naming exception, which the Go runtime has no owner for and ends the process over.
Undisarmed, every pipeline carrying an `srtsrc` or `srtsink` dies as it is built, reported as `Exception 0x406d1388 ... signal arrived during external code execution`.
`backend/internal/receive` disarms it in the backend, the process building those elements in-process.
A spawned `gst-launch-1.0` is a C program and never sees it, which is why the same pipeline plays there.
RTSP is unaffected, no libsrt being loaded.

A build reporting `build constraints exclude all Go files` for a go-gst package ran against a `go` that found no C compiler and disabled cgo, which excludes every file in a binding whose files are all cgo.
The extra tell is a `go: downloading go1.26.4` line, which a `go` newer than `backend/go.mod` would never print, betraying the Windows Go rather than MSYS2's.
The build task asks for cgo outright so this surfaces as the missing compiler instead, and `cmd //c "where go gcc"` shows which toolchain a native child of the current shell resolves.

A receive pipeline ends in `appsink`, which is core, so the tile path needs no plugin of its own.
What it needs is the source element of the leg it watches.

| Element | Package | Used by |
|---|---|---|
| `whepsrc` | `gst-plugins-rs` | the WHEP watch leg |
| `srtsrc`, `srtsink`, RTMP | `gst-plugins-bad` | the SRT and RTMP legs |
| `rtspsrc` | `gst-plugins-good` | a receive pipeline and the test streams |
| `rtspclientsink` | `gst-rtsp-server` | every RTSP publish and every test stream |
| `x264enc` | `gst-plugins-ugly` | the GStreamer publish engine and the encode-rate probe |
| `glupload`, `glcolorconvert`, `glcolorscale` | `gst-plugins-base` | the `gl` render chain |

`gst-rtsp-server` is pulled in by none of the `gst-plugins-*` packages, and its absence shows up as `no element "rtspclientsink"` the first time a test stream or an RTSP publish starts, long after the build succeeded.
A machine registering none of the `gl` elements falls back to the CPU chain rather than failing (`viewer-architecture.md`).

A machine that runs the app has no MSYS2, so `scripts/bundle-windows.sh` copies the runtime beside the binaries: the DLL closure of the backend, of `gst-launch-1.0.exe` and of every installed plugin, flat next to the executables where the Windows loader looks first; `gst-launch-1.0.exe` itself; and the plugins under `gstreamer-1.0`.
No GLib schema, icon theme or font is bundled, the shell shipping its own (`avalonia/README.md`).

GStreamer looks for plugins in the prefix it was built against, so the backend prepends its own `gstreamer-1.0` directory to `GST_PLUGIN_PATH` before initializing the library, and passes the same to every child (`publish.GstChildEnv`).
A directory that is not there is an ordinary installation, left to find its plugins itself.

### Arch Linux (AUR)

`packaging/arch/PKGBUILD`.
Declare ffmpeg as a runtime dependency and let pacman provide it.
Do not bundle.

```bash
depends=('ffmpeg')
```

`ffmpeg` pulls in `ffplay`.
The `kmsgrab` capability is not something a package should grant silently.
Note it in the description and leave the `setcap` step, or the portal, to the user.

### NixOS / nixpkgs

`nix/package.nix`, exported from the flake as `packages.default`.
The derivation wraps the app binary so ffmpeg and ffplay are on its `PATH`:

```nix
nativeBuildInputs = [ makeWrapper ];

postInstall = ''
  wrapProgram $out/bin/screen-sharing \
    --prefix PATH : ${lib.makeBinPath [ ffmpeg-full ]}
'';
```

The runtime dependency is expressed in the closure, not bundled by hand, which is what nixpkgs expects.
The full derivation builds the Go backend with `buildGoModule` and the shell with the .NET SDK, matching `Taskfile.yml`.
The GStreamer libraries are ordinary build inputs and the plugin path is set by the wrapper the way ffmpeg's is.
The `kmsgrab` capability is a system concern, handled by the `security.wrappers` entry under "Granting the capability".

### Debian, Fedora, and other package managers

Same model as Arch: declare the dependency, do not bundle.

- Debian: `Depends: ffmpeg`, in a `.deb` no recipe here builds. A Debian install takes the tarball below instead, which is why `docs/install.md` names the apt packages it has to be given.
- Fedora: `Requires: ffmpeg`. `packaging/fedora/screen-sharing.spec` requires the two paths rather than the name, `ffmpeg-free` and RPM Fusion's `ffmpeg` both providing them.

Distributions with no package here take the tarball `scripts/package-linux.sh` builds, which carries both binaries and the .NET runtime and takes ffmpeg and GStreamer from the distribution.

### AppImage and Flatpak

Neither format has a recipe here, so what follows is the model one would take.
Both are self-contained, so ffmpeg goes inside the image next to the binary: the bundling side of the convention, with the obligations the Windows archive carries (`THIRD-PARTY-NOTICES.md`).
A Flatpak additionally captures through the portal, so it needs no capability and runs in the sandbox unmodified.
The manifest requests the `ScreenCast` portal rather than raw DRM access.

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

A successful `kmsgrab` frame confirms both the device node and the capability.
`No handle set on framebuffer` means the capability is missing.
