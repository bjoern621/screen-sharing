# Design language

One visual language, every surface follows it, including where that overrides platform convention.
Token values live in `avalonia/ScreenShare.App/Design/`, the reference implementation: `Palette.axaml`, `Typography.axaml` and `Metrics.axaml` hold the numbers, `Text.axaml`, `Surfaces.axaml` and `Buttons.axaml` the roles built from them.
This page states the rules.

## Palette

Greyscale, and three hues that each answer one question about a stream.

Ramp: `#141414` `#181818` `#1C1C1C` `#212121` `#262626` `#2A2A2A` `#303030` `#3A3A3A` `#4A4A4A` `#565656` `#6E6E6E` `#9A9A9A` `#C8C8C8` `#D0D0D0` `#E6E6E6` `#F2F2F2` `#FFFFFF`.
Listed once and never named again: a component asks for the role it wants and the palette decides, so a light variant is a second dictionary rather than a sweep through every view.

Surfaces are four steps: app body, the two chrome bands one shade lighter, a recessed panel, a raised control.
One light surface, near-white, means selected: a chosen destination, an on toggle, the current step.
Text on it is near-black.
Lines are three weights: a hairline where one band meets the next, a divider inside a strip or menu, a raised control's edge.
Text is five steps from white down through control label, secondary copy, muted figures and faint hints, ending at the disabled grey.

| Hue | Says | Spent on |
| --- | --- | --- |
| red `#E5484D` | broken, or on air | a check that blocks, a failure sentence, the sharing pill and badge, `Stop sharing` |
| amber `#FFB224` | it runs, and something about it disappoints | a check that warns, a figure out of bounds, an estimate over the measured line, why a control is inert |
| green `#30A46C` | the way is clear | a check that passed, `Start sharing` |

Red is the one that has to carry across a room, so it stays scarce.
A screen with three red things on it says which one broke; a screen with thirty says nothing.
Amber always has a stream still running behind it, and states what that stream costs rather than asking for a press.
Green marks the way in and never the state at the end of it: `Start sharing` is green, and the machine that is sharing wears red.

A hue never says merely on, and never says selected, which is inversion ("Selection").
None of the three carries a fact alone: what is coloured also says itself in words, a glyph or weight, so a reader who separates none of them loses nothing.
A hue as a fill takes the label its contrast affords, white on red and green, near-black on amber.

**Why a control is inert is read before anything else on it.**
The reason takes the amber and the emphasis weight, so it stands above the paragraph teaching what the control would have done, and above the copy of a card that has greyed around it (`field-availability.md`).
A reader who has met a greyed control came for that line.

Everything a hue does not answer for is carried by weight, fill or inversion.

Hover, pressed and focus are **not** specified by the design.
The module's choice is recorded in `Design/Palette.axaml`, so no new value enters the palette: one step up the ramp on hover, one down on press, a hue held at reduced opacity rather than lightened.

The design states no text field, number field or slider either.
The module's choice is recorded in `Design/Inputs.axaml` and again spends nothing new.
A typed value wears the same raised control a button does, in tabular figures because it is typed digit by digit.
A number field is that box without a stepper.
A slider is a 6px track whose travelled half takes the one light surface.
A flag is the switch, never a tick box: one domain concept, one control.

## Typography

**One family for everything the product says: Inter.** Two weights, 500 and 600, nothing else.

Bundled rather than asked for by name and hoped for.
Naming the platform sans first renders Segoe UI on Windows, San Francisco on macOS and whatever fontconfig picks on Linux: three faces at three apparent sizes, the opposite of one product.
The module ships the family (`Avalonia.Fonts.Inter`, registered in `Program`), so the name resolves to one face everywhere and the platform sans is a fallback that should never run.

**One family for what another program said: JetBrains Mono.** Bundled the same way, out of `Assets/Fonts`.

A single role spends it, the **transcript**: a line reproduced from another process rather than composed here, which is the session log and what lands in it.
Such a line is an error nobody here wrote, wrapping over several rows of element names, socket addresses and codes, and the reader's next move is to copy a piece of it into a search box or a bug report.
Mono is what that reading needs: the columns hold under the wrap, and `0` stays unmistakable against `O`.
The cut bundled is the ligature-free one (`JetBrains Mono NL`), because `->` or `//` drawn as a single glyph is a string the reader cannot match against the text the backend printed.

