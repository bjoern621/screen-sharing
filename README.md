# screen-sharing

Self-hosted, high-quality group screen sharing for trusted friends.
Relay = **MediaMTX**. Transports = **SRT**, **RTSP**, **RTMP**, **WebRTC** (WHIP in, WHEP out) and **HLS** (watch only). Capture/encode/decode = **ffmpeg** and **GStreamer**.
Every stream crosses two legs, publisher to relay and relay to viewer, and each picks its own protocol: the relay re-serves what it ingests on all its listeners, so a stream published over SRT can be watched over RTSP.

No accounts. No remote control. Everyone publishes and watches at once.
Full color (4:4:4 + full range) - no WebRTC washout.

## Install

Packages for Windows, Arch, Fedora, NixOS and other Linux distributions are on the
[releases page](https://github.com/bjoern621/screen-sharing/releases), and
[`docs/install.md`](docs/install.md) has the steps for each.

The app is one window with two programs behind it, a headless backend and the shell in
front of it, and opening the shell starts the backend. There is nothing else to launch.

## The relay

Streams do not travel between machines directly.
Every publisher sends to one relay and every viewer reads from it, so one machine
everybody can reach runs MediaMTX: a VPS, a box on the LAN, or a host on a Tailscale
network.

```bash
docker compose up -d
```

That serves SRT (8890/udp), RTSP, RTMP, HLS, WebRTC, MoQ (8892 tcp+udp) and the API on
9997, all from `mediamtx.yml`.
Docker's UDP proxy rewrites the source port and breaks SRT's handshake on Windows, so a
Windows host runs the relay natively instead: `pwsh scripts/relay.ps1`.

## Topology

```
You              ──SRT──┐
Friend A         ──SRT──┼──► MediaMTX relay ──SRT──► anyone
Friend B         ──SRT──┘                            HLS/WebRTC for browser
```

[`docs/network-architecture.md`](docs/network-architecture.md) states why the relay is there
at all, which legs cross the internet and which stay on loopback.

## Bandwidth reality

- **Upload** = sum of your publish bitrates.
- **Download** = sum of bitrates you watch.
- Relay egress = publishers x viewers x bitrate - scales fast. Start modest
  (40–80 Mbps HEVC 4:4:4 already beats Discord), crank when few are watching.

## Repository layout

```
api/          the control contract, a module of its own so neither side depends on the other
backend/      the Go module: the headless backend and the group service
avalonia/     the shell in front of the backend
build/        icons, the Windows redistributables, and where a build lands
bruno/        the deployment's HTTP APIs as a request collection
deploy/       the relay and the group service as containers
docs/         architecture, domain model and terminology
nix/          the Nix packages and the NixOS module for the privileged capture path
packaging/    the Arch and Fedora recipes and the desktop entry
scripts/      packaging and development helpers
```

Each of `api/`, `backend/` and `avalonia/` carries a README stating what belongs in it.

## Building it

`Taskfile.yml` carries the development and packaging tasks; `task` lists them.
`docs/packaging.md` states what the app needs at run time and how each channel provides
it, and the recipes are in `packaging/` and `nix/`.

`scripts/` also holds the PowerShell tools the app replaced, which publish, watch and
list live streams from a terminal: `publish.ps1`, `watch.ps1`, `whoislive.ps1`.

## License

Apache-2.0 ([`LICENSE`](LICENSE)).
The Windows archive additionally carries ffmpeg and the GStreamer runtime, which stay
under their own GPL and LGPL terms;
[`THIRD-PARTY-NOTICES.md`](THIRD-PARTY-NOTICES.md) states what every artifact ships and
where its source is.
