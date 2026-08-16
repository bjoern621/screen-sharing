# Domain model as tables of facts

Codec, pixel format and rate-control mode are not free-form strings scattered through the code.
Each is a table of facts, and every rule is derived from those tables rather than restated.
One definition governs the encoder, the settings form, the bitrate estimate and the viewability verdict.

## The problem it removes

A codec carries many rules: which family it runs on, which pixel formats it encodes, which transport carries it, how efficient it is, which viewer decodes it.
Written imperatively they spread across files and drift.
The failure mode is two encodings of one constraint: the form greys an option while the normalizer still lets the value through.

## Where each fact lives

| Table | Holds, per row |
| --- | --- |
| `capabilities/capabilities.go` | per codec: encoder family, the pixel formats it may encode, what its encoder cannot do, and the scales its quantizer and bitrate targets count on, all per publish engine |
| `capabilities/audio.go` | per audio codec: the element each engine codes it with, the sample rate the branch resamples to, the bitrate |
| `platform/audio.go` | per second-track source: the operating systems whose sessions serve it, what serves it on each, what one that does not is missing |
| `gpupath/gpupath.go` | per capture backend and encoder family: whether frames reach the encoder without a trip through system memory, and what carries them |
| `capabilities/decoders.go` | per decoder element: the pixel formats it decodes and what bounds that list |
| `transport/*.go` | per protocol, leg and engine: the video formats and audio codecs that engine puts through that leg, declared beside the code serializing it |

`capabilities/decoders.go` describes the viewers, not this machine.
A stream is published once and watched on whatever hardware the watchers have, so nothing in it is probed and nothing restricts a choice.
The form reads it to say what a pixel format costs a viewer: every format has a software decoder, so the choice is a viewer's GPU against a viewer's cores.

The two engines wrap different encoder implementations, so a pixel format, colour range, rate-control mode or whole codec can be one engine's and not the other's.
Each difference is a `Gap` naming engine, option, value and reason, rather than a row narrowed to what both manage.
An option one engine reaches stays offered on that engine's capture backends and greys with the element's own limit on the other, so the form says "no GStreamer encoder element takes planar-RGB input" instead of hiding the format from everyone.

A gap takes one value of one option away, and `capabilities.Options` is which options exist.
Lookup, validator and frontend read that list rather than a field per axis, so an option becomes gappable by being named there, given a refusal phrase, and carried in `settings.Publish.CapabilityOptions`.
A gap naming no option takes the codec off that engine entirely, no value reaching an encoder that is not there.
Gap values are the settings' own: the option is a settings field name and the value one that field takes, so a gap and the control it greys are one identifier on both sides of the wire.

### One evaluator, and what a gap is now

A `Gap` and a numeric ceiling are how a codec's limits are *written*, on the row they belong to.
They are not what anything *reads*.
`capabilities/rules.go` turns each into a rule in `backend/internal/rules`, and every consumer asks that evaluator: the greying, the ends a numeric control is offered between, the repair, and `capabilities.Validate`'s refusal.

A gap can only name a codec, an engine, an option and a value.
A fact about a capture backend and a codec together, or a codec and the platform, has no row to sit on.
Each grew a table with a consumer written against it, and every one of those restated part of an answer the gap mechanism already knew how to give.
The ceilings show the cost.
`CqMax` and `BitrateLimitM` were columns the form narrowed a control by and the validator refused a value by, making the range a slider offers and the range a publish accepts two answers derived from one fact, free to gate on different things.

A rule is a row:

- the facts it binds under, keyed by axis
- the verdict, stated and not derived
- the control it lands on
- which of that control's values it takes, as a value set or a numeric band
- the fact behind it, as a code

One axis makes it broad, five make it surgical, so "no VP8 encoder has a colour range field" and "this codec on this capture backend on this engine alone" are one shape.
An axis is declared once, with what it reads as and the argument a statement carries it under.
A reason is assembled from the axes it matched, so a row states which fact it is and never which words carry it.

Rules are declared where the fact lives and registered into the one evaluator, which keeps `transport` declaring its carriage beside the code that serializes it.
Only verdicts cross the wire.
Rules and axis vocabulary stay in Go (`ipc-api.md`).

### The two ladders

How hard an encoder works and what it works towards are two settings, `publish.effort` and `publish.tune`, and each codec's row declares its own ladder for both (`capabilities/ladders.go`).

