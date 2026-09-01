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

A memory reading is the backend process alone. A tree figure moves by hundreds of megabytes and a
hundred threads depending on whether a pipeline happened to be up at that tick, and what a leak is
about is what the parent does not give back once every child is gone.

## Open

### The encode delay is a figure one engine measures

The broadcast header promotes four figures off an encoder sample. Three of them arrive on both
engines. `transit_ms`, drawn as "ms encode", arrives on the GStreamer engine alone:

| | `fps` | `inst_mbps` | `transit_ms` | `time_sec` |
| --- | --- | --- | --- | --- |
| GStreamer | yes | yes | yes | yes |
| ffmpeg | yes | yes | no | yes |

The GStreamer engine measures it in the pipeline itself, between the capture stamping a frame and
the encoded stream leaving (`internal/pipedelay`). ffmpeg's `-progress` output states nothing about
how long a frame was held, and this side runs no pipeline of its own to read it off, which
`ffmpeg/progress.go` says outright by marking the figure missing.

What a reader meets is a row that prints an ellipsis for the whole session on one engine and a
number on the other, with nothing on screen saying which they are on. Two ways out, both bigger than
the reading: measure the delay on the ffmpeg engine, or put the absence on the contract so the header
can say why the row is empty, as it already does for a round trip nothing timed
(`HeaderStatsViewModel.Untimed`).

### A decode context leaves its driver threads behind

Each local preview builds a decode pipeline in this process and the pipeline is handed back when the
stream stops (`Receiver.release`), so the elements, the pads and the descriptors go with it: a
backend that ran twenty-six software cycles ends holding sixteen descriptors.

The threads do not. Eighteen `hevc_vaapi` cycles leave a backend at 184 threads with nothing
publishing, and every one of them belongs to the video driver rather than to a pipeline:

```
     72 backen:traceq0
     18 backen:sh_opt0
     18 backend:sh0
     18 backend:disk$0
```

One set per cycle, which are Mesa's shader-compiler and disk-cache workers: a decode context is
created per preview and the driver keeps its threads for the life of the process. A software decode
costs about a fifth as many.

Nothing here holds the context. Reaching it would mean one decode context outliving the previews that
use it, which is a pipeline held open across streams rather than a leak to close.

## Answered

Each of these reproduced against the product and no longer does. What answers it is named beside it.

- **A rate the form offers overflows what the encoder derives from it.** `vah265enc` died at launch
  on a `cpb-size` out of range, at a target the form offered and the resolve left alone. The rate
  buffer is now bounded by the field it is read into, per codec and per engine
  (`capabilities.Codec.BufferLimitKb`, `form.bufferCeilingKb`), and the builder refuses a pair past
  it rather than handing GLib a value it will not take (`publish.vaLimits`).
- **A keyframe interval the form offers overflows `key-int-max`.** The same shape one property over,
  and the interval an unset field derives from the frame rate reaches it too. The va rows declare
  their bound (`vaGopLimit`), and `vaLimits` weighs the interval the elements are handed rather than
  the one a reader set.
- **The rate scale runs past what the GStreamer elements take.** `x265enc` stops at 102 Mbit/s,
  `x264enc` and the va elements at 2048, where the scale ran to 2147. Declared per engine on the
  rows (`BitrateLimitM`).
- **The target rate is offered from zero.** A draft holding zero in a mode that sends the encoder a
  target was publishable and priced at 0.00 Mbit/s. The floor is a rate in those modes now, and a
  stored zero is walked up to it (`form.fieldBitrateBounds`, `repairCeilings`).
- **A preset can produce a draft the encoders refuse.** `gaming` states its own target and carries
  the draft's burst ceiling, and the va elements express a VBR target as a percentage of that
  ceiling. The search now asks whether the elements can express a candidate before returning it
  (`publish.EncoderRefusal`).
- **An entry is emphasised and greyed at once.** `optionCursors` marks the embedded pointer on every
  draft and the scanout backend cannot draw one. A recommendation is dropped where the same
  combination rules the entry out, for every field rather than that one (`form.resolveOptions`).
- **The WebRTC leg is offered where the element it launches is missing.** An install carrying
  `whipsink` and not `whipclientsink` passed every encoder probe and died at launch. The probe now
  asks the registry about the elements each leg's sink is made of, and the leg greys with the
  element named (`encoders.Availability.Legs`, `TEXT_CODE_PUBLISH_SINK_ELEMENT_MISSING`).
- **The drop counter counts frames in flight and falls back when they drain.** `dropped_frames` fell
  from 1 to 0 on a healthy stream: the count was the difference between two pad probes, which
  includes whatever the queue is holding. The depth is subtracted now, and the total is a high-water
  mark so three readings taken one after another cannot lower it (`gstrun.shedCount`).
- **The backend held every pipeline it ever built.** Twenty-six publish cycles took an isolated
  backend from 28 MiB, 10 threads and 32 descriptors to 184 MiB, 150 threads and 241 descriptors,
  with nothing publishing and no child left. `receive.Receiver.Stop` took the pipeline to `StateNull`
  and dropped it: the binding unrefs a wrapper when Go collects it, and Go sees the wrapper rather
  than the decoder contexts and buffer pools behind it. The pipeline is handed back explicitly now
  (`Receiver.release`), by whichever side ends it first, and the same twenty-six cycles end at 73
  threads and 16 descriptors with the resident figure holding flat between streams instead of
  climbing every minute. What still climbs is below it, and is the entry under "Open".
- **The bitrate range overflowed every encoder** at `fieldRateCeiling = 10000`, and **the keyframe
  range exceeded what AMF takes**: both bounded on the rows (`amfGopLimit`, `RateFieldM`).
- **The burst ceiling was offered below the target it may not go under.** The band starts at the
  target in the modes that send one (`fieldMaxrateBounds`).
- **The encode-rate probe refused a measurement taken under load.** A bracket is answered in the
  order the two ends came out, with the inversion logged rather than refused (`encoderate.bracket`).
- **Run logs were never pruned.** `ffmpeg.newRunLog` takes the oldest off before opening the next,
  and the directory holds `runLogKeep` files.
- **`UNAVAILABLE` named only a child that could not be started**, where a child that started and
  exited on a value it would not take wears the same code. `docs/ipc-api.md` states both.

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
  `context canceled` are the probe being stopped mid-stream to swap binaries.
- A slider's stops are the round figures inside its band plus both ends, not a grid counted off the
  floor, so a 20 ms floor stepping by 50 stops on 20, 50, 100 and reaches 8000
  (`avalonia/.../Fields/ViewModel/FieldViewModel.cs`, `Ticks`). A probe writing between two stops
  reported every answer to a value no reader can reach.
- `publish.audio_sources[N].gain` coming back as 100 where a draft carried 0 is the wire's own
  answer: gain carries presence so that a silent source and an unset one differ, and a read of an
  absent field answers zero either way (`wire/settings.go`, `audioGain`). The probe skips a key the
  draft never stated.
- A counter falling across a relaunch is the new child counting from zero, which the probe reports
  as `publish.relaunch_resets_readout` rather than as a counter going backwards. What a reader sees
  is still a timer and a frame count that start again under a stream they never stopped.
- Numbers on the shell's screens spell their decimal point invariantly whatever the machine's
  locale: `InvariantGlobalization` is on for every project under `avalonia/`
  (`Directory.Build.props`), so a call site formatting without a culture gets the invariant one.

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
