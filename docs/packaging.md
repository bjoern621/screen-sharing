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
| `vainfo` | for driver-scoped codec gaps | Names the VA driver an encode runs through, which the codec table's `DriverDefect` rows match on. | `libva-utils`. |

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
No window-system probe takes part, so a package declares no dependency for one.

### The AMD AMF runtime

The `*_amf` encoders are the one family needing more than ffmpeg itself.
Compiled in unconditionally, they load AMD's closed-source `libamfrt64.so.1` at run time by soname, so it has to be on the loader path rather than merely installed.
AMD ships it in the proprietary part of its driver package (`amf-amdgpu-pro`), x86_64 only.
AMF reaches the encoder hardware through Vulkan, so the card also needs a Vulkan driver.
A missing runtime is not a failure the app handles: the encoder refuses to open, the probe sees that, and the form greys the family exactly as on a machine with no AMD card.

`LD_LIBRARY_PATH` reaches an unprivileged `ffmpeg` alone.
The kmsgrab wrapper carries file capabilities, which puts it in glibc's secure-execution mode where that variable is ignored, so a runtime delivered through the environment never reaches its loader.
Untreated that is a wrong answer rather than a missing encoder: the probe runs the unprivileged binary, finds the runtime, and the form offers a family that dies at launch under kmsgrab.
The `kmsgrab.amf` option of `nix/mirrorme.nix` records the runtime on `libavutil`'s `RUNPATH` instead, which the loader does honour.
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
Every row of the capability table is measured against ffmpeg and GStreamer, so that move is a change to the app's declared capabilities with no commit behind it.

The revision tracks nixos-unstable rather than a release channel: a release channel trails GStreamer by a minor series, and the plugin set is the thing under test.

Moving it is followed by re-measuring.
A package set that moved can change which codecs probe usable, which elements a receive pipeline autoplugs, and which pixel formats an element accepts.

### The build stamp

No file in the tree holds the app's own version.
A release is published under the tag `vX.Y.Z`, and that tag is the number:
a copy in the tree is a second answer, free to disagree with the tag the artifact ships under.
`.github/workflows/version.yml` reads it off the tag once per run and hands every job the result as `VERSION`.
A run behind no release computes `0.0.0.dev.<commit>` instead, which the release check reads as a version it cannot compare.

Each channel takes the number from there and passes it to the backend link as `-ldflags "-X main.version=..."`.
The two packaging scripts and the Task pipeline read the environment variable, the PKGBUILD reads that same variable under `makepkg`, and `rpmbuild` takes it as `--define "appversion ..."`.
The handshake answers with that string and the window shows it (`backend/cmd/backend/main.go`).
A recipe skipping the flag ships a binary calling itself `dev`, which the release check reads the same way (`backend/internal/release`).
The dev tasks leave it at that default.

The flake reads the same variable, under `--impure`, and stamps the revision it was built from where nothing sets it:
`builtins.getEnv` answers empty under pure evaluation, which every build outside the release pipeline is (`flake.nix`).

## How the app locates the programs it spawns

`FindExe` in the `ffmpeg` package resolves the name (`ffmpeg`, `ffplay` or `gst-launch-1.0`, `.exe` on Windows) in this order:

1. A copy next to the app binary.
2. The first match on `PATH`.
3. Otherwise an error naming the missing program and both places it looked.

Every GStreamer child goes through the same lookup: the publish engine, the encode probe and the test streams.
A bare name handed to the process launcher would search `PATH` alone and pass over the copy a bundle ships.
A bundled launcher is also given `GST_PLUGIN_PATH` at spawn, the prefix it was built against existing on no machine but the build host.

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
It goes on the app's own bundled copy, or on a dedicated wrapper.

```bash
sudo setcap cap_sys_admin+ep /path/to/app/ffmpeg
```

On NixOS the store is read-only, so `setcap` cannot target the store path.
`nixosModules.mirrorme` writes the `security.wrappers` entry that produces one under `/run/wrappers/bin`:

```nix
programs.mirrorme.kmsgrab.enable = true;
users.users.<name>.extraGroups = [ "mirrorme" ];
```

That adds `ffmpeg-kmsgrab`, mode 0510 `root:mirrorme`, and exports `MIRRORME_FFMPEG_KMSGRAB` for the app to run.
The capability is the module's whole contents: the app and ffmpeg install without root, so they come from wherever the host declares its packages.

The group is the gate, so the capability reaches whoever joins it and no other local user.
A dedicated group rather than `video`, which carries the machine's GPU users and is wider than the set trusted with `CAP_SYS_ADMIN`.
The wrapper takes a name of its own rather than shadowing `ffmpeg` on `PATH`, which would hand the capability to every capture path that needs none.

