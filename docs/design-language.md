# Design language

One visual language, and every surface follows it, including where that overrides platform
convention.
Token values live in `avalonia/ScreenShare.App/Design/`, which is the reference
implementation: `Palette.axaml`, `Typography.axaml` and `Metrics.axaml` hold the numbers,
and `Text.axaml`, `Surfaces.axaml` and `Buttons.axaml` hold the roles built from them.
This page states the rules.

One surface speaks it, and it is the only one there is.
The two that carried the language this page replaced were deleted with the shells they
belonged to; see "Where each surface stands".

## Palette

Greyscale everywhere except a single red.

The greyscale ramp runs `#141414` `#181818` `#1C1C1C` `#212121` `#262626` `#2A2A2A`
`#303030` `#3A3A3A` `#4A4A4A` `#565656` `#6E6E6E` `#9A9A9A` `#C8C8C8` `#D0D0D0` `#E6E6E6`
`#F2F2F2` `#FFFFFF`.
It is listed once and then never named again: a component asks for the role it wants and
the palette decides what that is, which is what keeps a light variant a second dictionary
rather than a sweep through every view.

Surfaces are a four-step ladder: the app body, the two chrome bands one shade lighter, a
recessed panel, and a raised control.
There is one light surface, near-white, and it means selected — a chosen destination, an on
toggle, the current step.
Text on it is near-black.
Lines are three weights: a hairline where one band meets the next, a divider inside a strip
or a menu, and a raised control's edge.
Text is a five-step ladder from white down through the control label, secondary copy, muted
figures and faint hints, ending at the disabled grey.

There is one hue, `#E5484D`, and it is reserved strictly for **on air** and **something is
wrong**.
It is the only colour on any screen, which is what makes a fault unmistakable across a room,
and it is the only one a colour-blind reader still separates from grey reliably.
Spending it on state that is merely on would cost the app its single unambiguous signal.
Red therefore appears at most a few times per screen, and exactly once on the control that
changes the world: `Go live`, and `Stop`.

Everything else that is merely notable is carried by weight, by fill, or by inversion.
A satisfied check is a filled grey badge with a white tick, not a green one.

Hover, pressed and focus are **not** specified by the design.
The Avalonia module's choice — one step up the ramp on hover, one step down on press, and
the red held at reduced opacity rather than lightened — is a decision recorded in
`Design/Palette.axaml`, so no new value enters the palette.

The design states no text field, no number field and no slider either.
The module's choice — a typed value wears the same raised control a button does and is set
in tabular figures because it is typed digit by digit, a number field is that same box
without a stepper, and a
slider is a 6px track whose travelled half takes the one light surface — is recorded in
`Design/Inputs.axaml` and again spends nothing the palette does not already hold.
A flag is the switch, never a tick box: one domain concept has one control.

## Typography

**One family carries the whole product: Inter.**
Two weights, 500 and 600, and nothing else.

Inter is bundled rather than asked for by name and hoped for.
A surface that names the platform sans first renders in Segoe UI on Windows, San Francisco
on macOS and whatever fontconfig picks on Linux — three different faces at three different
apparent sizes, which is the opposite of one product.
The Avalonia module ships the family with the app (`Avalonia.Fonts.Inter`, registered in
`Program`), so the name resolves to the same face everywhere; the platform sans is a
fallback behind it and should never run.

