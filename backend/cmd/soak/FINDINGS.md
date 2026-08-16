# Soak probe findings

The probe is `backend/cmd/soak`.
It drives the control contract with random legal settings and holds what the backend did against
what the settings asked for.

## How it runs

An isolated backend instance, so nothing here touches the running app: `XDG_RUNTIME_DIR` and
`XDG_CONFIG_HOME` point into this directory, `SCREENSHARE_TEST_STREAMS=0` keeps the synthetic
publishers off, and the session is stated as x11 so the capture list offers the portal-free
backends. No consent picker reaches anybody's screen. The relay is a local MediaMTX on offset
ports, so no capture leaves the machine.

Four modes:

| Mode | What it drives | Cost |
| --- | --- | --- |
| `form` | one legal move per resolve, then what the resolver owes that draft | no silicon |
| `encode` | `MeasureEncodeRate` on generated frames, real encoder, no capture | CPU and GPU |
| `publish` | a real stream to the relay, held and measured, then stopped twice | CPU and GPU |
| `multi` | the same measurement at 0, 1, 3, 6 and 9 competing synthetic publishers | CPU and GPU |

The GPU reading is per process, out of `/proc/<pid>/fdinfo` `drm-engine-*`, deduplicated by DRM
client. A second job loading the same GPU moves no figure. Encode engine time is counted for the
pipeline children alone, the backend decoding the broadcast preview in its own process.

## Confirmed

### 1. The bitrate range the form offers overflows every encoder

`fieldRateCeiling = 10000` (`backend/internal/form/fields.go`) bounds `publish.bitrate_mbps` and
`publish.maxrate_mbps`. Encoders take the rate as a 32-bit integer of bits per second, so anything
above 2147 Mbit/s overflows.

Observed on both paths:

```
[libx264 @ ...] Value 9131000000.000000 for parameter 'maxrate' out of range [-2.14748e+09 - 2.14748e+09]
[h264_vaapi @ ...] Value 7639000000.000000 for parameter 'maxrate' out of range [0 - 2.14748e+09]
WARNUNG: Die Eigenschaft »target-bitrate« im Element »vp9enc« konnte nicht auf »7639000000« gesetzt werden
```

A publish on such a value dies at launch and exhausts the retry budget. `MeasureEncodeRate` fails
the same way.

`fieldBitrateBounds` already states the intent: "An encoder with a ceiling refuses the encode
rather than clamping, so a target above it is a publish that dies at launch, and the range is where
that is cheapest to say." The per-codec narrowing works. The global ceiling above it does not.

### 2. The keyframe range the form offers exceeds what AMF takes

`fieldGopCeiling = 6000` bounds `publish.gop`. AMF maps it to `header_spacing`, whose range is
-1..1000.

```
[h264_amf @ ...] Value 2901.000000 for parameter 'header_spacing' out of range [-1 - 1000]
```

Same shape as the bitrate ceiling: a value the form offers as enabled, and a pipeline that refuses
it at launch.

### 3. The maxrate range starts below the target it may not go under

`publish.maxrate_mbps` is offered from 0 while `repair.go` walks any value under
`publish.bitrate_mbps` up to it. The slider therefore offers positions that are always silently
replaced. Either the range starts at the target, or the walk is not a repair.

### 4. The encode-rate probe refuses a measurement taken under load

`encoderate` times two content ends and refuses where the harder one measured faster:

```
cannot measure the encode rate: the encoder timed faster on the harder content (113.4 ...)
```

At rest the measurement is steady: five readings of one codec at one setting gave 12.3, 13.9,
14.0, 13.9 and 14.0 fps. Under competing encodes the two ends invert routinely, and the call answers
`UNAVAILABLE` rather than a wider bracket. A reader who measures while anything else on the machine
encodes is told the measurement failed instead of being told a range.

### 5. A stream on the ffmpeg engine reports no bitrate at all

Publishing `x11grab` to RTSP: 1152 frames over 39 samples, 60 fps reported on every one, and not a
single sample carrying `inst_mbps` or `avg_mbps`. The same stream through the GStreamer engine
(`ximagesrc`) reports a rate on every sample and raised no finding of any kind.

The cause is honest and documented in `ffmpeg/progress.go`: ffmpeg's RTSP muxer answers `total_size`
and `bitrate` with `N/A`, so `haveBytes` stays false and the parser marks both figures missing
rather than inventing one. The GStreamer engine taps its own byte counter off a loopback sink
(`publish/gststats.go`), so it always has one.

What a reader sees is a bitrate readout that stays empty for a whole session on one engine and
works on the other, with nothing on screen saying which they are on.

### 6. Run logs are never pruned

`~/.config/screenshare/logs` holds 3577 files and 95 MB from ordinary use. One file per run, and
nothing removes them. `control/effects.go` carries a comment about rotation; no pruning code
exists.

## Not defects, checked and dismissed

- `publish.audio_sources[N].source = "none"` reports as a repair of that key. It removes the entry,
  so the key it names is gone rather than walked.
- A second `StartPublish` with the same settings answers OK. That is the idempotency the repository
  is built on, not a missing refusal. The probe now asks for a *different* pipeline to test the
  refusal.
- `@DEFAULT_MONITOR@` in a rendered command is libpulse's magic name for the default sink's
  monitor, passed through on purpose.
- A ramp reading that rose with the load was the probe's own: it measured the next step before the
  previous step's publishers were gone, so the baseline priced the machine's recovery. The ramp
  settles for ten seconds after a load change now.
- `encode.probed_usable_but_unrunnable` fires on codecs the encoder probe passed and the machine
  then refused. Every instance so far is a parameter out of range, so it is the ranges above rather
  than a probe that answers for encoders it never started.
- Descriptors climbing 457, 805, 1158 during a load ramp were the synthetic publishers' own, around
  113 each. The watchdog reads the backend's own descriptors now, a child's going with it when it
  ends. The backend holds around 100, mostly sockets.
- A second `StartPublish` carrying another bitrate, answered OK over a running stream on a
  `lossless` draft. A bitrate reaches no lossless encoder, so that draft builds the pipeline already
  running. The check renders both drafts through `ResolveForm` now and expects a refusal only where
  the commands differ.
- `stop_failed`, `child_leaked`, `cycle_memory` and `rpc.resolve_failed` entries carrying
  `context canceled` are the probe being stopped mid-stream to swap binaries, not the backend.

## Open

The status code for an encoder that fails at launch is `UNAVAILABLE`, which `docs/ipc-api.md`
defines as the relay being unreachable or a child that could not be started. The child started and
exited on a parameter it would not take.

## What a second encode costs the first

The ramp times one encoder while synthetic publishers run beside it. Each publisher is an x264
software encode, so nothing competes for the GPU's encode engine: what the ramp prices is the rest
of the machine.

| Encoder | alone | 1 beside it | 6 beside it | 9 beside it |
| --- | --- | --- | --- | --- |
| `hevc_vaapi` | 280.9 fps | 228.2 fps (81%) | | 196.0 fps (70%) |
| `hevc_amf` | 142.7 fps | | 114.9 fps (81%) | |

A hardware encoder loses about a fifth of its rate to one competing software stream and about a
third to nine, though the competitors never touch the encode engine. The first competitor costs
most and the curve flattens after it, which is what capture, conversion and scheduling competing
for cores looks like rather than the encoder itself running short.

Half the readings under load were refused outright rather than measured: five of ten with
publishers running, none at rest. That is finding 4, and it is also why two cells above are
empty.
