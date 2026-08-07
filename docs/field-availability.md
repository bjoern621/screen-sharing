# Field availability: hide vs disable

A settings field can become inapplicable to the current combination of capture backend, codec, publish transport and mode.
Two treatments express that, and the choice between them is a fixed rule, not a per-field judgement.
A third form covers the neighbouring case, a field that stays applicable but means something else.

**Every "reason" on this page is a fact, not a sentence.**
Which fact greys a control is decided in Go and crosses as a code with the identifiers it is about; the wording is the shell's, written where the column width and the tone are visible (`ipc-api.md`, `api/proto/screenshare/v1/text.proto`).
So the quoted sentences below are one shell's rendering of the rule beside them, and the rule is what this page is about: a reason names the limit, names which side has it, and where two facts block one field, names the one the user can act on.

## The treatments

**Hidden.**
The field is not rendered at all.
Conditional JSX in `StreamSettingsCard` gates it on the parent selection, for example the DRM download field rendered only when the capture backend is `kmsgrab`.

**Disabled with a reason.**
The field renders greyed, carrying a `disabledReason` tooltip that states why it is inert.
The reason comes from `deps.disabled` in `evaluateDeps` (`util/deps.ts`), for example the encoder preset ladder greyed under software x264 with "only the NVIDIA NVENC encoders take an encoder preset".

**One option disabled with a reason.**
A dropdown keeps the option and greys that entry, whose tooltip carries the reason from `deps.optionDisabled`.
This covers a value the current combination rules out while a neighbouring combination allows it: planar RGB is greyed on the portal capture backend, because no GStreamer encoder element takes it, and selectable on the capture backends that run ffmpeg, which codes it.
The reason names the limit and which side has it, so the greyed entry tells the user what to change rather than only that the option is gone.
The audio codec is greyed from two tables at once, and the reason names which one: the publish leg carries no such track (AAC under WebRTC, Opus under RTMP), or the capture backend's engine has no encoder for it.

**Live with a note.**
The field stays editable and its tooltip gains a sentence from `deps.note`.
This is for a combination where the value still reaches the encoder but does something the field's own text does not describe, such as the bitrate becoming a burst ceiling in constant-quality mode on NVENC.
A note is not a third treatment of inapplicability: it exists so a knob that a builder does forward is never greyed, which would leave the encoder using a number the form refused to show.

The preset selector greys an entry the same way, for the same kind of reason.
A preset is a promise no encoder or capture backend on this machine delivers, so the option carries what it needed and the search worked within (`presets.md`).

The pixel-format control carries a note of the second kind: what the choice costs a viewer to decode, from the decode table (`capabilities.Decoders`).
It is a note rather than a greying because it is not a limit on this machine at all.
Every format has a software decoder, so a pixel format no GPU takes is a viewer spending cores, which is a trade the publisher is entitled to make once it is stated.

## Three facts decide a rate-control field

A quantizer target, bitrate bound, rate buffer, B-frame count or preset is live only when all three agree.

- The **mode's concept** uses the knob: `MODE_META` in `util/domain.ts` says which controls each rate-control mode needs.
- The **codec's encoder** takes the knob: `FAMILY_META` flags the families whose encoders read the B-frame count and the preset (`takesBframes`, `takesPreset`), so both fields grey for a family that carries neither flag, whatever its hardware could do with them, and the reason lists the families that do from the same table.
- The capture backend's **publish engine** forwards the value: `ENGINE_RULES` records where a builder drops a knob the mode uses, so the preset ladder greys on the GStreamer engine, whose elements have no equivalent.

When two of them block the same field, the reason names the one the user can act on.
B-frames under software x264 in VBR read "only the NVIDIA NVENC encoders take a B-frame count", not the mode sentence that would be a lie there.

## Three facts decide the frame memory

The frame-memory control is the one field neither the capture backend nor the codec decides alone.
Its direct value needs both ends to share device memory, which is a pair rather than a property of either: the portal capture shares memory with a VAAPI encoder and not with an x264 one, and a VAAPI encoder shares it with the portal capture and not with ximagesrc.
`gpupath.Paths` declares the pairs, `App.GpuPaths` hands them to the form, and `unavailableFrameMemories` greys the direct value for a selection matching no row, naming both ends so either one is a way to reach it.

The third fact is who converts, and it splits the direct value in two.
A row whose device-side filter is told the colour and states it reaches `gpu`; a row where the platform has no such filter and the encoder converts the captured RGB itself reaches `gpu-encoder-color` instead, and `gpu` greys with what that costs.
So two of the four values can be greyed and each greying names the other as the way across: a pair with no row greys both direct values, an exact row greys the encoder-colour one as having nothing to trade, and a trading row greys `gpu` and names the capture backend that reaches both.
That last reason is the useful one, since the same screen is often reachable on the other engine where the conversion does state its colour.

Auto and the system copy are never greyed.
Auto answers with whichever path the pair has, and the system copy is the path every pair has, so a combination with no row leaves a working control rather than a dead one.
Auto also never answers with the encoder-colour path: it is the value nobody chose, so it may not change what the stream looks like.

Where the pair does have a row, the DRM download strategy is hidden's neighbour: it stays rendered under kmsgrab and greys, because a run that downloads nothing chooses no mapping device.
It is greyed rather than hidden because the field is already gated on the capture backend, and a second gate would make it appear and vanish while the user changes codecs.

## The rule

Ask what kind of control the field is.

- A **backend implementation knob** that has no meaning outside one backend is **hidden** when that backend is not selected.
  Its tooltip describes a mechanism that a user on any other backend has no reason to read.
  DRM download is a knob of the kmsgrab scanout path and nothing else.

- A **general encoding or quality concept** that the current combination happens to block stays **disabled with a reason**.
  The concept is part of the model every user is expected to understand, so the greyed field plus its reason teaches why the concept does not apply here.
  Encoder preset, quantizer target, bitrate bound, B-frames, color range and chroma are all general concepts, disabled when the codec, the mode or the capture backend's engine rules them out.

The test: would the tooltip teach a user on a different backend something worth knowing?
Yes means disable-with-reason.
No means hide.

## A live stream blocks no field

Every field stays editable while a stream is publishing, and what reaches the stream is asked for separately (`capture-architecture.md`, "Changing settings on a live stream").
The two controls a live stream does block are measurements rather than settings: the uplink speed test and the encode-capacity probe both run the real thing, so one would compete with the stream for the line and the other with the encoder for the silicon.
Neither is a value the user chose, which is why the reason sits on the button that takes the measurement instead of greying the figure beside it.

## Why the split exists

The settings form is pedagogical: dense tooltips explain the encoding model as the user configures it.
A greyed field with a reason participates in that teaching, so a user hunting for the NVENC preset under x264 reads why it is absent instead of finding a blank.
A hidden field removes noise that would teach nothing, so a backend implementation knob does not follow every user across backends it does not concern.

## Where the rules live

Disable reasons are derived, never hand-set per field.
`evaluateDeps` produces `deps.disabled` and `deps.note` from the capability table, the domain meta tables and the engine rules, the same source `normalize` repairs settings from, so a disabled option and its replacement cannot disagree.
Where a dimension has nothing legal left, `normalize` picks nothing and the field stays disabled with its reason, rather than holding a value the same evaluation greys.
See `domain-model.md` for the capability and meta tables, and `frontend-coding-style.md` for the layer that `deps.ts` belongs to.
