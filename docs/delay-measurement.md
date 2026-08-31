# Measuring the delay

Every stage between a screen and a window is measured, or named and left without a figure.
Nothing is derived from a setting: a window a transport asked for is not what a packet took.

Two measuring points do the work, one per pipeline, and a clock inside the picture joins them.

## Where the readings are taken

```mermaid
flowchart LR
    subgraph P["publishing machine"]
        C[capture] --> E[encode] --> S(("stamp<br/>and probe"))
    end
    S --> R[relay]
    R --> D(("probe")) --> V[decode] --> W[window]
```

Both points sit on the encoded stream and never on a picture.
On the publishing side that is the encoded-frame counter's source pad, past the parser.
On the receiving side it is the decoder's sink pad.
So no measurement maps a video surface, and the frames a capture or a decoder leaves on the GPU stay there.

## What each row is

| Row | Measured at | By |
| --- | --- | --- |
| Capture and encode | publish pipeline, encoded-frame counter | its own clock, less the frame's running time |
| Publisher to relay | publish sink | the transport's stated window |
| both of those, on another machine's stream | the pictures themselves | the publisher's own readings, stamped into each one |
| Through the relay | derived | the timed way here, less both windows |
| Relay to here | watch leg's elements | the transport's stated window |
| Decode | receive pipeline, sink pad | its own clock, less the frame's running time |
| Held for play time | derived | the latency query, less the decode |
| At least, end to end | derived | the measured stages, added |

The way between the two machines is timed as a whole and drawn on no row of its own.
It is what the relay's row is derived from and what the total counts in place of the two windows it already covers, so a row would repeat what those two already carry.
It crosses the contract as `path_ms` all the same, this side stating what it measured and the panel deciding what to draw.

The total therefore stands above what the rows above it add up to, by whatever the windows did not account for.
On a pair of legs stating no window at all, that gap is the whole way between the machines and the rows above carry none of it.

`backend/internal/app/receivedelay.go` assembles the budget.
Which stage a figure belongs to and which figures may be added is a decision, so it is made once, and no shell makes it again.

Every figure is a mean over the interval between two samples, carried as a sum and a count so the interval stays the reader's.
The exception is the decode's worst single frame, a high-water mark over the whole run: a mean holds steady while single frames run long, and the long one is what a sink's deadline is judged against.

## The clock in the picture

A relay terminates one protocol and re-muxes for each listener, so no leg's counters cross it and it reports no delay of its own.
What crosses unchanged is the coded picture, a re-mux being no re-encode.

So the publisher writes into an unregistered SEI message ahead of each picture what only it can know, and the viewer reads it at the decoder's input (`internal/framestamp`).

Two things ride there.
The wall clock the picture left the encoder at, which the viewer subtracts: that reading spans both legs and the relay as one figure, over any transport, and it is the only figure between the two machines that is a measurement rather than a window a transport asked for, which is why the relay's share and the total both come off it.
The publishing pipeline's own running totals beside it, which is what puts capture and encode, and the window that leg settled on, in front of a viewer of somebody else's stream.

Those totals cross as a sum and a count and never as an average, so the viewer divides them over its own sampling interval exactly as it divides its own counters.
Where this machine is the publisher too it reads its own run instead, the same figure at full precision and there for a codec that carries no stamp at all.

The message goes behind the parameter sets and in front of the first picture unit.
Put ahead of them it is dropped by the parser that meets the stream there, which costs the first frame of every stream and of every mid-stream join.

Its identifying bytes hold no zero, so the emulation-prevention rule never rewrites them and a reader searches for them rather than parsing a bitstream it did not frame.

**Cost.** Sixty bytes a frame, and one more packet per frame on the legs that payload into RTP, the payloader giving each unit its own.
Around thirty kilobits a second at 60 fps: a third of one percent of an eight megabit stream, and proportionally more of a thinner one.
It scales with frame rate and not with bitrate, being a fixed size on every picture.

## What is left unmeasured

| Absent | Because |
| --- | --- |
| everything the message carries | a codec with no unit for it, a publisher that is not this app, or the ffmpeg engine, which exposes no point to write at |
| the relay's own share | the timing of the way here missing, or either leg stating no window of its own |
| capture and encode | a publish that measured none of its own stages, on a stream this machine does not publish either |
| a leg's window | a transport that states none, its buffering falling under the decode instead |

Each is a row without a figure and never a row left out: a total presented as the whole journey would be wrong by exactly what is missing, which is why it is stated as a floor.

Across two machines the subtraction is against two clocks and is worth what their synchronisation is worth.
A stamp from ahead of the reader's clock is dropped rather than shown as a path of no length.
A viewer of its own stream reads one clock and is exact.

The publishing figures are not part of that: they are one pipeline's readings of itself, carried rather than subtracted, so a pair of clocks too far apart to time the way here still leaves a viewer the stages ahead of it.
