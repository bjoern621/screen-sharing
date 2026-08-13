# Domain model as tables of facts

The codec, pixel format and rate-control mode are not free-form strings scattered through the code.
Each is a table of facts, and every rule the app enforces is derived from those tables rather than restated.
One definition governs the encoder, the settings form, the bitrate estimate and the two grid-viewability verdicts.

## The problem it removes

A codec carries many rules: whether it runs on NVENC, which pixel formats it can encode, which transport can carry it, how efficient it is, which viewer can decode it.
Written imperatively, those rules spread across files and drift.
The failure mode is two encodings of one constraint: the settings form greys out an option while the normalizer still lets the value through, or one copy is updated and the other is missed.

## Where each fact lives

Constraints the encoder and the UI must agree on live in Go and are the single source:

- `capabilities/capabilities.go`: per codec, the encoder family, the pixel formats it may encode, what its encoder cannot do, and the scales its quantizer target and bitrate target count on, both stated per publish engine.
- `capabilities/audio.go`: per audio codec, the element each publish engine codes it with, the sample rate the capture branch resamples to and the bitrate the track is coded at.
- `platform/audio.go`: per second-track capture source, the operating systems whose sessions serve it, what serves it on each, and what an operating system that does not is missing.
- `gpupath/gpupath.go`: per capture backend and encoder family, whether their frames reach the encoder without a trip through system memory, and what carries them.
- `capabilities/decoders.go`: per decoder element, the pixel formats it decodes and what bounds that list.
  It describes the viewers rather than this machine: a stream is published once and watched on whatever hardware the watchers have, so nothing in it is probed and nothing in it restricts a choice.
  The form reads it to say what a pixel format costs a viewer, every format having a software decoder and the choice being between a viewer's GPU and a viewer's cores.
- `transport/*.go`: per protocol, leg and engine, the video formats and audio codecs that engine puts through that leg, declared beside the code that serializes it.

The two publish engines wrap different encoder implementations, so a pixel format, a colour range, a rate-control mode or a whole codec can be one engine's and not the other's.
Each difference is a `Gap` naming the engine, the option, the value and the reason, rather than a row narrowed to what both engines manage.
An option one engine reaches therefore stays offered on that engine's capture backends and is greyed with the element's own limit on the other, so the form can say "no GStreamer encoder element takes planar-RGB input" instead of hiding the format from everyone.

A gap takes one value of one settings option away, and which options exist is the `capabilities.Options` list.
The lookup, the validator and the frontend read that list rather than a field per axis, so an option becomes gappable by being named there, given a refusal phrase, and carried in `settings.Publish.CapabilityOptions`.
A gap naming no option takes the codec off that engine altogether, since no value of any option reaches an encoder that is not there.
Gap values are the settings' own: the option is a settings field name and the value is one that field takes, so a gap and the form control it greys are the same identifier on both sides of the wire.

### One evaluator, and what a gap is now

A `Gap` and a numeric ceiling are how a codec's limits are *written*, on the row they belong to.
They are not what anything *reads*.
`capabilities/rules.go` turns each of them into a rule in `internal/rules`, and every consumer asks that evaluator: the form's greying, the ends a numeric control is offered between, the repair, and the refusal `capabilities.Validate` returns.

The move exists because a gap can only name a codec, an engine, an option and a value.
A fact about a capture backend and a codec together, or a codec and the platform, has no row to sit on, so each one grew a table of its own with a consumer written against it, and every one of those consumers restated part of an answer the gap mechanism already knew how to give.
The ceilings show what that costs: `CqMax` and `BitrateLimitM` were columns the form narrowed a control by and the validator refused a value by, which made the range a slider offered and the range a publish accepted two answers derived from one fact, free to gate on different things. They did: the validator applied the bitrate ceiling only in the modes that send a target, and the form applied it in all five.

A rule is a row: the facts it binds under, keyed by axis; the verdict, which is stated and not derived; the control it lands on; which of that control's values it takes, as a value set or a numeric band; and the fact behind it as a code.
Naming one axis makes it broad and naming five makes it surgical, so "no VP8 encoder has a colour range field" and "this codec on this capture backend on this engine alone" are the same shape.
An axis is declared once, with what it reads as and the argument a statement carries it under, and a rule's reason is assembled from the axes it matched on, so a row states which fact it is and never which words carry it.

Rules are declared where the fact lives and registered into the one evaluator, which is what keeps `transport` declaring its carriage beside the code that serializes it.
Only verdicts cross the wire; the rules and the axis vocabulary stay in Go, for the reason `ipc-api.md` gives.

### The two ladders

