# screen-sharing

Self-hosted, high-quality group screen sharing for trusted friends.
Relay = **MediaMTX**. Transports = **SRT**, **RTSP**, **WebRTC/WHIP** (publish only). Capture/encode/decode = **ffmpeg**.
Every stream crosses two legs, publisher to relay and relay to viewer, and each picks its own protocol: the relay re-serves what it ingests on all its listeners, so a stream published over SRT can be watched over RTSP.

No accounts. No remote control. Everyone publishes and watches at once.
Full color (4:4:4 + full range) - no WebRTC washout.

## Prereqs

- **ffmpeg** on PATH - includes `ffplay`. Recent build for `ddagrab`:
  `winget install Gyan.FFmpeg` (reopen shell after).
- Relay: native binary on Windows, or Docker on a Linux box (see below).

## 1. Start the relay

**On Windows** - run native. Docker Desktop's UDP proxy breaks SRT's handshake
(host→container SRT fails "I/O error"), so use the native binary - it binds
`:8890` directly, no NAT:

```bash
pwsh scripts/relay.ps1
```

First run downloads `mediamtx.exe` into `bin/`, then launches it with `mediamtx.yml`.
Add `-Background` to run hidden. Ctrl+C to stop foreground.

**On a Linux relay box / VPS** - Docker is fine there (UDP forwards correctly):

```bash
docker compose up -d
```

Either way: SRT (8890/udp), API (9997), plus RTSP/RTMP/HLS/WebRTC.

## 2. Publish your screen

```bash
pwsh scripts/publish.ps1 -Name bjorn
```

High-quality example (crisp text/color, GPU capture):

```bash
pwsh scripts/publish.ps1 -Name bjorn -Fps 144 -Bitrate 120M -Codec hevc_nvenc -Chroma yuv444p -Capture ddagrab
```

Key flags: `-Fps -Bitrate -Codec -Chroma -Range -Capture -Monitor -Relay`.
`-Chroma yuv444p -Range pc` = the no-washout combo.

## 3. See who is live

```bash
pwsh scripts/whoislive.ps1
```

Lists active streams from the relay API (account-free discovery).

## 4. Watch a friend

```bash
pwsh scripts/watch.ps1 -Name friendA -Relay <their-relay-ip>
```

## Topology

```
You (publish.ps1) ──SRT──┐
Friend A          ──SRT──┼──► MediaMTX relay ──SRT──► anyone (watch.ps1)
Friend B          ──SRT──┘        (docker)            HLS/WebRTC for browser
```

For friends across the internet: run the relay on one box everyone can reach
(VPS, or a Tailscale IP), pass its address via `-Relay`.

## Bandwidth reality

- **Upload** = sum of your publish bitrates.
- **Download** = sum of bitrates you watch.
- Relay egress = publishers x viewers x bitrate - scales fast. Start modest
  (40–80 Mbps HEVC 4:4:4 already beats Discord), crank when few are watching.

## Next

Tray app wrapping these commands (settings UI + live bandwidth meter).