### The portal alternative

The standards-track way to capture a Wayland desktop is the `org.freedesktop.portal.ScreenCast` portal backed by PipeWire, the path OBS and browsers use.
It grants no static capability: the user picks the surface in a portal dialog and the compositor hands back a PipeWire stream.
ffmpeg consumes it through the `pipewiregrab` filter, present only in a build with `--enable-libpipewire`.
The stock `ffmpeg-full` in nixpkgs is not, so this path takes a custom ffmpeg build.

### Capture device selection

The DRM node `kmsgrab` reads is resolved per run: the first `/dev/dri/card*` whose driver is a real GPU rather than `simple-framebuffer` (`drmCaptureDevice`).
A fixed node travels badly, the boot framebuffer often holding `card0` while the real GPU lands on `card1`, and the numbering following driver load order.

## Per-channel packaging

One section per channel, naming its recipe and which of the two provisioning models it takes.

### Windows (self-contained)

Two downloads over one staging step: `scripts/package-windows.ps1` writes the zip and leaves the staged directory, and `scripts/installer-windows.ps1` compiles `packaging/windows/mirrorme.iss` over that same directory.
So the installer and the zip carry one set of files rather than two assemblies free to disagree.
The install is per user, under `%LocalAppData%\Programs\MirrorMe`, which is what keeps a UAC prompt off an unsigned binary.
Inno Setup's compiler is the one build dependency neither the runner image nor a Windows checkout carries; `choco install innosetup` is how the release workflow gets it.

`scripts/package-windows.ps1` assembles this channel over what `task build:windows` and `task bundle:windows` produce.
Windows has no dependency manager the installer can rely on, so the app ships ffmpeg.
The build task copies `ffmpeg.exe` and `ffplay.exe` next to the backend binary, where `FindExe` finds them first.
A development run needs no bundle, `FindExe` falling back to `PATH`.
The bundle takes a third-party build (Gyan or BtbN) carrying `ddagrab`.
No privilege step: `ddagrab` and `gdigrab` capture without elevation.

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

A machine that runs the app has no MSYS2, so `scripts/bundle-windows.sh` copies the runtime beside the binaries:

- the DLL closure of the backend, of `gst-launch-1.0.exe` and of every installed plugin, flat next to the executables where the Windows loader looks first,
- `gst-launch-1.0.exe` itself,
- the plugins under `gstreamer-1.0`.

No GLib schema, icon theme or font is bundled, the shell shipping its own (`avalonia/README.md`).

GStreamer looks for plugins in the prefix it was built against, so the backend prepends its own `gstreamer-1.0` directory to `GST_PLUGIN_PATH` before initializing the library, and passes the same to every child.
A directory that is not there is an ordinary installation, left to find its plugins itself.

libsrt names its threads by raising the debugger's thread-naming exception, which the Go runtime has no owner for and ends the process over.
Undisarmed, every pipeline carrying an `srtsrc` or `srtsink` dies as it is built, reported as `Exception 0x406d1388 ... signal arrived during external code execution`.
The backend disarms it, being the process that builds those elements in-process.
A spawned `gst-launch-1.0` is a C program that never sees it, so the same pipeline plays there.
RTSP is unaffected, no libsrt being loaded.

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
  wrapProgram $out/bin/mirrorme \
    --prefix PATH : ${lib.makeBinPath [ ffmpeg-full ]}
