# Domain model as tables of facts

Codec, pixel format and rate-control mode are each a table of facts, and every rule derives from those tables rather than restating them.
One definition governs the encoder, the settings form, the bitrate estimate and the viewability verdict.

## The problem it removes

A codec carries many rules: which family it runs on, which pixel formats it encodes, which transport carries it, how efficient it is, which viewer decodes it.
Written imperatively they spread across files and drift.
The failure is two encodings of one constraint: the form greys an option while the normalizer still lets the value through.

## Where each fact lives

| Table | Holds, per row |
| --- | --- |
| codecs | the bitstream a codec produces, what produces it, the pixel formats it may encode, what its encoder cannot do, and the scales its quantizer and bitrate targets count on, all per publish engine |
| audio codecs | the element each engine codes it with, the sample rate the branch resamples to, the bitrate |
| second-track sources | the operating systems whose sessions serve a source, what serves it on each, and what one that does not is missing |
| capture and family pairs | whether frames reach the encoder without a trip through system memory, and what carries them |
| decoders | the pixel formats a decoder takes, and what bounds that list |
| carriage | per protocol, leg and engine: the video formats and audio codecs that engine puts through that leg |

The decoder table describes the machines watching.
A stream is published once and watched on whatever hardware the watchers have, so nothing in it is probed and nothing restricts a choice.
The form reads it to say what a pixel format costs a viewer: every format has a software decoder, so the choice is a viewer's GPU against a viewer's cores.

## The encode is two fields

A draft carries a format and an encoder.
Format is what every viewer decodes and what a transport carries.
Encoder is what produces it here, at the grain a picker offers one: the family wherever that family is one encoder, and the library where several share a family.

Neither field derives the other, so the pair is stored nowhere.
The grid is sparse: one vendor's runtime codes no VP9, so that pair names no row and the encoder control greys it with the formats that encoder does produce.

Two controls because the two answers move separately.
An encoder this machine cannot run moves the encoder alone, so a bitstream survives a card that is not there.
One field naming the whole encode has a single list to walk, and its first entry that runs decides the bitstream as a side effect.

| Codec | Chromas |
|---|---|
| H.264, NVENC | yuv444p, yuv420p |
| HEVC, NVENC | gbrp, yuv444p, yuv420p, p010le |
| AV1, NVENC | yuv420p, p010le |
| H.264, x264 | yuv444p, yuv422p, yuv420p, p010le |
| HEVC, x265 | gbrp, yuv444p, yuv422p, yuv420p, p010le |
| VP9, libvpx | gbrp, yuv444p, yuv420p, p010le |
| VP8, libvpx | yuv420p |
| AV1, libaom | gbrp, yuv444p, yuv420p, p010le |
| AV1, SVT | yuv420p, p010le |
| AV1, rav1e | yuv444p, yuv420p, p010le |
| VAAPI and QSV rows | yuv420p, and p010le on HEVC and AV1 |
| AMF and Vulkan rows | yuv420p, and p010le on HEVC and AV1 |

The chroma column is the union over the two publish engines, a format one engine's encoder will not take carrying a gap on the row, so the viewer side sees every chroma a stream may arrive in.
4:2:2 is the two software H.26x rows' alone: no hardware encoder here has an entrypoint for it, and the royalty-free formats have no 4:2:2 profile a fast encoder implements.

Two families have no GStreamer element at all, the only gaps taking a whole family off an engine.
One builds its device layer for Windows only; the other takes images on a device no capture backend on that engine produces.
On an AMD or Intel card the same silicon is reachable through the VAAPI rows.

Whether a GPU runs any of them is the driver's answer, the table stating only what a row could carry, and a probe per engine greys what this machine refuses.

## What a gap is

The two engines wrap different encoder implementations, so a pixel format, colour range, rate-control mode or whole codec can be one engine's and not the other's.
Each difference is a gap naming engine, option, value and reason, rather than a row narrowed to what both manage.
An option one engine reaches stays offered on that engine's capture backends and greys with the element's own limit on the other.
The form then says "no GStreamer encoder element takes planar-RGB input" instead of hiding the format from everyone.

A gap takes one value of one option away, and a list of options is what exists to be gapped.
Lookup, validator and frontend read that list rather than a field per axis.
A gap naming no option takes the codec off that engine entirely.
Gap values are the settings' own, so a gap and the control it greys are one identifier on both sides of the wire.

## What the installed driver gets wrong