Nothing else crosses over.
An identifier this product names for itself, `hevc_nvenc` or `gbrp`, sits inline in prose where a change of face would read as a change of subject, so it keeps Inter and is marked by the identifier role instead, one step quieter and one smaller than the copy beside it.
Digit alignment likewise stays a font feature rather than a face: **numbers that tick, count or sit in a column are set in tabular figures** (`FigureFeatures`, Inter's `tnum`), one advance width per digit without changing the face.
A timer, a throughput reading, a table cell and a plot annotation hold still as they update, and a line mixing prose and figures keeps its shape halfway through.

Weight is where the dark palette is paid for.
400 on `#141414`–`#262626` reads thin enough that muted grey copy starts to disappear, so body weight is **500** and emphasis **600**.
Regular is unused.

Sizes are four whole pixels, **12, 13, 14 and 16**, plus a single 26px for the figures a publisher watches while live, sized to read from across a room.
**14px is the default**: a control's label, body copy, and any line with no role stated.
12px is the floor, for text that labels rather than says: a step badge's number, a table's column keys.

Whole pixels, no half steps, no two steps closer than a pixel in the body range.
12px is the floor because the form's help text is the product's content rather than chrome, and it sits at the floor every published desktop scale starts at.
Gaps are a pixel or more because a ladder inside a 5px range is not one anyone can hear: 13 beside 14 beside 15 reads as one size rendered unevenly, so extra steps cost churn and buy no hierarchy.

Prose that wraps states its line height (18px small, 20px body, both ~1.4×).
A single line in a row already centred does not: a line box taller than the glyphs only moves the text off the centre it was placed on.
Bands and rows are sized from the text they hold, so the chrome heights in `Metrics.axaml` follow the default size.

No letter-spacing and no text-transform anywhere.
Labels render in the case they are written in.

## Surfaces and shape

Radius follows what a thing is, not how big it is: a segment, a control, a button, a strip, a panel, a video tile, and the one capsule, a stream chip.

Two things cast a shadow and nothing else is elevated.
The window's is the platform's, so the app states nothing for it.
The other belongs to a surface floating over the window: a menu, and the tooltip skinned like one.
A selected segment is flat, and a card is separated by its border.

Cards stack in a column with one gap between them, and the gap falls between the cards that drew something (`Controls/CardColumn`).
Two columns beside each other therefore start on one line, whatever either of them drew.

The window's chrome is the app's where the platform has one caption to stand in for: a custom title bar on Windows and macOS.
Beneath it a nav strip holds the same two regions on every screen, three destinations left and live state right.
On Linux the frame is the desktop's and no title bar is drawn: which buttons a window carries, which edge they sit on and whether it carries any are that desktop's answer, and a tiling session answers "none".
The nav strip is the first row of the window there, and it is the same strip.
No breadcrumb: the lit segment already says where the reader is, and saying it twice is noise in the one row that must stay scannable.
The destinations never move, so the strip becomes muscle memory.
Every one is **reachable at all times**, broadcast included.
That screen reports the stream that has ended as well as the one running, which is what a publisher goes looking for after a stream drops and what a live-only tab would take away.
An expert tool teaches its own shape, and a tab that comes and goes reads as a bug.

## Video surfaces

Tiles are black in both themes, video defining its own background.
A tile keeps its own aspect inside its cell (a 4:3 camera pillarboxes, a 21:9 desktop letterboxes) and is never cropped or stretched.
Tile chrome: 9px radius, a name plate lower left, figures lower right.

A stream filling a screen is the stream and nothing else: no chrome, no rail, no radius.
The aspect rule is the cell's, so the picture keeps its shape and the surround is black.
The way in is the tile's menu row and the key printed beside it.
Escape is always a way out, a screen that draws no controls still having to be one a reader can leave.

A grid is equal cells, and the arrangement is derived rather than configured: the one with the largest fitted picture wins, and a short last row centres itself.
No column count is written down anywhere.
Maximising the cell instead of the picture inside it picks a single long row every time.

A tile draws no ring, outline or dashed edge, in any state.
The edge belongs to the grid rather than to the stream inside it.
A stream that starts speaking, starts being shared or begins dropping frames therefore never repaints the boundary the reader navigates by.
One struggling stream never becomes the loudest thing on a screen someone is scanning.

What a stream is doing is said on its face, over the picture and at no cost to layout: a name plate with a small presence dot lower left, a status badge upper right, the figure lower right.
The badge is neutral except on this machine's own outgoing stream, which wears the filled red one.
A struggling stream wears the same neutral badge, names itself in words, and prints its drop count at a heavier weight.
The hues stay off the picture: a colour over arbitrary video is a colour against an unknown background, so a fault on a video surface is read rather than spotted.

Hiding a stream is a performance control, not a preference: it tears the decoder down, so the surface offering the toggle also reports the bandwidth and decode load that frees.

## Status language

One vocabulary everywhere, on a chip, a tile, the status bar and a button:

- Idle: a small static dot in the muted grey.
- Connecting: a spinning indicator.
  Where the transport reports connect phases, tiles add them as a step bar with a plain-words label.
  Otherwise the tile names the one thing it waits for.
- Live: the same small dot, and on the publishing surface the sharing pill: a solid white dot, the word `Sharing`, an elapsed timer in tabular figures.
- Degraded: the amber, the figure that is out of bounds, and the stream still running.
- Failed: the red, the reason in words, and a retry the surface already offers.
  Video tiles and the chips that drop them carry the reason in words alone and spend no colour on it ("Video surfaces").

The dot stays small (7px): state, not decoration.

**The reason in words is selectable wherever it lands**: a banner over a step, a hint under a field, a preflight row, a session log line, the sentence a dark tile carries.
It is the one string that has to leave the app for a bug report, a search box or a message to somebody else, and a caps negotiation or a relay address is not something anyone retypes off a screen.
Selectable text is a `SelectableTextBlock` carrying the role of the prose beside it, drawing its selection in the pair "Selection" states.
It wraps and never trims, an ellipsis eating the tail of the address the reader came for.
What decides is whether the string reports a failure, not how it is styled: a hint saying what a relay answered is error text, one explaining what a control does is not.

Two states are close enough in English to separate by name.
A stream is **live** when it is connected and frames are moving, true of every tile in the viewer and saying nothing about this machine.
This machine is **sharing** when it is the one sending, true at most once, and what the red is for.
`Live` and `IsLive` in the code mean the first, and the pill, the badge and the buttons mean the second.
Red is never spent on the first, however connected it is.

## Pressing

Every button answers a pointer with a surface: a fill on hover, a darker one under the press.
That holds for the flat variants, which carry no fill of their own and take one on hover rather than brightening their label and leaving the box empty.
A control whose only answer is a change of text colour is a label that happens to be clickable, and it is the one treatment this design does not allow.
The reader has to already know it is a control to discover that it is one.
The cursor does not stand in for the fill, saying a thing is pressable only once the pointer is on it.
A flat button therefore carries padding: the room the fill needs to read as a shape, and the press target the label alone would not give.

## Waiting

A control that starts a call the backend has to answer says so until it is answered.
Start sharing, Stop sharing, Measure, Look again, Open full log, a stream's grid toggle and its watch legs.
None is instant, and a control that answers a press by going quietly inert is read as broken.

One treatment everywhere: the label is replaced in place by a turning arc, in the control's own foreground, inside the box the label had.
The box does not resize, so a row does not reflow around the one that is working.
The arc turns at a constant rate and states no progress, nothing here knowing any: a control call is answered or it is not.
A waiting control is not pressable, a second press asking the backend for the same thing twice, so it also takes the unavailable treatment while the arc turns.
The arc tells the two apart: merely unavailable keeps its label and its reason stands in words nearby, waiting shows neither, the reason being that it is working.

## Selection

Selection is inversion: the selected thing takes the one light surface and near-black text.
Identical everywhere (a destination in the nav strip, a row in a list, the current step), so selection reads the same on every screen without a legend.
Toggling never rewrites a label: the state shows beside it or in the fill.
Every control that acts on state shows that state, so nothing asks the reader to remember what the last click did.

## Menus

A right-click menu, a submenu and a dropdown's option list are one object with one definition.
Whatever opened it, what opens is the same floating panel: panel radius, hairline edge, the one shadow a floating surface casts, and an inset keeping a row's fill clear of that edge.

A row is five columns and draws the ones it has: a glyph, the words, the key that does the same thing, the state, a submenu's chevron.
The glyph names the row and holds still while the row's state moves, so the shape a reader navigates by does not change under the pointer.
Dividers group the rows.
A menu whose rows are all peers makes the reader sort them on every open.

**A key is printed as a reading, not as part of the label.**
It sits at the quiet end in hint weight, answering a question the reader did not open the menu with.
The menu is where the shortcuts are documented, so a row that has one prints it and a reader learns it once.

**A row names a state, not a transition.**
"Mute", not "Unmute"; "Fullscreen", not "Leave fullscreen".
Whether that state is in force is the tick at the end of the row, the rule "Selection" states for everything else.
A menu is read far more often than pressed, and a label saying what pressing it would do never answers the question the reader opened it with.
It also makes a row idempotent to describe: the row names somewhere the thing can be, and pressing it twice is a round trip.

The tick is a read of that state and never a box the row keeps for itself.
A row that ticked itself on click would report the request instead of the answer, and would have to take the tick back wherever the backend refused.

**A row that lists rows is a row like the others.**
A dropdown offering entries this configuration rules out lists the ones that can be picked and closes with a row naming where the rest are, ticked while they are listed, counted at the quiet end.
It is the one row that leaves the menu open when pressed, what it changes being the menu the reader is standing in (`field-availability.md`, "Where a greyed entry sits").
A list of cards states the same under the cards, so what the press reveals arrives between the entries that can be picked and the control that revealed them.

A figure printed in a menu is not a row that refused to do anything.
Greying means an action some configuration took away, so spending it on a reading sends a reader looking for what would bring it back.
A figure is inert instead: quieter than a row, no fill under the pointer, out of the keyboard's path.

## Wording

Anything written as words for a reader is sentence case: headings, buttons, field labels, empty states, status lines, failure messages.
"Start sharing", "Force keyframe", "Edit in setup", "Read-only while live."
A standalone sentence takes a full stop.
A fragment labelling a state does not: "No frames arriving", "Waiting for the first frame".

Lowercase is reserved for figures, which are identifiers rather than prose: stat keys, chip values, table column headers, transport names.

**This is a screen-sharing app and it uses the words one has.**
A stream, a viewer, a window, watching, sharing.
Broadcast television has a term of art for most of these and every one loses to the plain word.
The test is whether a reader would have to be taught it: `program`, `on air`, `bug`, `lower third` and `take` all fail, and not one names something this product cannot already say.

`Sharing` is the state's name on every surface that has one: the pill in the nav strip, the pill on the broadcast header, the badge over the preview.
The controls that enter and leave it are `Start sharing` and `Stop sharing`, so button and state say one word rather than one saying `Go live` and the other answering `On air`.
The wizard's terminal step is `Share`.

A tooltip is prose: opens with a capital, closes with a full stop, and explains the control or figure instead of naming it again.
An icon button whose tooltip repeats its glyph teaches nothing, so the tooltip says what pressing it does and what it leaves alone.
A tip carries what the screen does not: a sentence already standing beside the control, an error or the reason it is inert, is read where it stands and gets no tip repeating it.

A figure keeps one name across surfaces: `transport`, `resolution`, `codec`, `bitrate`, `decoder`, `fps`, `frames`, `latency`, `rtt`, `loss`, `dropped`, `via`, and `n watching` for the number of open tiles.
A name is retired rather than reused when nothing reports it: a viewer's own buffer fill reaches no publisher, so that column states what the relay discarded on the way out (`field-availability.md`, "A figure with no measurement").
A surface with more to report adds rows instead of renaming the shared ones.
Stat rows spell their words out, join two figures with ` · `, and print `…` where there is no value yet.
Transport names stay lowercase, as the settings offer them: `hls`, `moq`, `rtmp`, `rtsp`, `srt`, `webrtc`.
On a viewer surface a bare `transport` means the watch leg.
A label for the publisher-to-relay leg says "publish".

## Ownership

A control appears in **exactly one** window.
The window that owns a value edits it.
Any other showing the same value shows it read-only, with one link back to the owner.
A second editor for one setting is the defect this prevents.

Setup owns the configuration of what this machine sends.
Broadcast owns the live-safe actions, the ones that change the stream without tearing it down, and shows configuration read-only with one `Edit in setup` link.
Viewer owns how this machine receives: the watch settings, saved from a panel beside the grid, and the per-tile controls over a running decode.
They sit with the tiles because a reader who only watches never opens setup, and a draft setup holds is persisted by going live (`viewer-architecture.md`, "Two legs, two protocols").

## Icons

Outline glyphs, vector paths at a 1.2px stroke with round caps.
Sizes range 12 to 22px by surface.
Emoji are never used: the platform emoji font paints them in colour and ignores the foreground brush, so a button's states become inexpressible.
Window controls are geometry rather than text, for the same reason a font's box-drawing metrics cannot be relied on: on Windows the caption glyphs live in Segoe Fluent Icons' private use area, so a missing face paints a tofu box where the close button was.

The shell uses the Tabler outline set, through `TablerIcons.Avalonia`.
Platform icon themes go unused, so every surface shows the same glyphs.
No surface draws its own path: a hand-written tick or chevron would be a fourth icon set nobody maintains, and would miss the stroke rule the first time it was resized.

The window controls are the one exception and are not on the ladder above.
They stand in for the caption buttons the platform hid when the client area was extended over them, so they are the platform's shapes at the platform's measurements.
On Windows a 10px box at a 1px line with mitred corners (minimise, maximise, restore, close), the middle button drawing restore while the window is maximised.
A reader compares them with every other window on the screen rather than with the app underneath, so an app-set glyph at the app's stroke is wrong there even when it is the same three lines.
They are absent on Linux rather than redrawn: the comparison has no fixed answer there, and the desktop that does have one is drawing the frame itself.

## Motion

Subtle and short.
Hover and opacity transitions around 200ms, video fade-in 500ms, tiles mount with a fade and slight zoom.

One named animation: `spin`, a 1s linear rotation, which turns the arc a waiting control wears.

## Empty states

A dashed rounded border, a circular muted badge holding an outline icon, one muted sentence.
No heading, no button.

A step or panel with nothing to show says so plainly rather than inventing content.
A figure the app has not measured prints `…`.
It never prints zero.
