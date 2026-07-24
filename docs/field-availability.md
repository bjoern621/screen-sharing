# Field availability: hide vs disable

A settings field can become inapplicable to the current combination of capture backend, codec and mode.
Two treatments express that, and the choice between them is a fixed rule, not a per-field judgement.

## The two treatments

**Hidden.**
The field is not rendered at all.
Conditional JSX in `StreamSettingsCard` gates it on the parent selection, for example the DRM download field rendered only when the capture API is `kmsgrab`.

**Disabled with a reason.**
The field renders greyed, carrying a `disabledReason` tooltip that states why it is inert.
The reason comes from `deps.disabled` in `evaluateDeps` (`util/deps.ts`), for example the NVENC preset ladder greyed under software x264 with "the p1-p7 ladder is NVENC-specific".

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
`evaluateDeps` produces `deps.disabled` from the capability table and the domain meta tables, the same source `normalize` repairs settings from, so a disabled option and its fallback cannot disagree.
See `domain-model.md` for the capability and meta tables, and `frontend-coding-style.md` for the layer that `deps.ts` belongs to.