A gap is what an encoder cannot do, and holds wherever the app runs.
What one driver gets wrong is a driver defect, and holds only while that driver sits under the encoder.

A defect names the driver, the option and value it withholds, optionally the adapter models carrying it, and the release that fixes it.
The identity is read out of the driver's own vendor string once per process, and the rows match on driver, model and version.
A machine naming no driver carries no defect, and a driver whose release version went unread keeps one.

Written down rather than probed, because trying it is the damage: one format under constant bitrate hangs an open-source driver's video block, and a probe would establish that by resetting the graphics device.
So the engine-scoped lookups keep answering what the encoder implements on any machine, and a defect reaches a form and a publish through the evaluator alone.

## One evaluator

A gap and a numeric ceiling are how a codec's limits are written, on the row they belong to.
Each becomes a rule, and every consumer asks that evaluator: the greying, the ends a numeric control is offered between, the repair, and the refusal.

A gap can only name a codec, an engine, an option and a value.
A fact about a capture backend and a codec together, or a codec and the platform, has no row to sit on.
Such a fact takes a table of its own with a consumer written against it, and that consumer restates part of an answer the gap mechanism already knows how to give.

A rule is a row:

- the facts it binds under, keyed by axis
- the verdict, stated and not derived
- the control it lands on
- which of that control's values it takes, as a value set or a numeric band
- the fact behind it, as a code

One axis makes it broad, five make it surgical, so "no VP8 encoder has a colour range field" and "this codec on this capture backend on this engine alone" are one shape.
A reason is assembled from the axes it matched, so a row states which fact it is.

Rules are declared where the fact lives and registered into the one evaluator, which keeps each transport declaring its carriage beside the code serializing it.
Only verdicts cross the wire; rules and axis vocabulary stay in the backend.

## The two ladders

How hard an encoder works and what it works towards are two settings, and each codec's row declares its own ladder for both.

Steps are the encoder's own identifiers: one counts in names, another in numbers to 13, another p1 to p7.
No scale is normalized across codecs, so a number carried across a codec change lands on a different real setting than the one held.
A step off the selected codec's ladder is reset to the one that codec's row declares for the mode, and the field is named in the repaired list so the change is readable.

Two fields rather than one: a live encode drops the lookahead and frame reordering a quality encode keeps, whatever effort it spends.

A row states where each mode starts on its ladder, and which modes pin the step instead of starting on it.
A pin is a fact about the encoder: one vendor fixes its preset under constant bitrate because a low-latency preset is what lets it hold a constant rate.
The form greys a pinned control and names the step in force, and both builders spend the declared step, so the sentence cannot name one the encode is not running.

## How each engine spells a step

Both builders read the row through one resolver, keeping a stream's look off the capture backend that produced it: one library through two bindings encodes alike.
Each engine still owns the spelling, and the spellings differ more than the option name does.
One says preset where the other says speed-preset.
One element splits a tune list across two properties.
Another's tune enum starts at a metric rather than at no tuning, so the untuned step is stated there by number.
The empty step is the one answer every builder shares: pass no such option at all.

A step the row does not declare is refused rather than forwarded, so a settings file that never went through the form fails naming the control rather than inside an encoder's error path.

What a tune step is differs by vendor, and the row states the vendor's own vocabulary.
Two name what the picture is or what the decoder needs, two the delay to hold, others a score to maximise, and one the scenario the session is for.
Where one engine's wrapper has no such knob the steps are gapped there rather than dropped from the row, so the option stays on the engine that reaches it.

A knob one engine spends and the other does not is a departure rather than a gap, the control being the same control on both.
VAAPI is the case: its rows declare seven target usages, where the other engine counts over the range the installed driver reports, measured 0..32 on one driver against 1..7 on another.
One step carried across would spend a different amount of work per engine and per card, so one builder spends none and the form greys the control there with that reason.

## Rows with no ladder

A row declaring no ladder is an encoder with no such knob, which greys the control naming that codec.
Either ladder can be absent alone: the Vulkan rows tune and take no effort step.

No VA profile carries a tuning hint, so the VAAPI rows tune for nothing.
AMF declares effort and no tune though the API has a usage hint, because this app pins it: a low-latency usage drops the keyframe period and leaves a late subscriber no recovery point.
VideoToolbox declares neither, the framework taking no preset and no tuning hint, and what stands in for both is the realtime flag every mapping sets.

## The two audio settings

