# The soak probe

Drives the control contract with random legal settings and states what the backend did about them.

Every value it writes comes off the form's own enabled options and declared ranges, so it only ever asks for what a reader could have asked for.
A finding is therefore a defect in what the backend did with a legal request.

## Running it

```
scripts/start.sh     # builds both binaries, starts an isolated backend, puts the probes on it
scripts/stop.sh      # ends the loops, the probes and that backend, and nothing else
```

Findings land as JSONL under `$SOAK_ROOT`, one file per mode, deduplicated by signature, so one defect repeated ten thousand times is five lines.
`SOAK_ROOT` defaults to `~/.cache/screenshare-soak`.

One field's shape, without running anything:

```
bin/soak -dump publish.maxrate_mbps -backend-pid <pid>
```

Needs the flake's dev shell for GStreamer and ffmpeg, an X display, and a relay at the address the isolated settings name.
`task relay` starts one on this machine.
Offset ports keep every capture here without taking the ports a relay somebody else started is on.
MediaMTX reads each address off the environment, so the configuration stays the one every relay reads:

```
MTX_SRTADDRESS=:18890 MTX_RTSPSADDRESS=:18554 MTX_RTMPSADDRESS=:11936 MTX_APIADDRESS=127.0.0.1:19997 task relay
```

## It runs beside the app

`scripts/env.sh` moves four things:

| Variable | What it moves |
| --- | --- |
| `XDG_RUNTIME_DIR` | the control socket, so this instance neither finds nor is found by a shell's |
| `XDG_CONFIG_HOME` | the settings, presets, portal token and run logs |
| `SCREENSHARE_TEST_STREAMS=0` | no synthetic publishers on boot, leaving the relay paths free |
| `PULSE_SERVER` | named outright, libpulse looking for it under the runtime directory just moved |

The session is stated as x11 so the capture list offers the portal-free backends.
A probe capturing through the portal would pop a consent picker on somebody's screen.

`scripts/findpid.sh` matches the backend on that runtime directory rather than on process order, so a probe never watches, or kills, the one a shell started.

## The modes

| Mode | What it drives | Cost |
| --- | --- | --- |
| `form` | one legal move per resolve, then what the resolver owes that draft | no silicon |
| `encode` | `MeasureEncodeRate` on generated frames: a real encoder, no capture | CPU and GPU |
| `publish` | a real stream to the relay, held and measured, then stopped twice | CPU and GPU |
| `multi` | the same measurement at 0, 1, 3, 6 and 9 competing publishers | CPU and GPU |

The measuring three run in turn, competing for one machine being what they measure.
The form walk runs beside them.

A publish run holds whatever `-capture` and `-codec` name, so one run answers for one engine and one encoder rather than for whichever ones the walk reached.
A move elsewhere that leaves a pinned value with no legal form sends the run back to the draft it opened with.

## What it holds a run to

Repairs settle, an offered option is legal, a greying names a reason, a publishable draft renders a command.
Frames arrive at the rate that was asked for, the bitrate lands near the ceiling, nothing is dropped and nothing retries.
A stop leaves no child behind, and the same settings started twice are one stream.

### What a control owes the screen

A widget draws `Field.value` and a start sends the draft, so the two carry one number.
A slider stops on the round figures inside its band and on both ends, so a held value off that ladder is one no drag lands on.
An entry is listed once, and one marked for emphasis can be chosen.
`publishable` and a blocking diagnostic agree in both directions, and a diagnostic names a field the form draws.
A publishable draft predicts a rate above zero and renders a command carrying no figure past what an encoder takes.
A preset applies to a draft that publishes, settles without a repair and comes back marked as delivered.

Each run ends by stating the entries it was offered and never held, and the bands it never stood at an end of.
A corner nothing reached is a gap in the run rather than a defect in the product.

### What the screen shows of a running stream

The broadcast screen draws four figures off an encoder sample: the frame rate, the rate over the last interval, the transit and the clock.
An absent figure prints as an ellipsis and holds its last measurement, so a figure no sample of a whole run carries reads empty for the session.
Each is checked for presence, for being finite, and for landing inside what a reader could act on.
A rate and a clock have to arrive on one sample or the egress plot draws nothing.
The counters count up, the clock moves while frames arrive, and the relay names a path for the stream while it runs.
That path is where the viewer count, the round trip and the loss are all read from.

A hardware family reaches the GPU's encode engine and a software one does not.
The reading is per process, out of `/proc/<pid>/fdinfo`, deduplicated by DRM client, so a second job loading the same GPU moves no figure.
Two traps it is written around: one GPU context appears under every descriptor naming it, and a process that ends takes its engine counters with it.
An operation is therefore sampled while it runs rather than bracketed.
CPU time has the mirror trap, a reaped child's landing in the parent's `cutime`.

Engine time is counted for the pipeline children alone.
The backend decodes the broadcast preview inside its own process, and that decode reaches the same silicon an encode does.

The rendered command is read beside that reading: a hardware family whose pipeline names a CPU encoder is coding on cores while the settings, the greying and the estimate all say hardware.
The CPU encoders it matches are the catalog's own rows, through `publish.GstEncoderElement`, so a codec joining the domain joins this check with it.

## What a leak is read off

The backend process alone.
A tree figure moves by hundreds of megabytes and a hundred threads depending on whether a pipeline happened to be up at the moment of the reading.
A leak is what the parent does not give back once every child is gone.
The tree stands beside it as context, and a child that outlived its stop is `publish.child_leaked` rather than a memory reading.

Every run ends on a `backend.drift` line stating what the backend held at the start against what it held at the end.
The line lands whether or not the climb was steep enough to be reported while it happened.
A threshold answers yes or no, and a leak hunt needs the figure.

## Reading a finding

```
{"kind":"publish.no_bitrate_reported","signature":"publish/no-bitrate/rtsp/libx264",
 "detail":"1152 frames were encoded over 39 samples and no sample carried a bitrate",
 "fields":{...},"settings":{...},"seed":1786844268,"iteration":3}
```

The settings are the whole draft, so a finding is reproduced by starting one publish on them.
The seed and the iteration replay the walk that reached it.

Every finding is held against the product before it is believed.
Some turn out to be the probe's own: a ramp measuring the machine's recovery, descriptors belonging to the publishers a ramp had just started, a relay port the walk had moved.
