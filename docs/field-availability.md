# Field availability: hide vs disable

A settings field can become inapplicable to the current combination of capture backend, codec and mode.
Two treatments express that, and the choice between them is a fixed rule, not a per-field judgement.
A third form covers the neighbouring case, a field that stays applicable but means something else.

## The treatments

**Hidden.**
The field is not rendered at all.
Conditional JSX in `StreamSettingsCard` gates it on the parent selection, for example the DRM download field rendered only when the capture API is `kmsgrab`.

**Disabled with a reason.**
The field renders greyed, carrying a `disabledReason` tooltip that states why it is inert.
The reason comes from `deps.disabled` in `evaluateDeps` (`util/deps.ts`), for example the NVENC preset ladder greyed under software x264 with "the p1-p7 ladder is NVENC-specific".

**Live with a note.**
The field stays editable and its tooltip gains a sentence from `deps.note`.
This is for a combination where the value still reaches the encoder but does something the field's own text does not describe, such as the bitrate becoming a burst ceiling in constant-quality mode on NVENC.
A note is not a third treatment of inapplicability: it exists so a knob that a builder does forward is never greyed, which would leave the encoder using a number the form refused to show.

## Three facts decide a rate-control field

A quantizer target, bitrate bound, rate buffer, B-frame count or preset is live only when all three agree.

- The **mode's concept** uses the knob: `MODE_META` in `util/domain.ts` says which controls each rate-control mode needs.
- The **codec's encoder** has the knob: the B-frame count and the p1-p7 ladder exist on NVENC only.
- The capture backend's **engine** forwards the value: `ENGINE_RULES` records where a builder drops a knob the mode uses, so the preset ladder greys on the portal path whose GStreamer elements have no equivalent.

When two of them block the same field, the reason names the one the user can act on.
B-frames under software x264 in VBR read "only the NVENC encoders take a B-frame count", not the mode sentence that would be a lie there.

## The rule

Ask what kind of control the field is.

- A **backend implementation knob** that has no meaning outside one backend is **hidden** when that backend is not selected.
  Its tooltip describes a mechanism that a user on any other backend has no reason to read.
  DRM download is a knob of the kmsgrab scanout path and nothing else.

- A **general encoding or quality concept** that the current combination happens to block stays **disabled with a reason**.
  The concept is part of the model every user is expected to understand, so the greyed field plus its reason teaches why the concept does not apply here.
  Encoder preset, quantizer target, bitrate bound, B-frames, color range and chroma are all general concepts, disabled when the codec or mode rules them out.

The test: would the tooltip teach a user on a different backend something worth knowing?
Yes means disable-with-reason.
No means hide.

## Why the split exists

The settings form is pedagogical: dense tooltips explain the encoding model as the user configures it.
A greyed field with a reason participates in that teaching, so a user hunting for the NVENC preset under x264 reads why it is absent instead of finding a blank.
A hidden field removes noise that would teach nothing, so a backend implementation knob does not follow every user across backends it does not concern.

## Where the rules live

Disable reasons are derived, never hand-set per field.
`evaluateDeps` produces `deps.disabled` and `deps.note` from the capability table, the domain meta tables and the engine rules, the same source `normalize` repairs settings from, so a disabled option and its fallback cannot disagree.
See `domain-model.md` for the capability and meta tables, and `frontend-coding-style.md` for the layer that `deps.ts` belongs to.