Audio is two settings against two tables, source and codec answering different questions.
Which sources exist is the platform's answer.
Which codec the track is coded in is the engine's and the leg's, and that row states the element each engine codes it with, the sample rate and the bitrate.

### The capture sources

A row names the settings value, the operating systems whose sessions serve it, what serves it on each, and what one that does not is missing.

It differs per platform because the engines do.
On a Linux sound session both engines open desktop audio as the monitor of the default sink, under one shared device name: two spellings of one server's name are two things able to disagree about which device a stream records.
On Windows one engine opens the default render device's loopback and the other has no input for it at all.
macOS serves nothing, reading what it plays needing a process tap neither engine has an element for.
Each refusal names what that machine is missing rather than saying "Linux only": a user who reads why macOS cannot do it cannot act on why Windows cannot.

Which engine opens a source is a second question, answered where the capture backends are, a backend fixing the engine.
Both places a source is one engine's are on that table.

**A kind is declared and what is inside it is enumerated**, so they are two controls and two answers.
Whether a machine serves desktop audio at all is the table's.
Which outputs a machine plays into no table can hold, so they are read off the sound server once and cached for the process lifetime.
A kind with nothing enumerated still has one thing in it: its own default, which is what an entry naming no device takes.
A selection the enumeration stops reporting stays on the list with a note rather than being dropped, the way a monitor index does: an application that is not running may be running when the stream starts.

### The second track is a list

A screen share is normally several sources at once, so the setting is a repeated source, device, gain and mute, and the entries mix into one track.
Carriage forces one track: RTMP carries one audio track and the relay re-serves every ingest on all listeners, so a two-track stream would be unplayable on the narrowest leg while the form said it published.

Each entry is addressed by an indexed key, so every control kind the form has edits a list item and a statement lands on one entry rather than on the control.
The list grows through the settings: the form draws one row past the end, and picking a kind on it is the write that adds the entry.
Setting a kind back to the absent one takes an entry off on the next repair.

Gain carries presence, zero being a level: a source turned all the way down is silent, and an entry nobody set a level on is at unity.
Without presence the two would be one value and the entry a reader creates would arrive silent.
Mute is a level too, the mixer multiplying by zero, which keeps unmuting a write to a running pipeline rather than a rebuild of the audio graph, and makes a list of nothing but muted sources still carry a track.

The engines read those rows through one derivation rather than each with a table of its own.
An engine builds its arguments from the settings alone and names no operating system, which lets a Windows pipeline be rendered and tested on Linux and makes the displayed command the one the publish button starts.

## Which protocol carries which format

A protocol carries a bitstream format, so each transport declares its own carriage and both directions read it.
The publish entry validates a publish command, and the watch entry answers what a viewer may receive over that leg.

| Transport | ffmpeg publish | GStreamer publish | Player watch | Tile watch | Browser watch |
|---|---|---|---|---|---|
| SRT | h264, hevc | h264, hevc | h264, hevc | h264, hevc | none |
| RTSP | all five | all five | all five | all five | none |
| RTMP | h264, hevc, av1, vp9 | none | h264 | h264 | none |
| WebRTC | h264 | h264, vp9, vp8 | none | h264, vp9, vp8 | h264, vp9, vp8 |
| HLS | none | none | h264, hevc, av1, vp9 | h264, hevc, av1, vp9 | h264, hevc, av1, vp9 |
| MoQ | none | none | none | none | all five |

Why each row is shaped that way:

- **SRT.** MPEG-TS registers a stream type for the two H.26x formats and for none of the others, and both engines write and read the same stream types.
- **RTSP.** RTP has a payload format for every video format and both audio codecs here, and both engines implement all of them, which is why RTSP is the fallback the other refusals point at.
- **RTMP.** One muxer writes the enhanced tags the relay ingests where the other writes the legacy ones alone, so there is no GStreamer publish form, and both watch cells sit on legacy-tag parsers.
- **WebRTC.** One muxer writes a single H.264 track and has no payloader for anything else, where the other payloads whatever the session negotiates.
  Playback is an exchange rather than an address, so no player opens it.
- **HLS.** The relay segments and serves HLS and ingests nothing over it, so the watch cells are the segment formats its muxer cuts.
  A tile opens the video rendition, which is why that leg carries no audio.
- **MoQ.** The relay packages every ingested format into tracks and ingests nothing over it, so it is the widest watch leg and a browser's alone: no other reader here has anything to open it with.