'';
```

The runtime dependency is expressed in the closure, which is what nixpkgs expects.
The full derivation builds the Go backend with `buildGoModule` and the shell with the .NET SDK, matching `Taskfile.yml`.
The GStreamer libraries are ordinary build inputs and the plugin path is set by the wrapper the way ffmpeg's is.
The `kmsgrab` capability is a system concern, handled by the `security.wrappers` entry under "Granting the capability".

### Debian, Fedora, and other package managers

Same model as Arch: declare the dependency, do not bundle.

- Debian: `Depends: ffmpeg`, in a `.deb` no recipe here builds. A Debian install takes the tarball below instead, which is why `docs/install.md` names the apt packages it has to be given.
- Fedora: `Requires: ffmpeg`. `packaging/fedora/mirrorme.spec` requires the two paths rather than the name, `ffmpeg-free` and RPM Fusion's `ffmpeg` both providing them.

Distributions with no package here take the tarball `scripts/package-linux.sh` builds, which carries both binaries and the .NET runtime and takes ffmpeg and GStreamer from the distribution.

### Flatpak

`packaging/flatpak/de.bjoernblessin.MirrorMe.yml`, built by `task package:flatpak`.
Self-contained, so it is the bundling side of the convention, with the obligations the Windows archive carries (`THIRD-PARTY-NOTICES.md`).

The channel exists for the distributions the tarball leaves out.
That artifact takes GStreamer from the machine and the backend links symbols 1.26 introduced, which is newer than the current Debian and Ubuntu long-term releases carry.

Three sources make up what is inside it.
`org.freedesktop.Platform` 25.08 carries GStreamer and the base plugin set.
The `ffmpeg-full` extension carries the codecs the runtime leaves out, libx264 and libx265 among them, and GStreamer's `x264enc` with them.
The manifest itself compiles ffmpeg and ffplay, which no extension ships: the extension's own ffmpeg is configured `--disable-programs` and is libraries alone.

What the app is comes in already built.
The shell is a self-contained .NET publish whose restore reaches NuGet, and a `flatpak-builder` module builds with no network, so `scripts/package-linux.sh` runs first and the manifest assembles what it staged.

Capture goes through the portal, so the sandbox needs no capability.
`kmsgrab` is unreachable from inside it and wants `CAP_SYS_ADMIN`, which is the whole reason the portal path exists.

### AppImage

No recipe here.
The model is the Flatpak's without the runtime: ffmpeg goes inside the image next to the binary, and the same obligations follow it.

### Container images

The relay, the proxy and the group service, built by `nix build .#relay-image` and its two siblings under `nix/`.
Each writes a `docker-archive` tarball, which is the format `docker load` reads.

`.github/workflows/images.yml` publishes them to `ghcr.io/bjoern621/screenshare-relay`, `-proxy` and `-groupd`.
The tag is the release's version, and only a published release pushes one, so a tag names a release and never moves.
A cluster pins an exact tag against that.
The release page carries the channels a person installs, so these are reached through the registry.

## Building on Windows

The backend links GStreamer through cgo and no cross toolchain builds that from Linux, so the Windows binary is built on Windows against MSYS2's toolchain.
Install MSYS2, then from its MINGW64 shell:

```bash
pacman -S mingw-w64-x86_64-{toolchain,pkgconf,gstreamer} \
          mingw-w64-x86_64-gst-{plugins-base,plugins-good,plugins-bad,plugins-ugly,plugins-rs,rtsp-server,libav}
```

Go is not in that list: it comes from a Windows install of Go, MSYS2 shipping a trimmed one that would need `GOROOT` named for it.
`.github/workflows` installs the same set.

A development run takes the launcher from MSYS2, whose prefix nothing puts on a normal `PATH`, so `task dev` appends `mingw64/bin` for the run.
Appended rather than prepended, MSYS2 shipping an ffmpeg of its own that a prefix in front would move every capture and encode onto.

Building needs no particular shell.
The build and bundle tasks reach the toolchain, `MINGW_PREFIX` and MSYS2's own `sh` by path, from `MSYS2_ROOT` in `Taskfile.yml`, so they behave the same from Git Bash, PowerShell or cmd.
Set `MSYS2_ROOT` for an install other than `C:/msys64`.
What that variable absorbs is the prefix on `PATH`, where the GStreamer the built binary loads sits, and the `CC` and `PKG_CONFIG` cgo compiles through.
Both are named by path rather than looked up: the prefix is appended, as in `task dev`, so a machine carrying another `gcc` and `pkg-config` ahead of it would decide the toolchain instead.
Strawberry Perl ships that pair, and its `pkg-config` resolves against a prefix holding no GStreamer, which is what a `Can't find gobject-2.0.pc` reports.

Reaching MSYS2 by path rather than through the shell is what makes Git for Windows' Git Bash safe to build from.
The trap otherwise: Git Bash is built on MSYS2, reports `MSYSTEM=MINGW64` and prints the same prompt, but its `/mingw64` is Git's own prefix and carries neither the toolchain nor GStreamer.
Its `ldd`, `cygpath` and `MINGW_PREFIX` are Git's too, so `bundle:windows` names MSYS2's `sh` instead of taking whichever is on `PATH`.

A build reporting `build constraints exclude all Go files` for a go-gst package ran against a `go` that found no C compiler and disabled cgo, which excludes every file in a binding whose files are all cgo.
The extra tell is a `go: downloading go1.26.4` line, which a `go` newer than `backend/go.mod` would never print, betraying the Windows Go rather than MSYS2's.
The build task asks for cgo outright so this surfaces as the missing compiler instead, and `cmd //c "where go gcc"` shows which toolchain a native child of the current shell resolves.
