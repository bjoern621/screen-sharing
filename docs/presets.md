# Presets

A preset is a promise about the picture, not a snapshot of the form.

Which encoder, pixel format and capture backend deliver a promise is the machine's answer and differs per machine: an NVIDIA desktop on X11 codes lossless planar RGB, the same desktop on Wayland reaches 4:4:4 and no RGB, and a machine with no GPU encoder reaches neither at 60 fps.
A stored set of field values can only be right on the machine it was written on.
So the table states the goal, and a search finds a configuration for it here.

Three presets exist: `lossless`, `gaming` and `readability`.
They live in `util/presets.ts`, and the region algebra they are written in lives in `util/claim.ts`.

## The two halves of a preset

**The claim** is a region of the settings space, declared per axis: which rate-control modes, which pixel formats, which frame rates, how many B-frames, how much SRT retransmit window.
Every settings object inside the region delivers what the preset's one-line hint says, and every object outside it does not.
The claim is what the selector reads, so it decides when a preset is still the selected one.

**The ladder** is an ordered list of rungs, each a picture the preset would accept, best first.
The order is where the preset states what it gives up first.
Lossless gives up the encoder before the format while both run on silicon, and the format before the encoder once neither does, because a CPU codes lossless 4:4:4 an order of magnitude faster than it codes lossless RGB and an encode that cannot keep up delivers no format at all.

The rest of a preset is its `base`: the rate-control recipe, frame rate and latency windows every candidate carries.
That part is the preset's identity rather than something to search for.

## Applying searches, it does not assign

`resolvePreset` walks rungs, and inside a rung codecs, and inside a codec capture backends.
Each candidate is put through `normalize` and kept only if it comes back intact (`applyIntact`): a candidate normalize had to repair is a different configuration under the same name, so it is rejected and the next one tried.
The first candidate that survives and delivers the claim is written to the settings whole.

The three axes the search varies:

- **Pixel format**, from the ladder.
- **Codec**, from the capability table: the encoders that come with a device before the ones that come with a build, and coding efficiency inside each half in opposite directions.
  A format spends fewer bits by searching more for them; on dedicated silicon that search is free, so the most efficient format wins, and on a CPU it is the frame rate, so the cheapest one wins.
- **Capture backend**, from `autoCaptures`, with the selected one first.
  A configuration reachable without switching backend is therefore the one taken.
  A backend behind a privilege nothing can check in advance is not in that set: `kmsgrab` needs CAP_SYS_ADMIN, so it stays selectable by hand and is never picked on the user's behalf.

The publish transport is not searched.
It is how viewers are reached rather than a property of the picture, so a preset never moves it, and the sentence on an unavailable preset names it as the thing the search worked within.

Applying twice equals applying once: the settings a search returns are themselves the candidate the next search reaches first.

## Unavailable presets

A preset no candidate satisfies is greyed in the selector with the reason, in the same treatment `field-availability.md` gives an option the tables rule out.
Nothing is applied and nothing is approximated: a repaired near-miss would be a configuration the user did not ask for, carrying the name of one they did.
A machine whose only encoders are VAAPI has no lossless preset, because no VA profile codes bit-exact.

## The selector is derived

The selected preset is read from the settings on every change, never remembered from the click that applied it.
A field edited to a value the claim still covers keeps the preset; one edited past the claim leaves it.
Changing the codec under `lossless` keeps `lossless`, and dropping the pixel format to 4:2:0 does not, because subsampled chroma is not what the preset promised.

Because the answer is derived, there is no stored selection to disagree with the settings, and no state to reconcile after a restart.

## At most one preset can match

Two presets whose claims intersect would both describe one settings object, and the selector has one value to show.
The claims are therefore written to be pairwise disjoint, each pair parting on one axis: the rate-control mode separates `lossless` from the other two, and the frame rate separates those two from each other.

`overlaps` in `util/claim.ts` decides that question from the same axis table `holds` reads, and the preset table is checked against it at load.
A claim widened past its neighbour fails there, rather than at a selector left to pick one of two right answers.

Settings that satisfy no claim read as `Custom`.
That is a state and not a choice, so the entry carries the reason and cannot be selected: the way out of it is to pick a preset.

## Saved presets

A user's own preset is the other thing entirely: a snapshot of every field, stored by the Go `settings` package and applied whole.
It goes through `normalize` on the way in and nothing else, since it was written on some machine and may name a codec or a capture backend this one lacks.
Where a built-in preset rejects a repaired candidate and tries the next, a snapshot has no next to try.

It carries no claim, so it is selected only while the settings equal it field for field, and it wins over a claim when they do, being the more exact statement.

## Adding a preset

Add the entry.
Its claim must be disjoint from every existing one, which the load-time check enforces, and its hint must be true of every settings object the claim accepts, which is what makes the claim a safe test for the selector.
The ladder holds only the pixel formats the preset would accept; the codec and capture backend follow from the tables and need no listing.
