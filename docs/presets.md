# Presets

A preset is a promise about the picture, not a snapshot of the form.

Which encoder, pixel format and capture backend deliver a promise is the machine's answer.
An NVIDIA desktop on X11 codes lossless planar RGB.
The same desktop on Wayland reaches 4:4:4 and no RGB.
A machine with no GPU encoder reaches neither at 60 fps.
A stored set of field values can only be right on the machine it was written on.
So the table states the goal, and a search finds a configuration for it here.

`lossless`, `gaming` and `readability` live in `form/presets.go`, beside the capability table the search walks and the repair it holds every candidate to (`ipc-api.md`).

They reach a surface on the resolved form, one entry per preset: the settings applying it would produce here, or the reason nothing here reaches it, and whether the draft already delivers it (`form.proto`, `BuiltinPreset`).
They belong there rather than on a method of their own: each of those answers changes with the draft exactly as a greying does.
A surface that asked for them separately would hold a preset verdict older than the settings beside it.

**A preset is a `PublishSettings` and nothing else.**
Relay coordinates are per-site, and the viewer's own fields are per-driver: a render chain this machine registers is one the machine it is copied to may not.
So neither travels in a preset, and a preset that carried either would be the thing that breaks on the machine it was copied to.

## The two halves of a preset

**The claim** is a region of the settings space, declared per axis: which rate-control modes, which pixel formats, which quantization ranges, which frame rates, how many B-frames, how much SRT retransmit window.
Every settings object inside the region delivers what the preset promises, and every object outside it does not.
The claim decides whether a preset is still the selected one.

The retransmit window on it is the publish leg's alone.
A viewer pays that window and the watch hop's together, but the watch hop belongs to the machine doing the watching and a preset carries no viewer settings.
The claim speaks for the half a preset can move.

Colour range is one preset's axis and not a shared one.
Coding the desktop into the narrower studio swing throws away code values before the encoder sees them, so `lossless` claims full range and would not be itself without it.
The other two promise nothing about the range and leave the settings' own where it is.
A base that wrote a value the claim does not mention would strand the preset on every machine that refuses that value, for a promise the preset never made.
The GStreamer VA elements signal no colour description, which is exactly such a machine.

**The ladder** is an ordered list of rungs, each a picture the preset would accept, best first.
The order is where the preset states what it gives up first.
Lossless gives up the encoder before the format while both run on silicon, and the format before the encoder once neither does.
A CPU codes lossless 4:4:4 an order of magnitude faster than it codes lossless RGB, and an encode that cannot keep up delivers no format at all.

The rest of a preset is its `base`: the rate-control recipe, frame rate and retransmit window every candidate carries.
That part is the preset's identity rather than something to search for.
It writes those fields and leaves the others standing, so a field the preset promises nothing about keeps what the settings hold.
A bitrate target means nothing to a lossless encode, and zeroing it would spend a figure the user chose on a field the preset never reads.

## Applying searches, it does not assign

`presetResolve` walks rungs, and inside a rung codecs, and inside a codec capture backends.
Each candidate goes through the same repair the form uses (`form/repair.go`).
A repair that walks the pixel format, either half of the encode or the capture backend has answered a different question than the one the search asked, so that candidate is rejected and the next one tried.
So does one that walks a setting outside the publish group, which is what a preset cannot carry.
The first candidate that survives and delivers the claim is written to the settings whole.

Every other field arrived holding what the settings held rather than something the preset chose, so the repair's answer for it is the machine's.
The GStreamer VA elements signal no colour description, and a draft on full range therefore leaves that range behind when it lands on one of them.
A preset that promised nothing about the range is still itself afterwards, and gating on the field instead would put every VA encoder out of reach and land a 60 fps desktop on a software encode.
What the promise does cover, the claim decides on the repaired settings: a rate-control mode walked to another one the promise names is still the preset, and one walked past it is not.

The settings a search runs against are the repaired ones, which makes rejecting a repaired candidate sound rather than merely strict.
A draft still holding a stranded value would have every candidate moved by the repair, and every preset would then be unreachable for a fault none of them has.
`Resolve` repairs first and everything after it describes what that reached, so the only thing left for a repair to move is what the preset itself put there.