How hard an encoder works and what it works towards are two settings, `publish.effort` and `publish.tune`, and each codec's row declares its own ladder for both (`capabilities/ladders.go`).

The steps are the encoder's own identifiers: x264 counts in names, SVT-AV1 in numbers to 13, NVENC from p1 to p7.
A scale normalized across codecs was rejected because a number carried across a codec change would land on a different real setting than the one that was held, so a step off the selected codec's ladder is reset to the one that codec's row declares for the mode, never mapped by position, and the field is named in the repaired list so the change is readable.

Two fields rather than one, because a live encode drops the lookahead and the frame reordering that a quality encode keeps, whatever effort it is spending.

A row states where each rate-control mode starts on its ladder, and which modes pin the step instead of starting on it.
A pin is a fact about the encoder rather than about the mode: NVENC fixes its preset in CBR because a low-latency preset is what lets it hold a constant rate, and x264 in the same mode takes whatever step it is given.
The form greys a pinned control and names the step in force, and both builders spend the same declared step, so the sentence cannot name one the encode is not running.

Both builders read the row through `Codec.ResolveSteps`, which is what keeps a stream's look off the capture backend that produced it: one library reached through two bindings encodes alike.
What each engine still owns is the spelling, and the spellings differ more than the option name does.
ffmpeg says `-preset` where the x264 element says `speed-preset`; that element splits x264's tune list across `tune` and `psy-tune`; the x265 element's tune enum starts at `ssim` rather than at no tuning, so the untuned step has to be stated there by number; and the nvcodec elements spell the NVENC tunes in full words where the row uses the SDK's abbreviations.
The empty step is the one answer every builder shares: it passes no such option at all.
A step the row does not declare is refused rather than forwarded, so a settings file that never went through the form fails naming the control rather than inside an encoder's own error path.

A row that declares no ladder is an encoder with no such knob, which greys the control naming that codec.
Either ladder can be absent on its own: the Vulkan rows tune and take no effort step, the libvpx ones take a step and tune for nothing, so each control asks about its own ladder.
The QSV and AMF rows declare an effort ladder and no tune: oneVPL counts target usages from the quality end to the speed one and AMD names three quality presets, and neither family exposes a tuning knob for a row to declare.

Audio is two settings against two tables, because the source and the codec answer different questions.
Which sources exist is the platform's answer and is the `Audio` field, a row of `platform.AudioSources`.
Which codec the track is coded in is the engine's and the publish leg's, and is `AudioCodec`, a row of `capabilities.AudioCodecs`.
A row states the element each engine codes it with, the sample rate and the bitrate, so both engines build their branch from one declaration instead of a hardcoded element list each.

### The second-track capture sources

`platform/audio.go` is the source table, and it is a table for the reason every other one here is: the same fact was being restated by whoever asked.
The list lived in the frontend as the keys of `AUDIO_META` and the refusals as `AUDIO_SOURCE_NEEDS` beside them, so what a machine could capture was written once in Go's publish engines and twice more in TypeScript.
A row names the settings value, the operating systems whose sessions serve it, what serves it on each, and what an operating system that does not serve it is missing.

It differs per platform because the engines do, and that is the whole reason it is not a list of strings.
Both open desktop audio as the monitor of the default sink - ffmpeg through `-f pulse -i`, GStreamer through `pulsesrc device=` - and neither has anything to open where no PulseAudio or PipeWire server runs.
The name they open it by is `platform.AudioMonitorDevice`, one constant for both, because the engines differ in how they pass the handle and not in what they pass; it was spelled once per engine, and two spellings of one server's name are two things able to disagree about which device a stream records.
So Linux serves that source and the other two are refused it, each with what it is missing rather than both with "Linux only": a user who reads why Windows cannot do it cannot act on why macOS cannot.
A Windows WASAPI loopback or a macOS aggregate device would be another platform on the row and another sentence naming what serves it there; neither engine has one, and a row that claimed otherwise would grey nothing and fail at launch.

The lookup is a table read and nothing else: the same `platform.Info` yields the same ordered list on every call, so a form may resolve on every keystroke without paying for it.

**A kind is declared and what is inside it is enumerated**, which is why they are two controls and two answers.
Whether a machine serves desktop audio at all is this table's; which microphone is plugged in is not something any table can hold, so `internal/audiodev` reads it off the sound server, once, cached for the process lifetime and read back separately from the call that takes it - the same division the encoder probe makes and for the same reason.
A kind with nothing enumerated still has one thing in it: its own default, which is what an entry naming no device takes.
A selection the enumeration stops reporting stays on the list with a note rather than being dropped, the way a monitor index no enumeration reported does, because an application that is not running now is one that may be running when the stream starts.

