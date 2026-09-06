# Measuring the delay

Every stage between a screen and a window is measured, or named and left without a figure.
Nothing is derived from a setting or from what a transport states: a delay a leg asked for is not what a packet took.

Two measuring points carry a clock between the pipelines, and a third brackets what the receiving pipeline itself spends.

## Where the readings are taken

```mermaid
flowchart LR
    subgraph P["publishing machine"]
        C[capture] --> E[encode] --> S(("stamp<br/>and probe"))
    end
    S --> R[relay]
    R --> D(("probe")) --> V[decode] --> K(("probe")) --> W[window]
```

The two points the clock joins sit on the encoded stream.
On the publishing side that is the encoded-frame counter's source pad, past the parser.
On the receiving side it is the decoder's sink pad.
The third sits at the sink, where a decoded frame is handed to the window.
So no measurement maps a video surface, and the frames a capture or a decoder leaves on the GPU stay there.

## What each row is

| Row | Measured at | By |
| --- | --- | --- |
| Capture and encode | publish pipeline, encoded-frame counter | its own clock, less the frame's running time |
| that row, on another machine's stream | the pictures themselves | the publisher's own reading, stamped into each one |
| Publisher to here | both measuring points | the clock in the picture, against this machine's |
| Buffered here | receive pipeline, decoder sink pad | its own clock, less the frame's running time |
| Decode | receive pipeline, sink pad | the same subtraction, less what the buffering took |
| Slowest frame | receive pipeline, sink pad | the worst that subtraction has ever read |
| Held for play time | derived | the latency query, less the buffering and the decode |
| End to end | derived | the measured stretches, each counted once |

Every row is a stage any transport can fill.
A leg's own delivery window is not one: SRT states a window and the other four transports state nothing, so a row for it would stand blank on most legs.
What a leg holds a packet for is the buffering row: an RTSP jitter buffer spends its window there, and an SRT leg buffers inside its source and leaves that row the demuxer and the parser.

Two readings cross that row.
The way here ends at the decoder, and the receiving pipeline's own subtraction starts at the leg's source, ahead of it.
So the stretch between the source and the decoder lies inside both, and the decode row is the receiving reading less it.

## Adding them up

The total is the stretches that carry a figure, added, each counted once.
The way here holds both the buffering row and the relay inside it, so the buffering row is added alone only where the way here is blank.
A way here with a figure makes the sum the whole journey, and one without leaves it short by everything between the machines, which is why it is stated as a floor.

The budget is assembled once, on this side.
Which stage a figure belongs to and which figures may be added is a decision, so no shell makes it again.

Every figure is a mean over the interval between two samples, carried as a sum and a count so the interval stays the reader's.
The exception is the slowest single frame, a high-water mark over the whole run.
A mean holds steady while single frames run long, and the long one is what a sink's deadline is judged against.

## The clock in the picture

A relay terminates one protocol and re-muxes for each listener, so no leg's counters cross it and it reports no delay of its own.
What crosses unchanged is the coded picture, a re-mux being no re-encode.

So the publisher writes what only it can know into an unregistered SEI message ahead of each picture.
The viewer reads it at the decoder's input.

Two things ride there.
The wall clock the picture left the encoder at, which the viewer subtracts.
That reading spans both legs and the relay as one figure, over any transport, and it is the only measurement of the way between the two machines.
The publishing pipeline's own running total beside it, putting capture and encode in front of a viewer of somebody else's stream.

That total crosses as a sum and a count, so the viewer divides it over its own sampling interval exactly as it divides its own counters.
Where this machine is the publisher too it reads its own run instead.
That is the same figure at full precision, and it is there for a codec that carries no stamp at all.

The message goes behind the parameter sets and in front of the first picture unit.
Put ahead of them it is dropped by the parser that meets the stream there.
That costs the first frame of every stream and of every mid-stream join.

Its identifying bytes hold no zero, so the emulation-prevention rule never rewrites them.
A reader searches for them rather than parsing a bitstream it did not frame.

The message costs fifty-seven bytes a frame, and one more packet per frame on the legs that payload into RTP, the payloader giving each unit its own.
Under thirty kilobits a second at 60 fps: a third of one percent of an eight megabit stream, and proportionally more of a thinner one.
Being a fixed size on every picture, it scales with frame rate.

## What keeps the publishing reading bounded

Two things stop the capture-and-encode figure growing without limit.

The trunk sheds ahead of the encoder, holding the newest frame and dropping the rest, so an encoder or a leg short of the capture rate costs frames instead of ageing every frame behind it.
What it dropped is counted and crosses on the same line, the two being one reading: what the shed threw away is what the delay did not grow by.

The software encoders run with their lookahead pinned off, frames an element holds leaving at whatever rate the transport takes.
Fifty held frames on a leg draining three a second is seventeen seconds of picture nobody has seen.

Frames the encoder repeated to hold the output rate and frames discarded before it are two counters.
Naming one after the other is how a health column ends up unable to move.

## Capture rate against encoded rate

How often the encoder emitted a frame and how often the screen produced a new one are two figures, and on a damage-driven capture they are far apart.
The pacer repeats the newest frame to hold the target, so the encoded rate follows the target whatever the screen does, and a counter downstream of it hands the target back as if it were a measurement.

So the capture rate is taken at the last point where one buffer is still one new picture.
It falls below the target both when the shared screen is static and when the capture path cannot keep up, which are the two things worth telling apart from a healthy stream.
It is a run's instrumentation, so a pipeline built without it reads unmeasured rather than zero.

The two engines' byte figures are not comparable: one counts the video elementary stream, the other the muxed stream with its audio track and container overhead.

## What is left unmeasured

| Absent | Because |
| --- | --- |
| everything the message carries | a codec with no unit for it, a publisher that is not this app, or the ffmpeg engine, which exposes no point to write at |
| capture and encode | a publish that measured none of its own stages, on a stream this machine does not publish either |
| the relay's own share | no reading is taken at the relay, so what it spent sits inside the way here |

The first two are still rows, drawn without their figures.
The third is a stage of the path and a row nowhere, there being nothing to draw.
A total presented as the whole journey would be wrong by exactly what is missing, so it is stated as a floor.

Across two machines the subtraction is against two clocks and is worth what their synchronisation is worth.
A stamp from ahead of the reader's clock is dropped rather than shown as a path of no length.
A viewer of its own stream reads one clock and is exact.

The publishing figure stands apart, being one pipeline's reading of itself, carried rather than subtracted.
A pair of clocks too far apart to time the way here still leaves a viewer the stage ahead of it.