A candidate whose own pixel format, encoder or capture backend the repair would already walk is dropped before the repair runs.
That is the same question asked of the same function, one field at a time instead of a fixed point over the whole table.
It decides nothing new, and it keeps a machine that reaches no preset from paying for a full repair per candidate on every keystroke.

The axes the search varies:

- **Pixel format**, from the ladder.
- **Codec**, from the capability table: the encoders that come with a device before the ones that come with a build, and coding efficiency inside each half in opposite directions.
  A format spends fewer bits by searching more for them.
  On dedicated silicon that search is free, so the most efficient format wins.
  On a CPU it is the frame rate, so the cheapest one wins.
- **Capture backend**, from `publish.AutoCaptures`, with the selected one first, so a configuration reachable without switching backend is the one taken.
  A backend behind a privilege nothing can check in advance is not in that set: `kmsgrab` needs CAP_SYS_ADMIN, so it stays selectable by hand and is never picked on the user's behalf.

The publish transport is not searched.
It is how viewers are reached rather than a property of the picture, so a preset never moves it.
The leg stays put while the engine under it moves with the capture backend, and a leg carries different formats on the two engines.
So which codecs a rung can reach changes as the search walks the backends.

Applying twice equals applying once: the settings a search returns are themselves the candidate the next search reaches first.

## Unreachable presets

A preset no candidate satisfies carries the reason instead of settings.
A surface draws it ruled out with that reason beside it, the same treatment `field-availability.md` gives an option the tables rule out.
Nothing is applied and nothing is approximated: a candidate whose own rung, encoder or backend the repair moved would be a configuration the user did not ask for, carrying the name of one they did.
A machine whose only encoders are VAAPI has no lossless preset, because no VA profile codes bit-exact.

The reason names the publish transport, the one dimension the search worked within rather than varied, so what it hands the reader is something they can change.

## The selection is derived

Whether a preset is the one in force is read from the settings on every resolve, never remembered from the click that applied it.
A field edited to a value the claim still covers keeps the preset.
One edited past the claim leaves it.
Changing the codec under `lossless` keeps `lossless`, and dropping the pixel format to 4:2:0 does not, because subsampled chroma is not what the preset promised.
No stored selection can disagree with the settings, and there is no state to reconcile after a restart.

## At most one preset can match

Two presets whose claims intersect would both describe one settings object, and a surface has one selection to show.
The claims are therefore written to be pairwise disjoint, each pair parting on one axis: the rate-control mode separates `lossless` from the other two, and the frame rate separates those two from each other.

The overlap check decides that question from the same axis table the claim test reads, and the preset table is checked against it at load.
A claim widened past its neighbour fails there, rather than at a surface left to pick one of two right answers.

Settings that satisfy no claim deliver no preset, and nothing is marked.
That is a state and not a choice: the way out of it is to pick a preset.

## Saved presets

A user's own preset is the other thing entirely: a snapshot of every field, stored by the Go `settings` package and applied whole.
It goes through the repair on the way in and nothing else, since it was written on some machine and may name a codec or a capture backend this one lacks.
Where a built-in preset rejects a repaired candidate and tries the next, a snapshot has no next to try.

It carries no claim, so it is marked only while the settings equal it field for field.
The two kinds are two lists rather than one, so a draft that equals a snapshot and delivers a promise marks a row in each.
Neither has to win: they are two true statements about one draft, made about different things.

## Adding a preset

Add the entry.
Its claim must be disjoint from every existing one, which the load-time check enforces.
The name a surface writes for it must be true of every settings object the claim accepts, which is what makes the claim a safe test for the selection.
The ladder holds only the pixel formats the preset would accept.
The codec and capture backend follow from the tables and need no listing.
The base writes only what the claim speaks for: a field written past the claim can strand the preset on a machine that refuses that value.

A surface then owes the new key a name, as it owes every identifier one (`ipc-api.md`).