Steps are the encoder's own identifiers: x264 counts in names, SVT-AV1 in numbers to 13, NVENC p1 to p7.
A scale normalized across codecs was rejected: a number carried across a codec change lands on a different real setting than the one held.
So a step off the selected codec's ladder is reset to the one that codec's row declares for the mode, never mapped by position, and the field is named in the repaired list so the change is readable.

Two fields rather than one: a live encode drops the lookahead and frame reordering a quality encode keeps, whatever effort it spends.

A row states where each mode starts on its ladder, and which modes pin the step instead of starting on it.
A pin is a fact about the encoder, not the mode: NVENC fixes its preset in CBR because a low-latency preset is what lets it hold a constant rate, and x264 in that mode takes whatever step it is given.
The form greys a pinned control and names the step in force, and both builders spend the declared step, so the sentence cannot name one the encode is not running.

Both builders read the row through `Codec.ResolveSteps`, keeping a stream's look off the capture backend that produced it: one library through two bindings encodes alike.
Each engine still owns the spelling, and the spellings differ more than the option name does.
ffmpeg says `-preset` where the x264 element says `speed-preset`.
That element splits x264's tune list across `tune` and `psy-tune`.
The x265 element's tune enum starts at `ssim` rather than at no tuning, so the untuned step is stated there by number.
The nvcodec elements spell the NVENC tunes in full words where the row uses the SDK's abbreviations.
The empty step is the one answer every builder shares: pass no such option at all.
A step the row does not declare is refused rather than forwarded, so a settings file that never went through the form fails naming the control rather than inside an encoder's error path.

A row declaring no ladder is an encoder with no such knob, which greys the control naming that codec.
Either ladder can be absent alone: the Vulkan rows tune and take no effort step, so each control asks about its own.

What a tune step is differs by vendor, and the row states the vendor's own vocabulary rather than a shared one.
x264 and x265 name what the picture is or what the decoder needs, NVIDIA and Vulkan the delay to hold, the AV1 and VP encoders a score to maximise or the judgement weighing what the eye sees, and QSV the scenario the session is for.
Where one engine's wrapper has no such knob the steps are gapped there rather than dropped from the row, so the option stays on the engine that reaches it: libaom takes a tune on ffmpeg where `av1enc` exposes none, and oneVPL's scenario reaches Intel's runtime through ffmpeg's `-scenario` and no qsv element property.

A knob one engine spends and the other does not is a departure rather than a gap, the control being the same control on both.
VAAPI is the case: its rows declare the seven target usages the `va` elements take, and ffmpeg's `-quality` counts over the range the installed driver reports, measured 0..32 on Mesa's radeonsi against oneVPL's 1..7 on Intel's.
One step carried across would spend a different amount of work per engine and per card, so the ffmpeg builder spends none and the form greys the control there with that reason (`form.availabilityEngineRules`).

Two rows declare no ladder at all, each for its own reason.
No VA profile carries a tuning hint, so the VAAPI rows tune for nothing.
AMF declares effort and no tune though the API has a usage hint, because this app pins it: a low-latency usage drops the IDR period and leaves a late subscriber no recovery point.
VideoToolbox declares neither, the framework taking no preset and no tuning hint, and what stands in for both is the realtime flag every mapping sets.

Audio is two settings against two tables, source and codec answering different questions.
Which sources exist is the platform's answer, the `Audio` field, a row of `platform.AudioSources`.
Which codec the track is coded in is the engine's and the leg's, `AudioCodec`, a row of `capabilities.AudioCodecs`.
That row states the element each engine codes it with, the sample rate and the bitrate, so both engines build their branch from one declaration instead of a hardcoded element list each.

### The second-track capture sources

A row of `platform/audio.go` names the settings value, the operating systems whose sessions serve it, what serves it on each, and what one that does not is missing.

It differs per platform because the engines do, so it is not a list of strings.
On a PulseAudio or PipeWire session both open desktop audio as the monitor of the default sink, ffmpeg `-f pulse -i` and GStreamer `pulsesrc device=`.
The name they open it by is `platform.AudioMonitorDevice`, one constant for both: two spellings of one server's name are two things able to disagree about which device a stream records.
On Windows the GStreamer engine opens the default render device's loopback through `wasapi2src`, which takes no handle for it, and ffmpeg has no WASAPI input at all.
macOS serves nothing, reading what it plays needing a CoreAudio process tap or ScreenCaptureKit audio that neither engine has an element for.
Each refusal names what that machine is missing rather than saying "Linux only": a user who reads why macOS cannot do it cannot act on why Windows cannot.

