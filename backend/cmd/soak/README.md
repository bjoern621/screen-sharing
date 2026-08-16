# The soak probe

Drives the control contract with random legal settings and states what the backend did about them.

Every value it writes comes off the form's own enabled options and declared ranges, so it only ever
asks for what a reader could have asked for.
A finding is therefore a defect and not an illegal input.

## Running it

```
scripts/start.sh     # builds both binaries, starts an isolated backend, puts the probes on it
scripts/stop.sh      # ends the loops, the probes and that backend, and nothing else
```

Findings land as JSONL under `$SOAK_ROOT`, one file per mode, deduplicated by signature so one defect
repeated ten thousand times is five lines.
`SOAK_ROOT` defaults to `~/.cache/screenshare-soak`.

One field's shape, without running anything:

```
bin/soak -dump publish.maxrate_mbps -backend-pid <pid>
```

Needs the flake's dev shell for GStreamer and ffmpeg, an X display, and a relay at the address the
isolated settings name.
A local MediaMTX on offset ports keeps every capture on the machine:

```
sed -e 's/^rtspAddress: :8554/rtspAddress: :18554/' -e 's/^srtAddress: :8890/srtAddress: :18890/' \
    -e 's/^apiAddress: :9997/apiAddress: :19997/' mediamtx.yml > $SOAK_ROOT/mediamtx.yml
mediamtx $SOAK_ROOT/mediamtx.yml
```

## It runs beside the app, never inside it

`scripts/env.sh` moves four things, and every one of them is needed:

| Variable | What it moves |
| --- | --- |
| `XDG_RUNTIME_DIR` | the control socket, so this instance neither finds nor is found by a shell's |
| `XDG_CONFIG_HOME` | the settings, presets, portal token and run logs |
| `SCREENSHARE_TEST_STREAMS=0` | no synthetic publishers on boot, leaving the relay paths free |
| `PULSE_SERVER` | named outright, libpulse looking for it under the runtime directory just moved |

The session is stated as x11 so the capture list offers the portal-free backends.
A probe capturing through the portal would pop a consent picker on somebody's screen.

`scripts/findpid.sh` matches the backend on that runtime directory rather than on process order,
which is what keeps a probe from watching, or killing, the one a shell started.

## The modes

| Mode | What it drives | Cost |
| --- | --- | --- |
| `form` | one legal move per resolve, then what the resolver owes that draft | no silicon |
| `encode` | `MeasureEncodeRate` on generated frames: a real encoder, no capture | CPU and GPU |
| `publish` | a real stream to the relay, held and measured, then stopped twice | CPU and GPU |
| `multi` | the same measurement at 0, 1, 3, 6 and 9 competing publishers | CPU and GPU |

The measuring three run in turn, competing for one machine being what they measure.
The form walk runs beside them.

## What it holds a run to

Repairs settle, an offered option is legal, a greying names a reason, a publishable draft renders a
command.
Frames arrive at the rate that was asked for, the bitrate lands near the ceiling, nothing is dropped,
nothing retries, a stop leaves no child behind, and the same settings started twice are one stream.

A hardware family reaches the GPU's encode engine and a software one does not.
The reading is per process, out of `/proc/<pid>/fdinfo`, deduplicated by DRM client, so a second job
loading the same GPU moves no figure.
Two traps it is written around: one GPU context appears under every descriptor naming it, and a
process that ends takes its engine counters with it, which is why an operation is sampled while it
runs rather than bracketed. CPU time has the mirror trap, a reaped child's landing in the parent's
`cutime`.

Engine time is counted for the pipeline children alone.
The backend decodes the broadcast preview inside its own process, and that decode reaches the same
silicon an encode does.

## Reading a finding

```
{"kind":"publish.no_bitrate_reported","signature":"publish/no-bitrate/rtsp/libx264",
 "detail":"1152 frames were encoded over 39 samples and no sample carried a bitrate",
 "fields":{...},"settings":{...},"seed":1786844268,"iteration":3}
```

The settings are the whole draft, so a finding is reproduced by starting one publish on them.
The seed and the iteration replay the walk that reached it.

A finding is a claim, not a verdict.
Every one of these has to be held against the product before it is believed: the ones that turned out
to be the probe's own were a ramp measuring the machine's recovery, descriptors belonging to the
publishers a ramp had just started, and a relay port the walk had moved.