**The second track is a list.**
A screen share is normally several sources at once - what the machine is playing, and whoever is talking over it - so `settings.Publish.AudioSources` is a repeated `{source, device, gain, mute}` and the entries mix into one track.
One track and not several is carriage rather than preference: RTMP carries one audio track and the relay re-serves every ingest on all of its listeners, so a two-track stream would be unplayable on the narrowest leg while the form said it published.

Each entry is addressed by an indexed key - `publish.audio_sources[2].gain` - so every control kind the form already has edits a list item and a statement lands on one entry rather than on the control.
The list grows through the settings and not through an effect on the contract: the form draws one row past the end of it, picking a kind on that row is the write that adds the entry, and setting a kind back to the absent one is what takes an entry off on the next repair.
Both are ordinary writes through ordinary controls, which is what keeps a shell from deciding anything about the list's shape.

The gain carries presence on the contract, because zero is a level and not an absence: a source turned all the way down is silent, and an entry nobody has set a level on is at unity.
Without presence the two would be one value and the entry a reader creates by picking a kind would arrive silent.
Mute is a level too - the mixer multiplies by zero - which is what keeps unmuting a write to a running pipeline rather than a rebuild of the audio graph, and what makes a list of nothing but muted sources still carry a track.

Every consumer reads those rows.
The catalog carries what this machine serves.
The form offers every declared source, greys the ones the machine does not serve with the row's own sentence (`field-availability.md`), and notes beside each of the others what serves it here.
The repair walks a stranded entry onto the first source the same rows leave standing, which for a machine that serves none of them is the absent one, and an entry left on that is an entry the repair then takes off the list.
`settings.AudioSource` spells the absent source by reading the table's constant rather than typing `"none"` a second time.
The list a form offers and the list a machine serves are therefore two projections of one table rather than two lists that agree until one is edited.

The publish engines read them too, and through one derivation rather than each with a table of its own.
An engine builds its arguments from the settings alone, so it names no operating system - that is what lets a Windows pipeline be rendered and tested on a Linux machine, and what makes the displayed command the one the publish button starts.
Which operating system a capture backend runs on is `publish.captureNeeds`' column, so `publish.AudioAvailable` is where the backend is turned into a platform and the source table asked, and it is the only place that conversion is made.
Before it, each engine refused desktop audio per capture backend with a sentence of its own, so one source's absence was written four times in Go and the greying a form showed came from a fifth.
A refused publish now carries the same sentence the greyed option does.

Which protocol carries a codec is not a column here.
A protocol carries a bitstream format, so each transport declares its own carriage (`transport.Formats`) and both directions read it: the publish entry validates a publish command, the watch entry answers what a viewer may receive over that leg (`viewer-architecture.md`, "Which protocol carries which format").
A carriage is per leg and per engine, and it names video formats and audio codecs together, since what a listener carries as a bitstream and what it carries as a second track are one fact about that listener.
The engine axis exists because a single list per leg would have to state the narrower of the two engines: the engine that carries more would be refused a format it serializes correctly, and no form could give a reason for the refusal.
WebRTC is where the two part, publishing H.264 on the ffmpeg engine and H.264, VP8 and VP9 on the GStreamer one.
`transport.Register` holds a stated carriage and the matching serialization capability to each other, so a transport can neither offer a leg it has no code to build nor build one no caller may reach.
Adding a transport is therefore one file in the `transport` package and no edit to the codec table.

The encoder reads this table directly.
Each builder keys its family-wide behaviour off a table the row's `Family` indexes (`familyMappings`, `hwSurfaces`, `gstFamilyLimits`, `gstFamilyChromaFormats`) rather than off a per-family flag or a codec-name suffix, so a family gains a behaviour by gaining an entry.
`capabilities.Validate` rejects a codec, option value or quantizer the table forbids, reading every option in `Options` the same way.
Both publish engines call that validator, naming themselves, so neither path accepts what the other rejects and a gap that belongs to one engine binds only there.
The second track is validated twice beside it, by `capabilities.ValidateAudio` for a codec the engine has no encoder for and by `transport.ValidatePublishAudio` for one the publish leg does not carry.
Two refusals rather than one because the fix differs: another capture backend for the first, another audio codec or another leg for the second.
The same table reaches the shell in the `Catalog`, so a combination the encoder would reject is the same combination the form greys out.
The decode table travels beside it and feeds a note on the pixel-format control rather than a greying, since a decoder the viewer lacks is a cost and not an illegal combination.

