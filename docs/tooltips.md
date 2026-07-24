# Tooltips

Every settings field and select option can carry an explanatory tooltip.
The tooltips are the form's teaching layer: they explain the encoding model as the user configures it.
This doc covers how one is wired, where its text comes from, and how an unavailability note is appended.

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

The reasons are not hand-written here.
They come from `deps.disabled` (whole control) and `deps.optionDisabled` (single option) produced by `evaluateDeps` in `util/deps.ts`.
See `field-availability.md` for the rule deciding whether an inapplicable field is hidden or disabled-with-a-reason, and `domain-model.md` for the tables the reasons derive from.

## Adding a tooltip

- A new field: pass `labelTip` (and optional `labelLink`) to the field wrapper. The shell renders the tooltip; there is no markup to write.
- A new select option: add `tip` (and optional `link`) to the `Option` in `util/options.ts`. `OptionRow` renders it.
- Anything else: wrap the element in `Tip` with a `text` prop.