The browser column is what the relay's page serves rather than what a browser decodes.
Which formats a given build has a decoder for is that browser's fact and no table here can hold it, so a narrower entry would refuse a page that would have played.

The engine axis exists because a single list per leg would state the narrower of the two: the engine carrying more would be refused a format it serializes correctly, with no reason any form could give.

Audio is carried per protocol: WebRTC carries Opus alone, RTMP AAC alone, and the rest carry both.
HLS on a tile carries none.

Two formats the relay would negotiate over WebRTC are missing all the same.
It refuses H.265 there for any stream carrying B-frames, a property of the encode unknowable for a stream this app did not produce.
An AV1 track negotiates and then yields no picture, which takes it off the publish cells too: a leg nothing can read back is not a leg.

Rules falling out of the table:

- A codec no transport publishes cannot be published at all, and the refusal names the transports that would have carried it on the running engine.
- What a viewer may receive over is the watch entry for its engine, so the choice is narrowed per stream and per engine.
- A publish leg the two engines carry differently is the capture backend's business as much as the transport's, the backend fixing the engine.

Which rate-control modes a row offers is not uniform, and a gap naming the mode carries it.
Lossless is the mode that goes missing: three encoders code bit-exact, one does so through one engine and not the other, and no AV1, VP8, VAAPI, AMF or Vulkan encoder does at all.

## The backend and family pair

Whether captured frames reach the encoder without a trip through system memory depends on capture backend and encoder family together, and on neither alone.
The portal capture shares device memory with a VAAPI encoder and not with a software one, and that VAAPI encoder shares none with an X11 grab.
A gap cannot express that, being a fact about one codec, so the pairs are their own table and the catalog carries it whole.
Each engine holds its half beside its builder, and a row whose engine half is missing is asserted rather than filled in.

## Rules in the backend, words in the shell

A shell shows what the backend describes, so greyings, predictions and option lists sit beside the tables they derive from.
Every label, tooltip and refusal sentence is written where the layout is, keyed by the identifiers this model already uses.

Per engine and control, a rule states where a builder departs from the mode table, dropping a knob the mode uses or forwarding one it marks unused, and carries a code rather than a sentence.

## A dropped knob and a gapped mode

A dropped knob and a mode the encoder cannot run are two facts, and the line between them is what the mode still is without the knob.
A rule withholds a knob the mode can do without: still that mode, with one field greyed.
Where the knob defines the mode, withholding it leaves the other mode under the first one's name, so the table gaps the mode instead.
Constrained VBR is the case: without a ceiling it is ABR, so encoders taking no ceiling gap the mode rather than greying the ceiling.

Constant quality is the other side of the same fact.
A quality target spends what the picture costs, so the ceiling is the one control that bounds it, and the encode is still constant quality without one.
That makes it a greyed field rather than a mode gap.

## What derives from the tables

| Consumer | Reads them for |
| --- | --- |
| availability | greying an option the rules refuse, and greying a rate-control field unless the mode uses it, the codec's encoder has it and the engine forwards it |
| repair | walking an illegal combination to the first legal value, leaving it standing where the walk finds none |
| the estimate | the pre-publish bitrate prediction, from coding efficiency and chroma weight |
| the option lists | so a control cannot offer a value the tables do not define |
| the viewability verdict | what the carriage gives a receiving pipeline over the selected tile watch leg |
| the preset search | the configuration a preset applies here (`presets.md`) |

Greying and repair read one source, so a greyed option and its replacement always agree.
That holds only while every value the repair can pick satisfies the rules the availability pass greys by, so a dimension with nothing legal left keeps the value it has rather than taking one from outside them.
The field then carries its reason and the publish refuses with it, one answer from both sides.

## Adding a codec, chroma or mode

Add the row.
Dropdowns, constraints, estimate and verdict follow with no further edits.
Where the engines disagree, the row states the wider fact and carries a gap for the engine that lacks it.
Narrowing the row instead takes the capability away from the engine that has it, with no reason shown anywhere.
Where one driver miscodes what the encoder does implement, the row carries a driver defect instead, so the value stays offered on every driver that runs it.

The shell owes the new value a name and nothing else, and a value it has no name for renders as the raw identifier: honest, visible, and still a defect.
Nothing can fall through a shell's runtime default, no shell holding a table to fall through.

## What stays imperative

Process supervision, event subscriptions, relay polling and the one-time encoder probe stay in the process layer.
The model governs domain rules, and a table describes what is true.
