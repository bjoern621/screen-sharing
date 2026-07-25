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
| `xdotool` | Linux X11/XWayland only | Detect when the viewer window has appeared. | Distro package `xdotool`. |

`ffplay` is part of a full ffmpeg build, so a single ffmpeg install satisfies
both.
Minimal ffmpeg builds sometimes omit `ffplay` and the capture demuxers; the
build has to include the demuxer for the target platform (`ddagrab` on Windows,
`x11grab` and `kmsgrab` on Linux).
`captureArgs` in the `ffmpeg` package is the source of truth for which backends
the app invokes.

`xdotool` is optional: without it the app still publishes and watches, it only
loses the "viewer ready" signal on X11.
The Nix dev shell in `flake.nix` lists the same set, and is the reference for a
known-good dependency set during development.

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
The build copies `ffmpeg.exe` and `ffplay.exe` into the Wails output directory
next to `screen-sharing.exe`, where `FindExe` finds them first.
Use a recent third-party build (Gyan or BtbN); `ddagrab` needs a current ffmpeg.
No privilege step: `ddagrab` and `gdigrab` capture without elevation.

### Arch Linux (AUR)

Declare ffmpeg as a runtime dependency and let pacman provide it.
Do not bundle.

```bash
depends=('ffmpeg')
optdepends=('xdotool: viewer-window detection on X11')
```

`ffmpeg` pulls in `ffplay`.
The `kmsgrab` capability is not something a package should grant silently; note
it in the package description and leave the `setcap` step, or the portal, to the
user.

### NixOS / nixpkgs

The derivation wraps the app binary so ffmpeg, ffplay and xdotool are on its
`PATH`, using `makeWrapper`:

```nix
nativeBuildInputs = [ makeWrapper ];

postInstall = ''
  wrapProgram $out/bin/screen-sharing \
    --prefix PATH : ${lib.makeBinPath [ ffmpeg-full xdotool ]}
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

- Debian: `Depends: ffmpeg`, `Recommends: xdotool`.
- Fedora: `Requires: ffmpeg`, `Recommends: xdotool`.

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