Which engine opens a source is a second question and is answered where the capture backends are (`publish.AudioAvailable`), a backend fixing the engine.
Both places a source is one engine's are on that table: a program's own output is a PipeWire node only `pipewiresrc` opens, and Windows audio is WASAPI only `wasapi2src` reads.
The element each kind opens on each platform is `publish.gstAudioElements`, keyed by the platform because the elements are the platform's and no element spans both.

The lookup is a table read: the same `platform.Info` yields the same ordered list every call, so a form may resolve on every keystroke without paying for it.

**A kind is declared and what is inside it is enumerated**, which is why they are two controls and two answers.
Whether a machine serves desktop audio at all is this table's.
Which outputs a machine plays into no table can hold, so `backend/internal/audiodev` reads them off the sound server, once, cached for the process lifetime and read back separately from the call that takes it: the division the encoder probe makes, for the same reason.
A kind with nothing enumerated still has one thing in it: its own default, which is what an entry naming no device takes.
A selection the enumeration stops reporting stays on the list with a note rather than being dropped, the way a monitor index does: an application not running now may be running when the stream starts.

**The second track is a list.**
A screen share is normally several sources at once, so `settings.Publish.AudioSources` is a repeated `{source, device, gain, mute}` and the entries mix into one track.
One track and not several is carriage, not preference: RTMP carries one audio track and the relay re-serves every ingest on all listeners, so a two-track stream would be unplayable on the narrowest leg while the form said it published.

Each entry is addressed by an indexed key (`publish.audio_sources[2].gain`) so every control kind the form has edits a list item and a statement lands on one entry rather than on the control.
The list grows through the settings, not through an effect: the form draws one row past the end, picking a kind on it is the write that adds the entry, and setting a kind back to the absent one takes an entry off on the next repair.
Ordinary writes through ordinary controls, which keeps a shell from deciding anything about the list's shape.

Gain carries presence, zero being a level and not an absence: a source turned all the way down is silent, and an entry nobody set a level on is at unity.
Without presence the two would be one value and the entry a reader creates would arrive silent.
Mute is a level too, the mixer multiplying by zero, which keeps unmuting a write to a running pipeline rather than a rebuild of the audio graph, and makes a list of nothing but muted sources still carry a track.

Every consumer reads those rows.
The catalog carries what this machine serves.
The form offers every declared source, greys the ones the machine does not serve with the row's own sentence (`field-availability.md`), and notes what serves each of the others here.
The repair walks a stranded entry onto the first source those rows leave standing, which for a machine serving none is the absent one, and an entry left there is one the repair takes off the list.
`settings.AudioSource` spells the absent source by reading the table's constant rather than typing `"none"` again.
So the list a form offers and the list a machine serves are two projections of one table.

The engines read them through one derivation rather than each with a table of its own.
An engine builds its arguments from the settings alone and names no operating system, which lets a Windows pipeline be rendered and tested on Linux and makes the displayed command the one the publish button starts.
Which OS a capture backend runs on is `publish.captureNeeds`' column, so `publish.AudioAvailable` is the only place a backend is turned into a platform and the source table asked.
A refused publish carries the same sentence the greyed option does.

Which protocol carries a codec is not a column here.
A protocol carries a bitstream format, so each transport declares its own carriage (`transport.Formats`) and both directions read it: the publish entry validates a publish command, the watch entry answers what a viewer may receive over that leg (`viewer-architecture.md`).
A carriage is per leg and per engine and names video formats and audio codecs together, what a listener carries as a bitstream and as a second track being one fact about that listener.
The engine axis exists because a single list per leg would state the narrower of the two: the engine carrying more would be refused a format it serializes correctly, with no reason any form could give.
WebRTC is where they part, publishing H.264 on ffmpeg and H.264, VP8 and VP9 on GStreamer.
`transport.Register` holds a stated carriage and its serialization capability to each other, so an entry can neither offer a leg it cannot build nor build one no caller may reach.
Adding a transport is one file in `transport` and no edit to the codec table.

The encoder reads this table directly.
Each builder keys family-wide behaviour off a table the row's `Family` indexes (`familyMappings`, `hwSurfaces`, `gstFamilyLimits`, `gstFamilyChromaFormats`) rather than off a per-family flag or a codec-name suffix, so a family gains a behaviour by gaining an entry.
`capabilities.Validate` rejects a codec, option value or quantizer the table forbids, reading every option in `Options` the same way.
Both engines call it, naming themselves, so neither path accepts what the other rejects and a gap belonging to one engine binds only there.
The second track is validated twice beside it: `capabilities.ValidateAudio` for a codec the engine cannot code, `transport.ValidatePublishAudio` for one the leg does not carry.
Two refusals because the fix differs: another capture backend for the first, another codec or leg for the second.
The same table reaches the shell in the `Catalog`, so what the encoder rejects is what the form greys.
The decode table travels beside it and feeds a note on the pixel-format control rather than a greying, a decoder the viewer lacks being a cost and not an illegal combination.

