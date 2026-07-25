# Viewer design language

The desktop frontend (the wails app) defines the product's visual language.
Other viewer surfaces, native or web, follow it, including where that overrides platform convention.
Token values live in `desktop/frontend/src/index.css` (shadcn on Tailwind, zinc base).
This page states the rules, not the numbers.

## Palette

Surfaces use the neutral zinc scale through the shadcn tokens (`background`, `card`, `muted`, `border`).
Light and dark are the same token names with different values; components never hardcode a theme.
There is one accent, `primary`, an emerald green.
It marks selected, active, and connected.
The sidebar carries a brighter accent pair, `sidebar-primary`, so the accent reads on the sidebar surface.
There is one danger color, `destructive`, a red.
It marks failure and stop/disconnect actions, never state that is merely on.
Secondary text uses `muted-foreground`.
Dark-theme borders are white at low alpha rather than a gray value.

## Video surfaces

Tiles are black in both themes; video defines its own background.
Everything drawn over video uses white at graded alpha (90, 70, 60, 15), not theme tokens.
Floating cards over video are the exception: theme `background` at high alpha with the standard border, so the controls they hold stay themed.
Tile chrome: radius `lg`, a 1px ring of `foreground` at 10%, corner labels in black-60 chips with white text.
A spotlit tile upgrades the ring to 2px `primary` at 60%.

Media controls sit in one such card, bottom-center, faded in on tile hover: ghost icon buttons, `destructive` only on disconnect, active toggles in `primary`.
The volume slider rises above the mute button in a matching card; a muted stream repeats the volume-off glyph in its corner label.
The stats overlay is a black-75 monospace card under the corner label, white keys at half alpha.

## Status language

A stream's connection state speaks one vocabulary everywhere (roster chip, tile, buttons):

- Idle: a small static dot, `muted-foreground` at half alpha.
- Connecting: a spinning `loader-2` icon.
  Where the transport reports connect phases (requesting, negotiating, buffering), tiles add them as a step bar with a plain-words label and the reached step pulses; otherwise the tile names the one thing it waits for.
- Live: the same small dot, colored and pulsing.
  On neutral surfaces the dot is `primary` (`sidebar-primary` in a sidebar).
  On a filled button it inherits the button's foreground; the destructive stop-publishing button adds an expanding ping ring.
- Failed: a `destructive` icon (`alert-triangle` in chips, `plug-connected-x` on tiles), the error message, and an outline retry button.

The dot stays small (6px): it is state, not decoration.
Red never means live.
The red dot on the stop button is the button's foreground on a destructive action, not a live color.

## Wording

Headings and buttons are sentence case: "Web grid", "Audio only", "Close".
Everything else is lowercase, including tooltips, chips, empty-state sentences and stat keys.
A figure keeps one name across surfaces: `transport`, `resolution`, `codec`, `bitrate`, `decoder`, `fps`, `frames`, `latency`, `lost`, and `n watching` for the number of open tiles.
A surface with more to report adds rows instead of renaming the shared ones.
Stat rows spell their words out, join two figures with ` · `, and print `…` where there is no value yet.
Transport names stay lowercase, the way the settings offer them: `rtsp`, `srt`, `webrtc`, `websocket`.
On a viewer surface a bare `transport` always means the watch leg, relay to viewer, on both grids; a label for the publisher-to-relay leg says "publish" (the settings field, the hop-1 latency).
Tile controls read `mute`/`unmute`, `stats`/`hide stats`, `spotlight`/`back to grid` and `disconnect`, and a failed tile offers `retry`.

## Selection

Watch toggles are pills.
Pressed: `primary` at 15% background and 50% border.
Toggling never rewrites the label; state shows beside it.

## Icons

Tabler outline icons (2px stroke, round caps): `@tabler/icons-react` on the web, vendored SVGs recolored at load on native surfaces.
Platform icon themes are deliberately not used, so every surface shows the same glyphs.
Sizes range 12 to 22px by surface.

## Motion

Subtle and short.
Hover and opacity transitions run around 200ms, video fade-in 500ms, tiles mount with a fade and slight zoom.
Named animations:

- `pulse`: opacity dips to 50% mid-cycle, 2s, ease-in-out.
- `ping`: a ring expands and fades, 1s.
- `shimmer`: a highlight sweeps across skeleton surfaces, 1.8s.
- `spin`: 1s linear rotation.

GTK CSS cannot animate transforms.
Native surfaces keep `pulse` (opacity) and drive `spin` from the frame clock; `ping` and `shimmer` stay web-only.

## Empty states

A dashed rounded border, a circular `muted` badge holding an outline icon, one `muted-foreground` sentence.
No heading, no button.

## Native (GTK) mapping

GTK CSS has no token pipeline, so token values are flattened to hex constants and a `.dark` class on the window switches them; `AdwStyleManager` drives the class.
Neutrals derive from the theme foreground at graded alpha instead of copying every zinc value.
The mapping lives in `nativegrid/style.css`.