The second family is gone, and with it the rule that anything machine-generated is mono.
What the split was really buying is column alignment in a run of digits, and that is a font
feature rather than a font: **numbers that tick, count or sit in a column are set in tabular
figures** (`FigureFeatures`, Inter's `tnum`), which gives every digit one advance width
without changing the face.
A timer, a throughput reading, a table cell and a plot annotation therefore hold still as
they update, and a line that mixes prose and figures — a status bar segment, a chip and its
count, a sentence naming a stream — no longer changes shape halfway through.
What is machine-generated is said by role instead: the identifier and figure roles are one
step quieter and one step smaller than the prose beside them.

Weight is where the dark palette is paid for.
400 on `#141414`–`#262626` surfaces reads thin enough that muted grey copy starts to
disappear, so the body weight is **500** and emphasis is **600**; regular is not used
anywhere.

Sizes are a closed set of four whole pixels — **12, 13, 14 and 16** — plus a single 26px
reserved for the figures a publisher watches while live, which are sized to be read from
across a room.
**14px is the default**: a control's label, body copy, and what a line with no role stated
renders at.
12px is the floor and is for text that labels rather than says — a step badge's number, a
table's column keys.

Whole pixels, no half steps, and no two steps closer than a pixel apart in the body range.
The set before this one ran 11 to 16 in seven steps, and both halves of that were
legibility rather than taste.
It was small — the form's help text is the product's actual content rather than chrome, and
11px body copy sits under the 12px floor every published desktop scale starts at.
And seven steps inside a 5px range is not a ladder anyone can hear: 13 beside 14 beside 15
reads as one size rendered unevenly, so the steps cost churn and bought no hierarchy.

Prose that wraps states its line height (18px under the small size, 20px under the body
size, both ~1.4×); a single line in a row that is already centred does not, because a line
box taller than the glyphs only moves the text off the centre it was placed on.
Bands and rows are sized from the text they hold rather than the other way round, which is
why the chrome heights in `Metrics.axaml` moved when the default size did.

There is no letter-spacing and no text-transform anywhere.
Labels render in the case they are written in.

## Surfaces and shape

Radius is chosen by what a thing is, not by how big it is: a segment, a control, a button, a
strip, a panel, a video tile, and the one capsule — a stream chip.

Only two shadows exist in the entire product: the window's own, and a context menu's.
Nothing else is elevated.
A selected segment is flat; a card is separated by its border, not by a drop shadow.

The window's chrome is the app's, not the platform's: a custom title bar, and beneath it a
nav strip that holds the same two regions on every screen — the three destinations on the
left, live state on the right.
The strip carries no breadcrumb: the lit segment already says where you are, and a second
label saying it again is noise in the one row that must stay scannable.
The destinations never move, so the strip becomes muscle memory.
An unavailable destination **dims rather than disappears**, and the whole treatment is a
colour change: no opacity, no strike-through, no badge.
An expert tool should teach its own shape, and a missing tab reads as a bug.

## Video surfaces

Tiles are black in both themes; video defines its own background.
A tile keeps its own aspect inside its cell — a 4:3 camera pillarboxes, a 21:9 desktop
letterboxes — and is never cropped or stretched.
Tile chrome is a 9px radius, a name plate in the lower left, and figures in the lower right.

A grid of tiles is equal cells, and the arrangement is derived rather than configured: the
one with the largest fitted picture wins, and a short last row centres itself.
No column count is written down anywhere.
Maximising the cell instead of the picture inside it is the wrong objective — it picks a
single long row every time.

A tile draws no ring, no outline and no dashed edge, in any state.
The edge belongs to the grid rather than to the stream inside it, so a stream that starts
speaking, goes on air or begins dropping frames never repaints the boundary the reader
navigates by, and one struggling stream never becomes the loudest thing on a screen someone
is scanning for something else.

What a stream is doing is said on its face instead, over the picture and at no cost to
layout: a name plate with a small presence dot in the lower left, a status badge in the
upper right, and the figure in the lower right.
The badge is neutral except on this machine's own outgoing stream, which is the one filled
red badge, because that is on air.
A struggling stream wears the same neutral badge as any other, names itself in words, and
prints its drop count in the figure at a heavier weight.
Weight, never a second colour: a fault on a video surface is read, not spotted.

Hiding a stream is a performance control, not a preference: it tears the decoder down, so
the surface that offers the toggle also reports the bandwidth and decode load that frees.

## Status language

A stream's connection state speaks one vocabulary everywhere — chip, tile, status bar,
button:

- Idle: a small static dot in the muted grey.
- Connecting: a spinning indicator.
  Where the transport reports connect phases, tiles add them as a step bar with a
  plain-words label; otherwise the tile names the one thing it waits for.
- Live: the same small dot, and on the publishing surface the on-air pill — a solid white
  dot, the words `On air`, and an elapsed timer in tabular figures.
- Failed or degraded: the one red, the reason in words, and a retry the surface already
  offers.
  Video tiles and the chips that drop them are the exception: they carry the reason in words
  alone and spend no colour on it, for the reason "Video surfaces" gives.

The dot stays small (7px): it is state, not decoration.
**Red never means live in the sense of merely connected.** It means on air — this machine is
broadcasting right now — or it means broken. Those are the only two.

## Waiting

A control that starts a call the backend has to answer says so until it is answered.
Go live, Stop, Measure, Look again, Open full log, a stream's grid toggle and its watch legs
are all of them; none of them is instant, and a control that answers a press by going quietly
inert is read as a broken one.

The treatment is one thing everywhere: the label is replaced in place by a turning arc, in
the control's own foreground, inside the box the label had.
The box does not resize, so a row of controls does not reflow around the one that is working.
The arc turns at a constant rate and states no progress, because nothing here knows any — a
control call is answered or it is not.
A waiting control is not pressable — a second press would ask the backend for the same thing
twice — so it also takes the unavailable treatment while the arc turns.
The arc is what tells the two apart: a control that is merely unavailable keeps its label and
its reason stands in words nearby, and a control that is waiting shows neither, because the
reason is that it is working.

## Selection

Selection is inversion: the selected thing takes the one light surface and near-black text.
That rule is identical everywhere — a destination in the nav strip, a row in a list, the
current step — so selection reads the same on every screen without a legend.
Toggling never rewrites a label; the state shows beside it or in the fill.
A menu row that toggles something shows it the same way: the words stay put and a tick, a
switch or a value sits at the end of the row.
Every row that acts on state shows that state, so nothing in a menu asks the reader to
remember what the last click did.

## Wording

Anything written as words for a reader is sentence case: headings, buttons, field labels,
empty states, status lines and failure messages.
"Go live", "Force keyframe", "Edit in setup", "Read-only while live."
A standalone sentence takes a full stop.
A fragment that labels a state does not: "No frames arriving", "Waiting for the first frame".

Lowercase is reserved for the figures, which are identifiers rather than prose: stat keys,
chip values, table column headers and transport names.

A tooltip is prose: it opens with a capital, closes with a full stop, and explains the
control or the figure instead of naming it a second time.
An icon button whose tooltip repeats its glyph teaches nothing, so the tooltip says what
pressing it does and what it leaves alone.

A figure keeps one name across surfaces: `transport`, `resolution`, `codec`, `bitrate`,
`decoder`, `fps`, `frames`, `latency`, `rtt`, `loss`, `buffer`, and `n watching` for the
number of open tiles.
A surface with more to report adds rows instead of renaming the shared ones.
Stat rows spell their words out, join two figures with ` · `, and print `…` where there is no
value yet.
Transport names stay lowercase, the way the settings offer them: `rtsp`, `srt`, `webrtc`,
`websocket`.
On a viewer surface a bare `transport` always means the watch leg, relay to viewer; a label
for the publisher-to-relay leg says "publish".

## Ownership

A control appears in **exactly one** window.
The window that owns a value edits it; any other window that shows the same value shows it
read-only, with a single link back to the owner.
A second editor for one setting is the defect this rule exists to prevent.

Setup owns every configuration decision.
Broadcast owns the live-safe actions — the ones that change the stream without tearing it
down — and shows configuration as a read-only list with one `Edit in setup` link.
Viewer owns per-tile and per-viewer decoder overrides, and nothing else.

## Icons

Outline glyphs, drawn as vector paths at a 1.2px stroke with round caps.
Emoji are never used: the platform emoji font paints them in colour and ignores the
foreground brush, so a button's states become inexpressible.
Window controls are geometry rather than text, for the same reason a font's box-drawing
metrics cannot be relied on — on Windows the caption glyphs live in Segoe Fluent Icons'
private use area, so a face that is missing paints a tofu box where the close button was.
Sizes range 12 to 22px by surface.

The shell uses the Tabler outline set, through the `TablerIcons.Avalonia` package.
Platform icon themes are deliberately not used, so every surface shows the same glyphs.
No surface draws its own path: a tick or a chevron that was hand-written would be a fourth
icon set nobody maintains, and it would miss the stroke rule above the first time it was
resized.

The window controls are the one exception, and they are not on the ladder above.
They stand in for the caption buttons the platform hid when the client area was extended
over them, so they are the platform's shapes at the platform's measurements: on Windows a
10px box at a 1px line with mitred corners — minimise, maximise, restore, close — and the
middle button draws restore rather than maximise while the window is maximised.
A reader compares them with every other window on the screen rather than with the app
underneath them, so an app-set glyph at the app's stroke is wrong there even when it is
the same three lines.

## Motion

Subtle and short.
Hover and opacity transitions run around 200ms, video fade-in 500ms, tiles mount with a fade
and slight zoom.
Named animations:

- `pulse`: opacity dips to 50% mid-cycle, 2s, ease-in-out.
- `ping`: a ring expands and fades, 1s.
- `shimmer`: a highlight sweeps across skeleton surfaces, 1.8s.
- `spin`: 1s linear rotation.

GTK CSS cannot animate transforms.
Native GTK surfaces keep `pulse` (opacity) and drive `spin` from the frame clock; `ping` and
`shimmer` stay web-only.

## Empty states

A dashed rounded border, a circular muted badge holding an outline icon, one muted sentence.
No heading, no button.

A step or panel with nothing to show says so plainly rather than inventing content.
A figure the app has not measured prints `…`; it never prints zero.

## Where each surface stands

| Surface | Speaks this language |
| --- | --- |
| `avalonia/ScreenShare.App` | Yes, and it is the only surface. It is the reference implementation, and `Design/` is where the numbers live. |

There is nothing left to port.
The two surfaces that did not speak this language — the Wails frontend on shadcn zinc with
an emerald accent, and the GTK grid carrying that palette flattened to hex constants — were
deleted rather than restyled (`ipc-api.md`, `viewer-architecture.md`).
A language with one speaker needs no conformance column, and this section stays only to
record that the split it described is closed.