Which engine runs a capture backend is a publish-layer fact and the catalog carries it too.
It is a settings input because the two engines express the same five rate-control modes through different properties, so a knob one forwards the other may drop.

`gpupath/gpupath.go` is a table of pairs rather than of codecs, the one constraint neither end declares alone.
Whether captured frames reach the encoder without a trip through system memory depends on backend and family together: the portal capture shares device memory with a VAAPI encoder and not with an x264 one, and a VAAPI encoder shares it with the portal capture and not with ximagesrc.
A `Gap` cannot express that, being a fact about one codec, so the pairs are their own table and the catalog carries it whole.
Each engine holds its half beside its builder (`gstGpuMemories`, `gpuConverts`), and a row whose engine half is missing is asserted rather than filled in, the alternative being a memory the elements do not carry (`capture-architecture.md`, "Frame memory").

Heuristics live in Go, and the words do not live here at all.
`ipc-api.md` settles that: a shell shows what the backend describes, so greyings, predictions and option lists sit beside the tables they derive from, and every label, tooltip and refusal sentence is written where the layout is, keyed by the identifiers this model already uses.

The engine rules are the same split.
Per engine and control, a rule states where a builder departs from the mode table, dropping a knob the mode uses or forwarding one it marks unused.
Each mirrors a branch of `encoderArgs` or `gstEncoder` and carries a code rather than a sentence.

A dropped knob and a mode the encoder cannot run are two facts, and the line between them is what the mode still is without the knob.
A rule here withholds a knob the mode can do without: still that mode, with one field greyed.
Where the knob defines the mode, withholding it leaves the other mode under the first one's name, so the capability table gaps the mode instead and the form offers the mode that describes the encode.
Constrained VBR is the case: without a ceiling it is ABR, so encoders taking no ceiling gap `vbr` rather than greying `maxrateM`.

Constant quality is the other side of the same fact.
A quality target spends what the picture costs, so `maxrateM` is the one control that bounds it, and the encode is still constant quality without one.
That makes it a greyed field rather than a mode gap: `capabilities.QualityCeiling` states per codec and engine whether the element holds a quality target inside a rate buffer, and `publish.RateCeilingMbps` is the one answer to "what is this encode held to" that the plot's rule, the prediction and the encoder all read.

## What derives from the tables

| Consumer | Reads them for |
| --- | --- |
| `form/availability.go` | greying an option the rules refuse, and greying a rate-control field unless the mode uses it, the codec's encoder has it and the engine forwards it |
| `form/repair.go` | walking an illegal combination to the first legal value, leaving it standing where the walk finds none |
| `form/estimate.go` | the pre-publish bitrate prediction, from coding efficiency and chroma weight |
| `form/options.go` | the option lists, so a control cannot offer a value the tables do not define |
| the viewability verdict | what the transport table gives a receiving pipeline over the selected tile watch leg (`viewer-architecture.md`) |
| the preset search | the configuration a preset applies here, over the codecs declared and the capture backends the platform runs (`presets.md`) |

The facts a resolve is evaluated against are assembled in `form/facts.go`, the layer holding both the draft and the machine's answers.
The rules package holds neither, being what every domain package registers into.

Greying and repair read one source, so a greyed option and its replacement always agree.
That holds only while every value the repair can pick satisfies the rules the availability pass greys by, so a dimension with nothing legal left keeps the value it has rather than taking one from outside them.
The field then carries its reason and the publish refuses with it: one answer from both sides, rather than a form offering what the encoder rejects.

## Adding a codec, chroma or mode

Add the row.
Dropdowns, constraints, estimate and verdict follow with no further edits.
Where the engines disagree, the row states the wider fact and carries a `Gap` for the engine that lacks it.
Narrowing the row instead takes the capability away from the engine that has it, with no reason shown anywhere.
The shell owes the new value a name and nothing else, and a value it has no name for renders as the raw identifier: honest, visible, and still a defect (`ipc-api.md`).
Nothing can fall through a shell's runtime default, no shell holding a table to fall through.

## What stays imperative

The model governs domain rules, not effects.
Process supervision, event subscriptions, relay polling and the one-time encoder probe stay in the Go process layer.
A table describes what is true.
It does not run a child process or subscribe to a stream.
