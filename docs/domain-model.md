# Declarative domain model

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
The lookup, the validator and the frontend read that list rather than a field per axis, so an option becomes gappable by being named there, given a refusal phrase, and carried in `settings.Stream.CapabilityOptions`.
A gap naming no option takes the codec off that engine altogether, since no value of any option reaches an encoder that is not there.
Gap values are the settings' own: the option is a `settings.Stream` JSON field name and the value is one that field takes, so a gap and the form control it greys are the same identifier on both sides of the wire.

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
Nothing enumerates the machine's audio devices, and if anything ever does, it is cached for the process lifetime and read back separately from the probing call, the way the encoder probe already divides them.

Every consumer reads those rows.
The catalog carries what this machine serves.
The form offers every declared source, greys the ones the machine does not serve with the row's own sentence (`field-availability.md`), and notes beside each of the others what serves it here.
The repair walks a stranded draft onto the first source the same rows leave standing, and `settings.Stream.Audio` spells the absent source by reading the table's constant rather than typing `"none"` a second time.
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
The same table reaches the frontend through the `App.Capabilities` binding, so a combination the encoder would reject is the same combination the UI greys out.
The decode table reaches it through `App.Decoders` beside it, and feeds a note on the pixel-format control rather than a greying, since a decoder the viewer lacks is a cost and not an illegal combination.

Which publish engine runs a capture backend is a fact of the publish layer, and `App.CaptureEngines` carries it to the frontend.
It is a settings input because the two engines express the same five rate-control modes through different properties, so a knob one forwards the other may drop.

`gpupath/gpupath.go` is a table of pairs rather than of codecs, and it is the one constraint neither end declares alone.
Whether captured frames reach the encoder without a trip through system memory depends on the capture backend and the encoder family together: the portal capture shares device memory with a VAAPI encoder and not with an x264 one, and a VAAPI encoder shares it with the portal capture and not with ximagesrc.
A `Gap` cannot express that, since a gap is a fact about one codec, so the pairs are their own table and `App.GpuPaths` carries it to the frontend whole.
Each engine holds its own half beside its builder (`gstGpuMemories`, `gpuConverts`), and a row whose engine half is missing is asserted rather than filled in, because the alternative is negotiating a memory the elements do not carry (`capture-architecture.md`, "Frame memory").

Presentation and heuristics live in the Wails frontend today, and are moving to Go.

That is the direction `docs/ipc-api.md` settles: a shell shows what the backend describes and decides nothing, so the labels, the greyings and the predictions below belong beside the tables they derive from rather than restated per shell.
With one frontend the split was defensible; with three it means one rule written three times in three languages, which is the drift this page exists to prevent.
Until the move lands, the tables below are the frontend's, and the `Form` message in `api/proto/screenshare/v1/form.proto` is where each of them arrives afterwards.

- `frontend/src/util/domain.ts`: per codec, chroma and mode, the label, tooltip, reference link, coding efficiency, raw bits-per-pixel, what a non-4:2:0 chroma asks of a decoder, and which controls each mode uses.
- `frontend/src/util/domain.ts` `ENGINE_RULES`: per engine and control, where a builder departs from the mode table, either dropping a knob the mode uses or forwarding one it marks unused.
  Each rule mirrors a branch of `encoderArgs` or `gstEncoder` and carries the sentence the form shows for it.

A dropped knob and a mode the encoder cannot run are two different facts, and the line between them is what the mode still is without the knob.
A rule here withholds a knob the mode can do without: the encode is still that mode, with one field greyed.
Where the knob is what defines the mode, withholding it leaves the other mode under the first one's name, so the capability table declares a `Gap` on the mode instead and the form offers the mode that describes the encode.
Constrained VBR is the case: without a ceiling it is ABR, so the encoders that take no ceiling gap `vbr` rather than greying `maxrateM`.

## What derives from the tables

Each consumer reads the tables instead of restating a rule:

- `deps.ts` `evaluateDeps`: greys out an option when the tables make it illegal for the current settings, and greys a rate-control field unless the mode uses it, the codec's encoder has it and the capture's engine forwards it.
- `deps.ts` `normalize`: repairs an illegal combination by walking the same tables to the first legal value, and leaves the value standing where the walk finds none.
- `estimate.ts`: the pre-publish bitrate prediction, from coding efficiency and chroma weight.
- `webgrid.ts`: the web-grid viewability verdict, from the codec's format, the chroma's 4:2:0 flag and the `WEB_GRID_DECODE` paths.
- `nativegrid.ts`: the native-grid viewability verdict, from what the transport table gives a receiving GStreamer pipeline over the grid's selected watch leg.
- `options.ts`: the dropdown lists, built from the meta tables so a control cannot offer a value the tables do not define.
- `presets.ts`: the configuration a preset applies here, searched over the codecs the capability table declares and the capture backends the platform runs, and greyed where the tables leave none (`presets.md`).

Because `evaluateDeps` and `normalize` read one source, a greyed option and its replacement always agree.
That holds only while every value `normalize` can pick satisfies the rules `evaluateDeps` greys by, so a dimension with nothing legal left keeps the value it has instead of taking one from outside them.
The field then carries its reason and the publish refuses with it, which is the same answer from both sides rather than a form that offers what the encoder rejects.

## Adding a codec, chroma or mode

Add the row to the table.
The dropdowns, constraints, estimate and verdict follow with no further edits.
Where the two engines disagree about the addition, the row states the wider fact and carries a `Gap` for the engine that lacks it; narrowing the row instead would take the capability away from the engine that has it, with no reason shown anywhere.
The `Codec`, `Chroma` and `Mode` union types force a new value into every meta table, so an incomplete addition fails to compile instead of falling through a runtime default.
A codec whose constraints also reach the encoder is added to the Go `capabilities` table, and the frontend receives it over the wire.

## What stays imperative

The model governs domain rules, not effects.
Process supervision, event subscriptions, relay polling and the one-time encoder probe are imperative by nature and stay in the hooks and the Go process layer.
A table describes what is true; it does not run a child process or subscribe to a stream.
