#!/usr/bin/env bash
# Builds both binaries, starts an isolated backend on them, and puts the probes on it.
# Stop with scripts/stop.sh.
set -eu
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$here/env.sh"

mkdir -p "$SOAK_ROOT/bin"

# A first run publishes to a relay on this machine rather than to whatever the defaults name:
# the probe starts streams unattended,
# and the streams of an unattended probe belong on loopback.
# The ports are the offset ones README.md names, so a relay somebody else started keeps its own,
# and each names the listener that relay actually binds:
# RTSP and RTMP terminate TLS, so those two are the encrypted listeners (deploy/mediamtx-groups.yml).
#
# A stream lives in a group, so the probe joins one of its own:
# a key drawn here on the first run, kept in the settings file beside every other value,
# and a name claimed under it. The group service beside the relay issues the token that key buys,
# and the SRT legs carry the passphrase both ends derive from it.
if [ ! -f "$XDG_CONFIG_HOME/screenshare/settings.json" ]; then
  mkdir -p "$XDG_CONFIG_HOME/screenshare"
  group_key="$(head -c 32 /dev/urandom | base64)"
  cat > "$XDG_CONFIG_HOME/screenshare/settings.json" <<JSON
{
  "relay": {
    "host": "127.0.0.1",
    "srtPort": 18890, "apiPort": 19997, "rtspPort": 18554,
    "webrtcPort": 18889, "rtmpPort": 11936, "hlsPort": 18888, "moqPort": 8892,
    "groupKey": "$group_key", "displayName": "soak"
  },
  "publish": {
    "name": "soak", "transport": "rtsp", "codec": "libx264", "mode": "cbr",
    "chroma": "yuv420p", "colorRange": "tv", "fps": 30, "cq": 23,
    "bitrateM": 6, "maxrateM": 8, "vbvMs": 0, "gop": 60, "bframes": 0,
    "effort": "veryfast", "capture": "x11grab", "monitor": 0, "uplinkM": 100
  },
  "viewer": {}
}
JSON
fi
(cd "$here/../../.." && go build -o "$SOAK_ROOT/bin/backend" ./cmd/backend && go build -o "$SOAK_ROOT/bin/soak" ./cmd/soak)

rm -f "$SOAK_ROOT/stop"
if ! "$here/findpid.sh" > /dev/null; then
  nohup "$SOAK_ROOT/bin/backend" > "$SOAK_ROOT/backend.log" 2>&1 &
  sleep 4
fi
"$here/findpid.sh" > "$SOAK_ROOT/backend.pid"
echo "isolated backend $(cat "$SOAK_ROOT/backend.pid"), writing to $SOAK_ROOT"

nohup "$here/supervise.sh" > "$SOAK_ROOT/supervise.out" 2>&1 &
nohup "$here/form-loop.sh" > /dev/null 2>&1 &
echo "probes running: form beside encode, publish and multi in turn"
