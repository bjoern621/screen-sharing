# Installing

The downloads are on the [releases page](https://github.com/bjoern621/screen-sharing/releases),
one file per platform. Nix builds from the flake and downloads nothing.

The app is one window with two programs behind it: a headless backend that captures,
encodes, publishes and decodes, and the shell in front of it.
Opening the shell starts the backend, so there is one thing to launch and nothing to
start in a particular order.

Streams do not travel between machines directly.
Every publisher sends to one relay and every viewer reads from it, so somebody has to be
running a relay before anything is watchable.
"The relay" below covers that.

## Windows

1. Download `screen-sharing-<version>-windows-x64.zip`.
2. Extract it anywhere.
3. Run `screenshare-avalonia.exe`.

Nothing else has to be installed: ffmpeg and GStreamer are inside the archive.

The binaries are unsigned, so the first run raises SmartScreen's "Windows protected your
PC". "More info", then "Run anyway".

## Arch Linux

```sh
sudo pacman -U screen-sharing-<version>-1-x86_64.pkg.tar.zst
```

pacman pulls ffmpeg and the GStreamer plugins from the repositories.

Building it instead needs the recipe in `packaging/arch` and nothing else from this
repository:

```sh
makepkg -si
```

## Fedora

```sh
sudo dnf install ./screen-sharing-*.rpm
```

Two encoders are missing afterwards, and both are Fedora's packaging rather than the
app's:

- `x264enc`, the software H.264 encoder the GStreamer publish engine uses, lives in
  [RPM Fusion](https://rpmfusion.org/Configuration): `sudo dnf install gstreamer1-plugins-ugly`.
- The WebRTC transports (WHIP to publish, WHEP to watch) need `gst-plugins-rs`, which
  Fedora does not package. SRT, RTSP, RTMP and HLS are unaffected.

## NixOS and Nix

Run it without installing anything:

```sh
nix run github:bjoern621/screen-sharing
```

Install it from a flake:

```nix
inputs.screen-sharing.url = "github:bjoern621/screen-sharing";

environment.systemPackages = [ inputs.screen-sharing.packages.${system}.default ];
```

The flake turns on GStreamer's Vulkan and QSV plugins, which no binary cache carries, so
the first build compiles `gst-plugins-bad` from source.

Capturing a Wayland desktop with `kmsgrab` needs a privileged ffmpeg, which
`nixosModules.screenShare` sets up. The portal path below needs nothing.

## Debian, Ubuntu and other distributions

The tarball carries both binaries and its own .NET runtime, and takes ffmpeg and
GStreamer from the distribution:

```sh
sudo apt install ffmpeg gstreamer1.0-tools \
  gstreamer1.0-plugins-base gstreamer1.0-plugins-good \
  gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly \
  gstreamer1.0-libav gstreamer1.0-rtsp gstreamer1.0-pipewire libnice10

tar xf screen-sharing-<version>-linux-x64.tar.gz
./screen-sharing-<version>-linux-x64/screenshare-avalonia
```

Debian and Ubuntu package no `gst-plugins-rs`, so the WebRTC transports are absent there
for the same reason they are on Fedora.

## The relay

One machine runs [MediaMTX](https://github.com/bluenviron/mediamtx) and everybody points
at it.
It can be a VPS, a machine on the LAN, or a host on a Tailscale network; it needs to be
reachable by every publisher and every viewer.
The app ships pointing at `streamrelay.bjoernblessin.de`, and the Setup screen's Relay
field changes that.

Running one, from a checkout of this repository:

```sh
docker compose up -d
```

`mediamtx.yml` is the configuration that starts, and the ports it opens are SRT
8890/udp, RTSP 8554, RTMP 1935, HLS 8888, WebRTC 8889 and the API on 9997.
Docker's UDP proxy rewrites the source port and breaks SRT's handshake on Windows, so a
Windows host runs the relay natively instead: `pwsh scripts/relay.ps1`.

## Capturing a Wayland desktop

`x11grab`, the default capture backend on Linux, sees only XWayland windows in a Wayland
session, not the desktop.
Two backends capture it properly:

- The desktop portal, through PipeWire, which asks for the surface in a dialog and needs
  no privilege. It runs on the GStreamer publish engine.
- `kmsgrab`, which reads the scanout framebuffer straight from DRM and needs
  `CAP_SYS_ADMIN`. That capability is close to root and belongs on a dedicated ffmpeg
  copy rather than on `/usr/bin/ffmpeg`, which is what the NixOS module builds and what
  `docs/packaging.md` describes for everyone else.

## Where settings are kept

`~/.config/screenshare/settings.json` on Linux, `%AppData%\screenshare\settings.json` on
Windows.
Deleting the file resets every setting to its default.