Which publish engine runs a capture backend is a fact of the publish layer, and the catalog carries it too.
It is a settings input because the two engines express the same five rate-control modes through different properties, so a knob one forwards the other may drop.

`gpupath/gpupath.go` is a table of pairs rather than of codecs, and it is the one constraint neither end declares alone.
Whether captured frames reach the encoder without a trip through system memory depends on the capture backend and the encoder family together: the portal capture shares device memory with a VAAPI encoder and not with an x264 one, and a VAAPI encoder shares it with the portal capture and not with ximagesrc.
A `Gap` cannot express that, since a gap is a fact about one codec, so the pairs are their own table and the catalog carries it whole.
Each engine holds its own half beside its builder (`gstGpuMemories`, `gpuConverts`), and a row whose engine half is missing is asserted rather than filled in, because the alternative is negotiating a memory the elements do not carry (`capture-architecture.md`, "Frame memory").

Heuristics live in Go, and the words do not live here at all.

That is what `docs/ipc-api.md` settles: a shell shows what the backend describes and decides nothing, so the greyings, the predictions and the option lists sit beside the tables they derive from, and every label, tooltip and refusal sentence is written where the layout is, keyed by the identifiers this model already uses.
The split used to run the other way - `frontend/src/util/domain.ts` held the labels, the coding efficiencies, the mode metadata and the engine rules in TypeScript, derived from tables that live in Go and crossed the wire as raw rows.
That was defensible with one frontend and became three copies of one rule with three shells, which is the drift this page exists to prevent, so the shells that held copies were deleted and what they knew moved here.

The engine rules were part of that move.
Per engine and control, a rule states where a builder departs from the mode table, either dropping a knob the mode uses or forwarding one it marks unused; each mirrors a branch of `encoderArgs` or `gstEncoder`, and each carries a code the form sends rather than the sentence a shell writes for it.

A dropped knob and a mode the encoder cannot run are two different facts, and the line between them is what the mode still is without the knob.
A rule here withholds a knob the mode can do without: the encode is still that mode, with one field greyed.
Where the knob is what defines the mode, withholding it leaves the other mode under the first one's name, so the capability table declares a `Gap` on the mode instead and the form offers the mode that describes the encode.
Constrained VBR is the case: without a ceiling it is ABR, so the encoders that take no ceiling gap `vbr` rather than greying `maxrateM`.

## What derives from the tables

Each consumer reads the tables instead of restating a rule:

- `form/availability.go`: greys out an option when the rules refuse it for the current settings, and greys a rate-control field unless the mode uses it, the codec's encoder has it and the capture's engine forwards it.
  The facts one resolve is evaluated against are assembled in `form/facts.go`, which is the layer holding both the draft and the machine's answers; the rules package holds neither, because it is what every domain package registers into.
- `form/repair.go`: repairs an illegal combination by walking the same tables to the first legal value, and leaves the value standing where the walk finds none.
- `form/estimate.go`: the pre-publish bitrate prediction, from coding efficiency and chroma weight.
- `form/options.go`: the option lists, built from the same tables so a control cannot offer a value the tables do not define.
- The viewability verdict: what the transport table gives a receiving GStreamer pipeline over the selected tile watch leg (`viewer-architecture.md`).
  There is one verdict where there were two, because the two grids that failed on different things are gone and a tile decodes whatever this machine's GStreamer decodes.
- The preset search: the configuration a preset applies here, searched over the codecs the capability table declares and the capture backends the platform runs, and greyed where the tables leave none (`presets.md`).

Because the greying and the repair read one source, a greyed option and its replacement always agree.
That holds only while every value the repair can pick satisfies the rules the availability pass greys by, so a dimension with nothing legal left keeps the value it has instead of taking one from outside them.
The field then carries its reason and the publish refuses with it, which is the same answer from both sides rather than a form that offers what the encoder rejects.

## Adding a codec, chroma or mode

Add the row to the table.
The dropdowns, constraints, estimate and verdict follow with no further edits.
Where the two engines disagree about the addition, the row states the wider fact and carries a `Gap` for the engine that lacks it; narrowing the row instead would take the capability away from the engine that has it, with no reason shown anywhere.
The shell owes the new value a name and nothing else, and a value it has no name for renders as the raw identifier - honest, visible, and still a defect, which is what gets it written (`ipc-api.md`).
Nothing about the addition can fall through a shell's runtime default, because no shell holds a table to fall through.

## What stays imperative

The model governs domain rules, not effects.
Process supervision, event subscriptions, relay polling and the one-time encoder probe are imperative by nature and stay in the hooks and the Go process layer.
A table describes what is true; it does not run a child process or subscribe to a stream.
