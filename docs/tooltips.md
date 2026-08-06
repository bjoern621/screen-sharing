# Tooltips

Every settings field and select option can carry an explanatory tooltip.
The tooltips are the form's teaching layer: they explain the encoding model as it is configured, and on the native grid they explain the figures the stats overlay prints.
This doc covers how one is wired, where its text comes from, how an unavailability note is appended, and how the native grid does without the component the form uses.

## The layers

A tooltip passes through three layers, from the Base UI primitive up to the app-facing text.

**Primitive.** `components/ui/tooltip.tsx` wraps `@base-ui/react/tooltip`.
`TooltipContent` sets the popup styling, the arrow, and the collision rule: prefer the requested side flipping left/right, fall back to the perpendicular axis preferring above.
`TooltipProvider` is mounted once at the root (`App.tsx`) with `delay=0`, so a hovered trigger shows its tooltip immediately.

**`Tip`.** `components/Tip/Tip.tsx` is the app-facing wrapper every component uses.
It takes a `text` string and renders `children` as a non-interactive trigger.
Base UI takes the trigger through its `render` prop, not a Radix-style `asChild`.
`TooltipContent` carries `whitespace-pre-line`, so a `\n` in `text` renders as a line break.

**Consumers.** Two components build a `Tip`: `FieldShell` for a field's label, `OptionRow` for a select option.
Neither is used directly. Field wrappers (`SelectField`, `NumberField`, `TextField`, `UplinkField`) compose `FieldShell`, and `SelectField` renders one `OptionRow` per option.

## Where the text comes from

Field label text is the `labelTip` prop, written at the call site in `StreamSettingsCard`.
Text that depends on the current settings is built by a helper in `util/options.ts` instead of hard-coded there, so it cannot describe a codec the user did not select.
`cqTip` is the case that needs it: the quantizer scale follows the codec and, where the two engines set different properties, the capture backend's engine, so the tip places its quality landmarks on the scale the running combination actually counts on.

A tooltip that mentions a transport names its leg, publish (publisher to relay) or watch (relay to viewer), because the two are chosen independently and a bare protocol name would read as both.
Leg-neutral protocol facts stay in `TRANSPORT_META`, whose entries serve the publish select and the watch dropdown alike; whatever holds for one leg only belongs in that field's own `labelTip`.

Option text is the `tip` field on an `Option`, declared once in the option metadata (`util/options.ts` and the domain meta tables) and never inlined twice.
An `Option` may also carry a `link`; `OptionRow` renders it as an `InfoIcon` beside the label that opens the reference article in the system browser.

```ts
{
    value: "srt", label: "srt - Secure Reliable Transport",
    link: "https://en.wikipedia.org/wiki/Secure_Reliable_Transport",
    tip: "Secure Reliable Transport: UDP with selective retransmission (ARQ) ...",
}
```

## Availability notes are additive

When a control or option is unavailable, its reason is appended to the normal tip rather than replacing it.
The tooltip states what the thing does, then a blank line, then why it is inert, so the description is never lost.

`FieldShell` appends `Ignored: <reason>` for a whole control that the current settings ignore.
`OptionRow` appends `Unavailable: <reason>` for a single disabled option.
Both join with a blank line and drop the empty half, so a field with no `labelTip` shows only the status note.

```ts
const ignored = disabledReason ? `Ignored: ${disabledReason}` : "";
const tip = [labelTip, ignored].filter(Boolean).join("\n\n");
```

A live control can also carry a note, appended by `withNote` at the call site: `deps.note` explains what the value does in a combination the base text does not cover, without greying the field.

The reasons are not hand-written here.
They come from `deps.disabled` (whole control), `deps.note` (live control) and `deps.optionDisabled` (single option) produced by `evaluateDeps` in `util/deps.ts`.
See `field-availability.md` for the rule deciding whether an inapplicable field is hidden or disabled-with-a-reason, and `domain-model.md` for the tables the reasons derive from.

## The native grid

GTK carries a tooltip on any widget and walks up to the first ancestor that has one, so the grid needs no wrapper component.
The teaching layer there is the stats overlay: a card of keys and numbers explains nothing on its own, so every row and block heading carries a tooltip, declared in the same table as the row (`internal/ui/stats/rows.go`).
The row's box holds it, which covers the key, the value and the gap between them.

A value too long for the card ellipsizes, and its label then carries the full text above the row's explanation, joined by the same blank line the form uses for its notes.
Where the value fits, the label takes itself out of the tooltip query instead of carrying an empty one, so the row's explanation reaches the pointer.

Transport counters are not in that table: the player labels them, so `player.StatRow` carries a `Tip` beside its `Label` and the element table in the GStreamer backend fills both.

The sidebar's watch-leg popover is the other teaching surface there, and none of its text lives in that binary.
Each knob crosses the process boundary in the roster with the transport that declared it (`transport.WatchTunable`), tip included, so what SRT's latency window means sits beside the code that writes it into the source fragment.

The app bar under the rows is the exception that stays in the binary: its two controls act on the app rather than on a stream, and their tooltips say what each reaches into the other process for and what it leaves playing.

## Adding a tooltip

- A new field: pass `labelTip` (and optional `labelLink`) to the field wrapper. The shell renders the tooltip; there is no markup to write.
- A new select option: add `tip` (and optional `link`) to the `Option` in `util/options.ts`. `OptionRow` renders it.
- Anything else: wrap the element in `Tip` with a `text` prop.
- A native grid stat row: add `tip` beside its `key` in the blocks table, or `tip` to the field in `statSources` for a transport counter.
- A native grid watch option: write it into the knob's declaration in the transport's `watchKnob` list (`desktop/internal/transport`). It travels in the roster and the popover renders it.
