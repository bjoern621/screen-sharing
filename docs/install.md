# Installing

The downloads are on the [releases page](https://github.com/bjoern621/screen-sharing/releases), one file per platform.
Nix builds from the flake and downloads nothing.

The app is one window with two programs behind it: a headless backend that captures, encodes, publishes and decodes, and the shell in front of it.
Opening the shell starts the backend, so there is one thing to launch.

Streams do not travel between machines directly.
Every publisher sends to one relay and every viewer reads from it, so somebody has to be running a relay before anything is watchable ("The relay").

## Windows

1. Download `mirrorme-<version>-windows-x86_64-setup.exe`.
2. Run it.

It installs for the current user under `%LocalAppData%\Programs\MirrorMe`, so it asks for no administrator rights, and it writes a Start-menu entry and an uninstaller.

`mirrorme-<version>-windows-x86_64.zip` is the same files with nothing registered: extract it anywhere and run `mirrorme.exe`.

Nothing else has to be installed either way: ffmpeg and GStreamer are inside both downloads.

The binaries are unsigned, so the first run raises SmartScreen's "Windows protected your PC".
"More info", then "Run anyway".

## Arch Linux

```sh
sudo pacman -U mirrorme-<version>-1-x86_64.pkg.tar.zst
```

pacman pulls ffmpeg and the GStreamer plugins from the repositories.

Building it instead needs the recipe in `packaging/arch` and nothing else from this repository:

```sh
makepkg -si
```

## Fedora

```sh
sudo dnf install ./mirrorme-*.rpm
```

What is missing afterwards is Fedora's packaging rather than the app's:

- `x264enc`, the software H.264 encoder the GStreamer publish engine uses, lives in [RPM Fusion](https://rpmfusion.org/Configuration): `sudo dnf install gstreamer1-plugins-ugly`.
- The WebRTC transports (WHIP to publish, WHEP to watch) need `gst-plugins-rs`, which Fedora does not package.
  SRT, RTSP, RTMP and HLS are unaffected.

## NixOS and Nix

Run it without installing anything:

```sh
nix run github:bjoern621/screen-sharing
```

Install it from a flake:

```nix
inputs.mirrorme.url = "github:bjoern621/screen-sharing";

environment.systemPackages = [ inputs.mirrorme.packages.${system}.default ];
```

The flake turns on GStreamer's Vulkan and QSV plugins, which no binary cache carries, so the first build compiles `gst-plugins-bad` from source.

Capturing a Wayland desktop with `kmsgrab` needs a privileged ffmpeg, which `nixosModules.mirrorme` sets up.
The portal path needs nothing.

## Flatpak

The bundle carries its own ffmpeg and GStreamer, so it installs on a distribution that packages neither.

```sh
flatpak install --user ./mirrorme-<version>-x86_64.flatpak
flatpak run de.bjoernblessin.MirrorMe
```

Capture goes through the desktop portal, which asks for the surface in a dialog.
The sandbox holds no privilege, so `kmsgrab` is unreachable there and nothing has to be granted.

Two things are outside the bundle.
The WebRTC transports need `gst-plugins-rs`, which the runtime does not carry, so WHIP and WHEP are absent and SRT, RTSP, RTMP and HLS are unaffected.
The ffmpeg engine's software H.264 and HEVC rows probe as unavailable and the form greys them, the GStreamer engine encoding H.264 in software instead.

## Debian, Ubuntu and other distributions

The Flatpak above is the one to take on a distribution whose GStreamer is older than 1.26, which both current Debian and Ubuntu long-term releases are.

The tarball carries both binaries and its own .NET runtime, and takes ffmpeg and GStreamer from the distribution.
GStreamer has to be 1.26 or newer, the backend linking symbols that release introduced:

```sh
sudo apt install ffmpeg gstreamer1.0-tools \
  gstreamer1.0-plugins-base gstreamer1.0-plugins-good \
  gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly \
  gstreamer1.0-libav gstreamer1.0-rtsp gstreamer1.0-pipewire libnice10

tar xf mirrorme-<version>-linux-x86_64-portable.tar.gz
./mirrorme-<version>-linux-x86_64-portable/mirrorme
```

Debian and Ubuntu package no `gst-plugins-rs` either, so the WebRTC transports are absent there too.

## The relay

One machine runs [MediaMTX](https://github.com/bluenviron/mediamtx) and everybody points at it.
It can be a VPS, a machine on the LAN, or a host on a Tailscale network.
It needs to be reachable by every publisher and every viewer.
The app ships pointing at `streamrelay.bjoernblessin.de`, and the Setup screen's Relay field changes that.

Running one, from a checkout of this repository:

```sh
task relay
```

`deploy/mediamtx-groups.yml` is the configuration that starts, with the group service beside it.
The relay checks a token on every connection against the key set that service publishes, so a relay without it serves nobody.
A self-signed certificate is drawn into `dev-relay/` where none is there.
Its path and the read hook's are handed to MediaMTX as environment overrides, so the file itself is the one a deployment reads.

It opens:

| Leg | Port |
| --- | --- |
| SRT | 8890/udp |
| RTSPS | 8322 |
| RTMPS | 1936 |
| HLS | 8888 |
| WebRTC | 8889 |
| MoQ | 8892 on both TCP and UDP, native QUIC beside it on 8893/udp |
| API | 9997 |

HLS, WebRTC and the API answer on loopback, a deployment reaching them through the reverse proxy in `deploy/Caddyfile` under one name on 443.
MoQ is the exception: no proxy carries WebTransport, so the relay answers that port directly and a watcher's network has to pass both sides of it.
`docs/network-architecture.md` covers which leg is encrypted with what, and `backend check-relay` says which of them a given machine is answering on.

The binaries come from the flake's dev shell on Linux and macOS.
Windows has no such shell, so a Windows host runs `pwsh scripts/relay.ps1`, which fetches `mediamtx.exe` into `bin/` on first run and starts both against the same configuration.

## Capturing a Wayland desktop

`x11grab`, the default capture backend on Linux, sees only XWayland windows in a Wayland session.
Two backends capture the desktop properly:

- The desktop portal, through PipeWire, which asks for the surface in a dialog and needs no privilege.
  It runs on the GStreamer publish engine.
- `kmsgrab`, which reads the scanout framebuffer straight from DRM and needs `CAP_SYS_ADMIN`.
  That capability is close to root and belongs on a dedicated ffmpeg copy rather than on `/usr/bin/ffmpeg`.
  The NixOS module builds that copy, and `docs/packaging.md` describes it for everyone else.

## Where settings are kept

`~/.config/mirrorme/settings.json` on Linux, `%AppData%\mirrorme\settings.json` on Windows.
Deleting the file resets every setting to its default.
