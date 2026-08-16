# screen-sharing

Self-hosted, high-quality group screen sharing for trusted friends.

Relay = **MediaMTX**.
Transports = **SRT**, **RTSP**, **RTMP**, **WebRTC** (WHIP in, WHEP out), **HLS** (watch only).
Capture, encode and decode = **ffmpeg** and **GStreamer**.

Every stream crosses two legs, publisher to relay and relay to viewer, and each leg picks its own protocol.
The relay re-serves what it ingests on all its listeners, so a stream published over SRT can be watched over RTSP.

No accounts. No remote control. Everyone publishes and watches at once.
Full colour, 4:4:4 and full range, so no WebRTC washout.

A group is one key friends share, and a member is in it for as long as their own app says so.
[`docs/membership.md`](docs/membership.md) states what that key buys and what a lapse closes.

## Install

Packages for Windows, Arch, Fedora, NixOS and other Linux distributions are on the
[releases page](https://github.com/bjoern621/screen-sharing/releases).
[`docs/install.md`](docs/install.md) has the steps for each.

One window, two programs behind it: a headless backend and the shell in front of it.
Opening the shell starts the backend. Nothing else to launch.

## The relay

Streams do not travel between machines directly.
Every publisher sends to one relay and every viewer reads from it, so one machine everybody can reach runs MediaMTX and the group service that signs the tokens it checks: a VPS, a box on the LAN, a host on a Tailscale network.

```bash
task relay
```

Both of them, on this machine, from `deploy/mediamtx-groups.yml`: the configuration a deployment runs.
A self-signed certificate is drawn where none is there, so the encrypted listeners come up on a machine that has no certificate of its own.
Serves SRT (8890/udp), RTSPS (8322), RTMPS (1936), HLS, WebRTC and MoQ (8892 tcp+udp), plus the API on 9997.
The binaries come from the flake's dev shell on Linux and macOS.
Windows has no such shell and runs `pwsh scripts/relay.ps1`, which fetches MediaMTX on first run.

## Topology

```
You              ──SRT──┐
Friend A         ──SRT──┼──► MediaMTX relay ──SRT──► anyone
Friend B         ──SRT──┘                            HLS/WebRTC/MoQ for browser
```

[`docs/network-architecture.md`](docs/network-architecture.md) states why the relay is there at all, and which legs cross the internet.

## Bandwidth reality

- **Upload** = sum of your publish bitrates.
- **Download** = sum of the bitrates you watch.
- Relay egress = publishers x viewers x bitrate, which scales fast.

Start modest, 40 to 80 Mbps HEVC 4:4:4 already beating Discord, and crank it when few are watching.

## Repository layout

| Path | Holds |
| --- | --- |
| `api/` | the control contract, a module of its own so neither side depends on the other |
| `backend/` | the Go module: the headless backend and the group service |
| `avalonia/` | the shell in front of the backend |
| `build/` | icons, the Windows redistributables, and where a build lands |
| `bruno/` | the deployment's HTTP APIs as a request collection |
| `deploy/` | the relay and reverse-proxy config a deployment on the internet runs |
| `docs/` | architecture, domain model and terminology |
| `nix/` | the Nix packages and the NixOS module for the privileged capture path |
| `packaging/` | the Arch and Fedora recipes and the desktop entry |
| `scripts/` | packaging and development helpers |

`api/`, `backend/` and `avalonia/` each carry a README stating what belongs in it.

## Building it

`Taskfile.yml` carries the development and packaging tasks.
`task` lists them.
`docs/packaging.md` states what the app needs at run time and how each channel provides it.
Recipes are in `packaging/` and `nix/`.

`scripts/` also holds the PowerShell tools the app replaced, which publish, watch and list live streams from a terminal: `publish.ps1`, `watch.ps1`, `whoislive.ps1`.

## License

Apache-2.0 ([`LICENSE`](LICENSE)).
The Windows archive additionally carries ffmpeg and the GStreamer runtime, which stay under their own GPL and LGPL terms.
[`THIRD-PARTY-NOTICES.md`](THIRD-PARTY-NOTICES.md) states what every artifact ships and where its source is.
