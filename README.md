# MirrorMe

[![Release](https://img.shields.io/github/v/release/bjoern621/screen-sharing)](https://github.com/bjoern621/screen-sharing/releases)
[![License](https://img.shields.io/github/license/bjoern621/screen-sharing)](LICENSE)

Screen sharing for a group, at the quality the hardware can produce.
Everyone shares and everyone watches, at once.
Free and open source, with no accounts.

Built for the screens a video call blurs: code, terminals, spreadsheets, a game at 60 fps.
Text stays sharp because the picture is coded at 4:4:4 or straight RGB, full range, 8 or 10 bit.
Resolution, frame rate and bitrate are settings, and 40 to 80 Mbps HEVC is an ordinary choice.

## What it does

- Every member of a group publishes and watches. The Viewer tab draws each live stream as a tile, with the group's members beside it.
- 4:4:4, planar RGB and full range, so text carries no colour fringe and dark scenes stay dark.
- Hardware encoders: NVENC, Intel Quick Sync, AMD AMF, VAAPI, Vulkan Video, V4L2 and Rockchip MPP. Software: x264, x265, SVT-AV1, libaom, rav1e and libvpx.
- Frames reach the encoder without a trip through system memory where capture and encoder share a device.
- H.264, HEVC, AV1, VP9 and VP8. Setup offers what this machine's encoder and the chosen transport carry, and greys the rest with the reason.
- `lossless`, `gaming` and `readability` presets each name a picture, and the app finds the encoder, pixel format and capture backend that deliver it on this machine.
- Desktop audio as Opus or AAC. One application's audio alone, on Linux.
- Delay measured per stage, capture to a viewer's window, and shown in the app. The SRT latency window is a setting.
- Viewers watch in the app, in a browser over HLS, WebRTC or Media over QUIC, or in any player that speaks SRT or RTSP.
- A group is a 256-bit key. Holding it is membership. Paste it in to join, take it out to leave.

Windows and Linux.

## Install

Downloads are on the [releases page](https://github.com/bjoern621/screen-sharing/releases).
[`docs/install.md`](docs/install.md) has each platform in full.

| Platform | Get it |
| --- | --- |
| Windows | `mirrorme-<version>-windows-x86_64-setup.exe`, or the `.zip` to run from anywhere. ffmpeg and GStreamer are inside both. |
| Arch Linux | `sudo pacman -U mirrorme-<version>-1-x86_64.pkg.tar.zst` |
| Fedora | `sudo dnf install ./mirrorme-*.rpm` |
| NixOS, Nix | `nix run github:bjoern621/screen-sharing` |
| Flatpak | `flatpak install --user ./mirrorme-<version>-x86_64.flatpak`, the pick where the distribution's GStreamer is older than 1.26 |
| Other Linux | `mirrorme-<version>-linux-x86_64-portable.tar.gz`, with ffmpeg and GStreamer 1.26 or newer from the distribution |

One window.
Opening it starts the backend behind it.

## Share a screen

1. Open MirrorMe on the Setup tab.
2. Create a group, or paste a group key a member sent.
3. Pick a screen and a preset, or open the quality step and set codec, encoder, chroma and bitrate by hand.
4. Share.

Every stream in the group lands on the Viewer tab, one tile each.
A stream shared with no group key is public: anybody who reaches the relay can watch it, and the app says so before it starts.

## How it works

```
publisher ──SRT/RTSP/RTMP/WebRTC──► relay ──SRT/RTSP/WebRTC/HLS/MoQ──► viewers
```

Every publisher sends one copy to a [MediaMTX](https://github.com/bluenviron/mediamtx) relay and every viewer reads from it,
so an upload carries one stream however many watch, and no home connection needs an open port.
The relay re-serves what it ingests on every listener,
so a stream published over SRT is watched over WebRTC in a browser and over RTSP in a player, at once.
Capture, encode and decode run on ffmpeg and GStreamer.

A group is a path prefix derived from its key.
The group service trades the key for a short-lived token, the relay checks that token by signature alone, and membership is a lease each member's own app keeps alive.
The relay sees every stream it carries.

The app ships pointing at the project's relay.
A group that wants its streams on its own machine runs one, and [`docs/install.md`](docs/install.md), "The relay", has the steps.

`docs/` holds the rest:
[`network-architecture.md`](docs/network-architecture.md),
[`membership.md`](docs/membership.md),
[`domain-model.md`](docs/domain-model.md),
[`video-stack.md`](docs/video-stack.md).

## Developing

Go backend, Avalonia shell, a gRPC contract between them in [`api/`](api/).
`task` lists the development and packaging tasks, and `task all` runs relay, backend and shell together.
[`docs/development-principles.md`](docs/development-principles.md) governs every change.
`api/`, `backend/` and `avalonia/` each carry a README for their layout.

## License

Apache-2.0 ([`LICENSE`](LICENSE)).
The Windows downloads also carry ffmpeg and the GStreamer runtime, under their own GPL and LGPL terms.
[`THIRD-PARTY-NOTICES.md`](THIRD-PARTY-NOTICES.md) states what every artifact ships and where its source is.
