# avalonia

The Avalonia app, and the only desktop surface there is. The Wails app and the GTK4 grid
it launched were both deleted rather than kept in step (`docs/viewer-architecture.md`), so
this is the whole surface - the settings form, the encoder and transport pickers, the
live-now list and the tile grid all live here.

It started as one vertical slice, a port of `internal/relay` that polled the relay's HTTP
API itself. That slice is gone, and its deletion is worth stating because it is the rule
this module runs on: **the shell reads, it does not find out.** Whether the relay is up is
a state the backend polls and announces (`docs/ipc-api.md`), so a second poller here was a
second answer to the same question - and for a while it was the only one running, since
nothing on the backend polled at all and every screen read a snapshot that had never been
taken. What is left is one relay reading in the whole app, on `Backend/Session.cs`.

## Running it

```sh
task avalonia          # run it
task avalonia:build    # build into build/bin/avalonia
task avalonia:test     # the view-model suite, no relay and no backend needed
```

`task relay` first, or the app renders its failure state, which is also worth looking at.

The setup flow needs the Go backend running, since everything on it is resolved there. The app
starts one itself when nothing is listening on the control endpoint, so `task avalonia` is the
whole of what a reader has to run; a backend already up - a `task dev` run, a second window -
is connected to rather than duplicated, and one this app started is stopped when the window
closes. Where there is no backend binary to start, the app says so and offers to look again,
which is the other state worth looking at.

### Which windowing backend runs

Wayland where the session has a compositor, X11 where it does not, decided at startup by
`UseWaylandWithFallback` (`Program.cs`). `Avalonia.Desktop` carries the X11 backend alone, so
the Wayland one is a package reference of its own and the flake's dev shell carries the
libraries it resolves by soname.

Which one runs decides how the window looks on a scaled desktop. An X11 client in a Wayland
session goes through XWayland, and a compositor that scales XWayland hands the client the
logical size and magnifies what it drew: a soft window beside sharp native ones, and nothing
the app can correct after the fact. The Wayland client draws at the output's own scale, and
follows a window moved between outputs of different scales.

Two things the X11 backend does that the Wayland backend does not. It sets `WM_CLASS`, where
the Wayland one sends no `xdg_toplevel.set_app_id`, so a compositor sees an empty application
id and window rules keyed on it do not match. And it draws the client-side decorations a
desktop without server-side ones expects; on Wayland the same case is negotiated through
`zxdg_toplevel_decoration_v1`, and a compositor that answers "server side" draws whatever it
draws, which on a tiling session is nothing (`Features/Shell/Model/WindowChrome.cs`).

### What a capture of this machine sees

Every window this shell opens asks to be kept out of the captures this machine takes, and the
window states it for itself when it opens (`Features/Shell/Model/CaptureExclusion.cs`).
The main window and each popped-out stream are the two.

The reason is a feedback loop, and it closes in the captured pixels rather than anywhere this
code can reach.
A tile draws a decode of the screen it is itself drawn on, so the next capture carries the tile,
which carries the capture taken before it.
Every round trip through the encoder and the decoder nests one more copy of the screen inside
the picture, and the depth grows with how long the stream has been running.
Nothing downstream can undo it, because the nesting is already in the frames the capture handed
over.

Windows stores a display affinity against the window in kernel mode, and `WDA_EXCLUDEFROMCAPTURE`
leaves the window on the monitor and takes it out of what the desktop window manager composes for
anything else.
That covers every Windows capture backend here: the desktop duplication and graphics capture
behind `ddagrab` and `d3d11screencapturesrc`, and the screen bit blit behind `gdigrab`.
Windows before 10 version 2004 applies `WDA_MONITOR` in its place, which leaves an empty
rectangle where the window is.
The two pictures differ, and both break the loop: an empty rectangle carries no copy of the
screen either.

Linux is left as it is, so a tile there goes on nesting for as long as the stream runs.
X11 serves the root window to any client that asks, so a capture is a read of the whole screen
and there is no per-window fact to set.
No Wayland protocol carries the request.
The compositors that can black a window out of a screencopy, Hyprland and niri among them, do it
from a window rule in their own configuration, which is the user's to write rather than this
app's to ask for.
Such a rule is keyed on the application id, and the Wayland backend sends none, so it matches
this window in an X11 session alone.

## Layout

Four layers, and the direction of dependency runs one way through them: a feature reads the
design system and the controls, and neither of those has ever heard of a feature.

| Path | Holds |
| --- | --- |
| `Contracts/Assert.cs` | always-on assertions, the C# counterpart of the Go `assert` package |
| `Mvvm/` | `Observable` and `DelegateCommand`: the change notification a compiled binding reads, and nothing else |
| `Design/` | the whole design system as tokens and styles - `Palette`, `Typography`, `Metrics`, `Text`, `Surfaces`, `Buttons`, `Inputs`, `Menus`, `Tooltips`, `Icons` |
| `Controls/` | the primitives more than one feature needs: `Chip`, `StatusPill`, `CheckItem`, the segmented control, the switch |
| `Copy/` | every word on screen: what each identifier the backend sends is called, the paragraph behind each choice, each control's heading and help, and the sentence for each statement the backend makes |
| `Features/Shell/` | the window, the title bar, the shared nav strip, the status band, and which destination is showing |
| `Backend/` | the control-plane seam: `IBackend`, the gRPC client that answers it over the local socket, and the settings write that goes through the message descriptor |
| `Features/Fields/` | the generic renderer for one group of the resolved form, and the placement table saying which destination draws which group. It is not under a feature because two of them draw form groups |
| `Features/Setup/` | the publish wizard, one step per group of the resolved form that is about sending, plus a terminal one: the step strip, the screen picker, the Quality form, the raw-property drawer, the cost rail, the review, and the saved presets beside it |
| `Features/Broadcast/` | the live overview: the promoted figures, the live-safe actions, read-only configuration, the outgoing preview, the per-viewer table, the sparklines |
| `Features/Viewer/` | the tile grid and the rail beside it: one entry per stream the relay carries, the arrangement of the ones being watched, and the panel holding the settings that govern how this machine receives |

### The two rules the tree encodes

**A slice per component, and MVVM inside it.** A feature directory holds `Model/`,
`ViewModel/` and `View/` for the feature itself, and a further directory per component that
is substantial enough to have its own three. `Features/Setup/QualityStep/ViewModel/QualityStepViewModel.cs`
is the shape. Namespaces mirror the path exactly, so a file's name says where it sits.

**Nothing outside `Design/` states a colour, a size, a font or a radius.** A component asks
for the role it wants - `MutedBrush`, `RadiusPanel`, `FontSizeLabel` - and the palette
decides what that is. That is what keeps a light variant a second dictionary rather than a
sweep through eighty files, and it is why the greyscale ramp is listed once and then never
named again.

### The design language

Greyscale everywhere except a single red, `#E5484D`, reserved strictly for "sharing" and
"something is wrong". It is the only hue on any screen and the only one a colour-blind
reader still separates from grey reliably, so spending it on state that is merely on would
cost the app its one unmistakable signal.

One family and two weights carry everything: Inter, bundled with the app rather than named
and hoped for. What a second family would have bought is column alignment in a run of
digits, and that is a font feature rather than a face: anything that ticks, counts or sits
in a column is set in tabular figures (`FigureFeatures`, Inter's `tnum`), so a line that
mixes prose and figures no longer changes shape halfway through. What is machine-generated
is said by role instead, one step quieter and one step smaller than the prose beside it
(`docs/design-language.md`). Sizes are whole pixels; the half-pixel steps the mockups were
written in are gone, because 11.5px and 11px land on the same pixel grid.

There is **one button and one of every input**, and both tables are keyed on the type rather
than on a name: `Design/Buttons.axaml` skins `Button` itself and `Design/Inputs.axaml` skins
`TextBox`, `NumericUpDown` and `Slider`, so a view that writes a bare control gets the design
without asking for it. A named theme is only ever a variant - `ActionButton`, `FooterButton`,
`DangerButton`, `PrimaryButton`, `LinkButton`, `CardButton` and the `OptionCard` built on it -
and every one of them inherits the base and states only its difference. A variant that
restates the template is a second button, and two buttons drift: that is exactly how setup's
"Look again" ended up wearing Fluent while the control beside it wore the design. The inputs
have one variant on the same terms - `Controls/NumberSelect`, the number box and the button
glued into the one control that is both, each inheriting its type's theme and setting only
the corner that differs. A flag is the switch in `Controls/Toggle`, never a `CheckBox`, for
the same reason.

There is **one menu**, on the same terms. `Design/Menus.axaml` skins `MenuFlyoutPresenter`,
`MenuItem` and `Separator` themselves, so a right-click menu, a submenu and a dropdown's option
list are one surface and one row wherever they open. `SelectMenuItem` in `Controls/Select` is
the only variant, and it adds the two setters that make a row an option - what picking it does,
and whether it can be picked - over the base. The type-keyed form is what fixes the failure
that produced this file: nothing had restyled `MenuItem`, so a view had to opt into a menu's
look by name, the two right-click menus never did, and both wore Fluent inside a product whose
design states that it has no Fluent in it.

Every icon is a Tabler outline icon from the `TablerIcons.Avalonia` package, and `Design/Icons.axaml`
is the single rule that sizes and strokes them: three sizes off the design's 12-22px range,
one 1.2px stroke, one brush role. Nothing here draws a path and nothing here uses a character
as a glyph - a `✓`, a `⌄` or a hand-written close cross is the platform text face rather than
the icon set, so it lands at a different weight beside a real icon and cannot be restated at
another size.

`docs/design-language.md` states these rules for the whole product, and this module is its
reference implementation: `Design/` is where the numbers live. The two surfaces that carried
the zinc-and-emerald language this replaced are deleted, so there is nothing left to port.

`ScreenShare.App` is the shell rather than a viewer, because the publish side lands here
too. The settings form is now the `Setup` slice, which was the surface with the most state
and the most cross-field rules from `docs/domain-model.md` - the one that said most about
whether this shape holds.

### This module is a display

It decides nothing. Every value a control offers, every label on it, every greyed entry and
the sentence explaining the greying, every warning and every derived figure comes from the
Go backend over the control API. This module contributes layout, typography, colour, motion,
input handling and accessibility, and that is its whole job.

`docs/ipc-api.md` states the rule and the reasoning; `api/proto/screenshare/v1` is the
contract. The message to read first is `Form`: `ResolveForm` takes a settings draft and
returns the complete description of the screen, and a view model renders it rather than
evaluating it.

The practical consequence for anything written here: a `switch` over a codec name, a list of
rate-control modes, a hardcoded resolution ladder or a hand-written tooltip is a defect, in
the same way a view field mirroring a model field is. The seeded values described below are
the one exception, and they are exceptions with an expiry date - the setup flow's have already
expired, and its screen is drawn from `ResolveForm`.

The shape that consequence takes in the wizard is worth stating, because it is the whole
argument in one place. Every step but two is **one component**, `Features/Fields/`,
instanced once per group. They differ in nothing this module can see: each is a `FieldGroup`
of the resolved form, a run of fields with different keys, and the renderer switches on
`ControlKind` rather than on what the field means. A capture view and an encode view written
separately would be this module writing down what a capture and an encode are.

**Which steps there are is the form's answer too.** They used to be a table of seven rows here,
each naming the group key it drew - and three of those keys named groups the backend does not
answer with, so three steps of the wizard drew an empty column and four real groups were
unreachable. Nothing said so, and the tests passed, because the fixture had been written
against the same table. `SetupSteps.For` derives the strip from `Form.groups` instead: a group
added to the contract is a step that appears and works with nothing here to edit, and one
renamed cannot leave a hole. What is still this module's is placement - the terminal step, the
one group drawn by a layout of its own (`Model/QualityLayout.cs`), and which destination draws
which group at all (`Features/Fields/Model/GroupPlacement.cs`).

**The watch group is drawn by the viewer, and that is the same placement rule doing real work.**
The wizard configures what this machine *sends*; the watch group is the legs a stream comes back
on, the jitter buffers a receiver holds and the chain a tile converts frames with. It was a step
of the wizard, so a reader who only watched had to open the broadcast setup to change how their
tiles decode - and the change only ever persisted if they went live, because the wizard's draft
reaches the backend through `StartPublish`. The group is beside the tiles now, in a column with a
commit of its own and a button to dismiss it.

**The group is staged, so nothing in it reaches a decode until the commit does.** Every knob a
receive pipeline reads is read by the backend out of its own settings as the pipeline is built,
and the one value the shell names in the call is the tile's leg, which is read out of those same
stored settings (`Features/Viewer/Tile/Model/TileLeg.cs`). Taking the leg off the draft instead
made half the panel take effect as it was edited and held the other half back, so a decode could
open on an unkept protocol with kept buffers. `FormSession.Stored` is what both now read.

That is also why the panel says when what it shows is not what is stored. A staged group draws
the same controls whether or not it has been kept, so without the sentence the button has nothing
to mean and a repaired value nobody wrote back is invisible. A commit that lands closes the
column, because what the reader asked for has happened; one the backend refuses leaves it open,
because the sentence explaining the refusal is on it.

**The commit goes down the same queue an applied field's keystroke does** (`FormSession.SaveAsync`).
The settings travel whole, so the two are writes of one message, and sent from two places they
are two unary calls with no ordering between them - the older snapshot landing last is a stored
setting the reader had already changed.

**Both screens read one draft.** `Backend/FormSession.cs` owns the settings being edited and the
form they resolve to, for the whole window, exactly as `Backend/Session.cs` owns the running
state. A draft per screen would be two copies of one message, and the publish commit persists
the whole message - so whichever screen committed would overwrite the other's half.

Two things keep the rest honest. A field's key is a settings group and a field in it, so a write
goes through the message descriptor (`Backend/SettingsDraft.cs`) and a field added to the
contract is a control that appears and works with nothing here to edit. And the form's answer
is adopted whole: `FormSession` replaces its draft with the one `ResolveForm` returned rather
than merging, which is what keeps a greyed option and its replacement from disagreeing.

`Backend/ControlBackend.cs` is what answers `IBackend`: a gRPC client over the named pipe on
Windows and the Unix socket elsewhere (`Backend/ControlEndpoint.cs`). It names no codec, no
encoder family and no rule - a greyed option arrives greyed, carrying the sentence that says
why - and the stand-in it replaced now lives in the test project, where a hand-written form is
a fixture rather than a second copy of the domain.

Two things about it are worth knowing before reading the class.

**The encoder probe is asked for once, in the background.** `ResolveForm` reads what has been
probed rather than probing, because a resolve is called on every keystroke and the probe
test-encodes on every engine. So the first forms of a session grey no codec for missing
hardware - which is the honest reading of a fact nobody has established, and also a dropdown
still offering Quick Sync on a machine with no Intel GPU. The client asks for the probe behind
the handshake, and raises `IBackend.Changed` when it lands; the setup flow re-reads, and the
codecs this machine cannot run come back greyed with what is missing. Nothing about which
codecs those are reaches this module.

**A backend that is not running is a sentence, not a gap.** The reads throw
`BackendUnavailableException`, the flow shows its message above the steps with a "look again"
button, and no form is invented in the meantime. There is still no timer behind that button,
so an absent socket is not hammered for as long as the window is open.

**A backend that comes back is noticed, though, and that is not the same thing as a timer.**
The session already dials the backend every couple of seconds - it has to, since the event
stream ends when the backend goes away - so the news that it answered is in the window
whether or not anything asked. The flow reads that transition and asks again once per
recovery, which is the case nearly every start meets: the app launches the backend and
reaches it a moment later, so the flow's opening read is the one call that fails. It used to
leave the reader looking at a sentence about an absent backend, beside a viewer screen that
had quietly filled in. What the button is still for is the failure nothing else reports - a
read the backend served a refusal to.

That state is narrower than it was, and deliberately not gone. `ControlEndpoint` starts a backend
when the endpoint refuses a connection and asks again until it binds (`BackendProcess`), because
the backend is a headless binary and a reader who opened the app asked for both halves of it.
What is left is the case the shell cannot act on: no binary beside the app or on `PATH`, or an
operating system that refused to run it. The sentence is the same one either way - nothing is
listening on this endpoint - because that is what is true whether a start was attempted or not.

**Which sentence it is depends on who wrote the status, not on which code it carries.** A
status the backend served carries prose written for a person and is shown as it arrived: the
contract's table gives `UNAVAILABLE` to a relay that could not be reached and to a child
process that would not start, so reading that code as absence answered a press of Start
sharing with a sentence about the endpoint the shell had just resolved a form through. What
says the backend is absent is `Status.DebugException`, which the client library sets on a
status it made from a local failure and leaves null on one that arrived - told apart by code
rather than by matching on a sentence, which is the input that changes without anything
failing to compile.

That is the transport. What the flow finally does with it is the commit, and that is worth its
own paragraph because it is the one control on this surface that changes the world.

**The commit is real, and it reads four states rather than one.** It sends the draft to
`StartPublish`, which persists it and starts the encoder on it; the reply says nothing and the
stream that resulted arrives on the event stream, so the window that pressed the button and the
window that did not learn it the same way. What decides it is
`Features/Setup/Model/PublishGate.cs`, and every condition in it is a whole state some other
side stated: `Form.publishable` for the settings, the backend's own sentence when it cannot
describe them at all, `RelayStatus.reachable` for somewhere to send to, and the presence of
`PublishState.live` for a stream already on the air. None of them is ranked or re-decided here,
and only one sentence is shown - a reader fixes them in that order anyway. A settings problem
gets no sentence of its own at all, because the preflight card beside the button already carries
every one of them in the backend's words; paraphrasing a diagnostic would be this module writing
a rule down twice.

**The fourth of those is not a lock, and the difference is the whole shape of that file.** A
stream already on the air used to refuse the commit and send the reader to the broadcast screen
to stop it, because the only effect this shell could reach was `StartPublish` and the backend
refuses that while a pipeline is in force. `ApplyToStream` is the effect for exactly that state,
so a live stream now decides *what the commit does* rather than whether it can be done, and the
gate says which of the two as data (`PublishGate.Commit`) instead of leaving the label, the
sentence and the call to look at the publish state again and each reach their own answer. The
press reads that state once more on its own pass rather than trusting the gate the last render
composed: a stream can start or end in between, and the backend refuses each of the two effects
in precisely the state the other one is for.

**The word on the button says which it is, and says what applying costs.** `Model/CommitCopy.cs`
is one row per commit - the label, and the two halves of the sentence the stream name sits in -
read by the view model and bound whole, rather than a ternary at the binding site. The apply row
says in plain words that the stream restarts, because it does: both engines run a child built
from an argv, so new settings tear the pipeline down and launch another, and every viewer loses
the picture across the gap. That is the same fact the broadcast screen's quality track is greyed
and carrying (`Features/Broadcast/Nudge`), and a button that promised a seamless change would be
the one place in the app that lied about it.

The relay half is worth stating, because it is the one state the shell reads from a poll it does
not run: the backend polls the relay for as long as it is up, records each snapshot and answers
`GetRelayStatus` from it, and its opening value is unreachable with no reason - which is the
honest reading of a relay nobody has asked yet, and which the gate says as much about.

That opening value is also where this went wrong once, and the shape of the bug is worth keeping.
Nothing on the backend polled: the fetch happened inside a method the deleted frontend called on
a timer, so with that surface gone the recorded snapshot stayed at the opening value forever and
every shell drew "the relay could not be reached" beside a relay that was up. The contract had
said the backend polls all along. The lesson is not "poll harder" - a second poller on this side
would have hidden it - it is that the side the contract names as the owner has to actually do the
owning, and the honest opening value is what makes the difference visible rather than plausible.

Where the window goes afterwards is the window's. The flow raises `WentLive`, the shell records
that it owes the reader the broadcast screen, and it moves on the pass where the stream is
actually in force - a start that was accepted is not yet a stream, and navigating on the reply
would be the window claiming a state the backend has not reported.
Every start earns the move: a stream this window started is what the window then shows, so the
destination a commit leads to is not a setting the review asks about.

### What is seeded rather than real

The setup flow is real: it is resolved by the Go backend end to end, and its commit starts a real
stream.

**The broadcast screen is real too now, and what it cannot measure it says so about.** Every
figure on it is composed in `Features/Broadcast/Model/BroadcastSnapshot.cs` out of three whole
states the backend sent - the publish state, the newest encoder sample and the relay snapshot -
and a figure with no source prints an ellipsis rather than a zero.

**The viewer table is a row per viewer, and the relay measures them one leg at a time.** The
backend reads the relay's reader array per path and joins each entry to the per-protocol
connection list its type names, so a row is an address, a join time, and whatever that leg is
instrumented for (`internal/relay/readers.go`). SRT is the one the relay times a round trip and
states a loss rate on; the rest report what was sent to them and what the relay's own queue had
to discard. A cell with no measurement behind it is an ellipsis, so a viewer over RTMP reads as
untimed and never as a viewer with a perfect link - which is also why the severity rule in
`Features/Broadcast/Model/ViewerRow.cs` reads presence rather than value, and why the header
promotes the *worst* viewer's round trip and loss under a label that says so. Two of the design's
columns named figures nobody reports to a publisher, buffer fill and the decoder in use, and they
carry what the relay does measure at that width instead: what was dropped, and the leg it went
out over.

**The session log names who arrived and who left, and nothing announced either.**
There is no arrival event and no departure event on the contract: the relay reports who is
connected at each poll and says nothing about who stopped being.
So the audience lines are the difference between two consecutive rosters in the snapshot series
the session holds (`Features/Broadcast/Model/Audience.cs`), derived on every render pass rather
than accumulated as viewers come and go.
An arrival carries the relay's own join time, a departure the arrival time of the poll that first
did not name the reader, and a poll that named no path for this stream contributes nothing at
all - reading an unreachable relay as an empty roster would log every viewer leaving each time it
hiccupped.

What is still absent is the congestion band. The relay states its figures as they stand at each
poll and marks no interval, so a window shaded on the latency plot would be a detection this side
performed and attributed to the backend.

What went with the seeds is worth listing, because each was a mockup number that read as a
measurement: the sharing pill's timer, which stood at `00:42:18` in every window whatever was
publishing and is now the encoder's own clock, read back off the broadcast screen so the pill in
the chrome and the pill in the header cannot disagree; the sparkline's `60 s` window labels,
which named a span the plot did not cover and now name the axis the plot is drawn on - both
series are stamped, so a point is placed by when it was taken against a fixed span ending at the
newest reading, and a run younger than that span fills the right of the card instead of being
stretched over it; the dashed red congestion
band, drawn at a fixed quarter of the way across a plot with nothing in it, for a condition
nothing detects; the `vbv ceiling` rule, drawn at a constant third of the height while the curve
is scaled to the run's own peak, so it marked the ceiling only by coincidence and now is placed
against that peak or not drawn at all; and the viewer table's `every 5 s`, a period the contract
does not carry and which the backend did not use.

**The preview tile draws a frame, by one of two routes the reader picks between.** On the
local route the publish child copies its already-encoded video to a loopback port beside the
sink that feeds the relay, the backend decodes what arrives there, and this card subscribes to
it. On the end-to-end route the card opens a decode of this machine's own stream off the relay,
over the leg the viewer receives on, so the picture crosses the uplink, the relay and the way
back (`docs/viewer-architecture.md`, "What the broadcast preview draws"). Both reuse the same
`Features/Viewer/Tile` view model and control the viewer's grid uses rather than growing a
second frame consumer - two frame paths would be two answers to what a dropped frame is and
where a lent handle goes back. What differs between the two is `Tile/Model/TileSource.cs`,
which is the contract's own oneof: a relay decode named by stream and leg, or the running
publish's preview named by nothing at all.

**The two routes cost opposite things, which is why the card offers a choice rather than
picking.** The local pipeline goes up with the publish child and down with it, so that route
sends no `StartReceive` and closes no decode; `PublishState.Live.preview` is what says whether
there is a picture to draw, read through on every pass like every other state. The end-to-end
route is a relay client like any tile in the grid: it takes a reader slot, it is counted among
the viewer figures beside the card, and it pays a viewer's downstream bandwidth. That is the
whole reason the card opens on the local route - the preview used to be a relay client and
nothing else, so a stream nobody was watching reported a viewer and the worst-viewer plot
described the publisher's own round trip.

A decode is one pipeline whoever asked for it, so the end-to-end route can be sharing one with
a tile in the viewer's grid. It reads the grid's answer through before closing anything and
leaves the pipeline to the window that still wants it, and it asks again for one it saw running
and no longer sees.

Two things about it are still the shell's own arrangement. Whether the card draws is the
reader's, written by the transport control over the picture and by nothing else; the card opens
drawing. It does not follow the window, which is the one place this card and the wizard's screen
picker part company: a publisher's window stands behind the thing being shared for most of a
session, so a card that stopped whenever nobody was looking at it would be dark at the moment a
reader came back to check on it, and would pay a pool import and a reconnect to come back. What
the stop is worth is the end-to-end route's reader slot, which is why it closes the decode
rather than merely clearing the tile. And the placeholder stays for the states where there is no
picture - stopped, a leg that refused the decode, nothing publishing, a route with nothing
running behind it, and the tile's own three - each of them saying which one it is.

The card's own sentence is the selected route's, because the two make opposite claims and one
sentence for both would be false under one of them. The local route says it costs one decode
and no bandwidth, adds no viewer to the counts, and is taken *before* the relay; the end-to-end
route says it is what a viewer receives and that it is paying for it as a viewer does. A reader
who took a perfect local preview for a healthy stream would be reading it exactly wrong, which
is why the sentence is on the card and not in a comment (`Copy/Cards.cs`).

**The review carries the preset card, and it draws two different things under one heading.**
Above are the built-in presets, which are promises about the picture: what "gaming" is on this
machine is a search the backend ran over its own capability tables, so a row can be unreachable
and what applying it writes differs from machine to machine (`docs/presets.md`).
Below are the saved ones, and a saved preset is a name.
That is what the switch this card replaced could not be: "Save as preset" was a flag, and a
preset is kept, replaced and deleted by the name it is under, so the control was inert and would
have stayed inert whatever it was wired to.
The card is therefore a row per promise, a row per saved preset, and a name box
(`Features/Setup/Presets/`).
It sits on the review because a preset is the whole way of publishing and the review is where the
whole way of publishing is read back - the steps each own a fraction of it.

Two things about it follow from the contract rather than from the layout.

A preset is a `PublishSettings` and nothing else, so applying one replaces that group of the draft
and leaves the relay and the watch settings where they are (`docs/presets.md`).
Nothing is committed by it: publish settings are staged until a commit carries them, so trying a
preset out costs nothing and puts nothing on the air.

The store is the one state on this seam that no event announces.
Presets are a file the backend does not run on, so a save or a delete is followed by a read rather
than by patching the list with what was just sent, and the re-read is offered as a button - a
preset another window saved is invisible here until someone asks again.
The built-in half needs none of that: it arrives on the form, so it is as current as everything
else the resolve answered with, and applying one reads the settings off the form the window holds
now rather than off the row that was rendered - a promise resolves against the draft, so what is
behind a key moves as the draft does.

Which row is marked as the one in force is derived on every pass, and the two halves derive it
differently. A saved preset is marked while the draft equals it field for field, because a
snapshot says every field. A built-in one is marked while the draft delivers its promise, which
the backend states on the form, so a field the promise says nothing about can move without taking
the mark off. Neither is a stored selection, so there is nothing to reconcile after a restart.

**The viewer is a grid over a roster.** The grid draws the streams the reader asked to see,
from the GPU memory the backend decoded them into: a row's `show` toggle opens a decode
through `StartReceive` and the tile subscribes to its frames, each frame arriving as a slot
of a lent pool that goes back only once the compositor has taken it
(`docs/viewer-architecture.md`, "The buffer-ownership protocol"). Nothing about the
arrangement crosses the contract and nothing could: the backend describes decodes, and a
decode is not a tile.

**Nothing is measured over the picture.** A tile draws its name on hover and a colour badge
where the range needs one, and every figure about the decode is in the stats panel the tile's
menu opens. The strip of figures that used to sit in the bottom corner said a handful of the
same things, in a space too small to say what any of them meant, over the part of the picture
a reader is watching.

The panel is composed from two readings that come from opposite directions. What the decode is
doing - what is arriving, what came out of the decoder, what the sink took and threw away, the
counters the transport's own elements keep - is `ReceiveStats`, a sample the backend reads off
the running pipeline once a second and pushes, exactly as it pushes the encoder's progress
(`docs/viewer-architecture.md`, "What a tile reports"). What this window got and drew is the
tile's own and can come from nowhere else: a backend cannot see that a compositor was too slow
to take a frame, so the dropped count is the one figure the shell reports rather than receives.

Every row of it carries a tooltip saying what a reading of it is evidence of, which is what
separates a diagnostic from a wall of numbers. The rows are keyed on the identifiers the two
sides share - the contract's own field names, and the elements' own names for their counters -
and every word is in `Copy/Counters.cs`, where a key with no entry renders as the key
(`docs/tooltips.md`). The panel is composed only while it is up, so a grid of tiles nobody has
opened one on builds nothing.

The `show` toggle is one control and not one per leg, unlike the roster's. Which protocol a
tile receives on is `viewer.tile_watch_transport`, a setting the backend resolves and
repairs; offering it per row would be this screen deciding something the settings screen
already decides.

**The roster underneath is unchanged.** Every row comes off
`GetRelayStatus` and `GetViewerState`: which streams the relay carries, whether each is being
served, what it says they carry, how many readers each has, what each is ingesting, and which
legs this machine already has a viewer open on. The legs a row offers are the options of the
form's watch-leg field, so this module holds no list of protocols, and each arrives carrying
whether it is reachable and the sentence that says why not. A leg the availability pass ruled
out keeps its place, greys and draws its reason under its name, the treatment every other
option in the product gets (`docs/field-availability.md`). The reason is in the row rather than
in a tip, because a disabled control in Avalonia takes no pointer and a tip on it never opens.

**Two different facts, and only one of them is greyed.** Whether this machine has a player for
a protocol at all is the availability pass's answer and it does not go stale, so the row obeys
it. Whether a *given stream* can travel on a leg is answered against the stream when the viewer
is opened, and the relay's snapshot can be older than the stream - greying from that would
refuse a viewer that would have worked, so the backend refuses with the format named and the row
shows that sentence instead. A leg already open stays pressable either way, because the press is
what closes it.

**The spotlight, the per-tile menu, the chip row and the pop-out windows are still gone.**
They drew mockup figures beside real ones, and the thing they were mockups *of* needed
frames. The grid that came back was designed against a real decode path rather than against
a seed, which is why it is one tile shape with measured figures and none of the four. Nothing
in the contract describes any of it either, and that is the point: how a viewer arranges what
it receives is this module's job, so the backend describes no grid to open, no tiles to
report and no layout to pick (`docs/ipc-api.md`).

The keys are the tile's own rather than the window's, and each names a state the menu names:
`F`, `O` and `P` for filling a screen, focused and in a window of its own, `M` for silenced,
`S` for the stats overlay, and `+` and `-` for the level the decode plays at.
A shortcut on the window would have to invent a rule for which tile it meant, and each candidate
for that rule (the focused tile, the last one touched) is a second arrangement state to keep.
Hanging them off the tile makes the pointer the rule instead, so each card listens for keys on
the window it is drawn in and answers only while the pointer is over it
(`Features/Viewer/Tile/View/TileKeys.cs`).
The card never takes the keyboard, because taking it on hover would take it out of whatever the
reader was typing in.
The keys are one table, read by the press and by the gesture the menu row prints, since a menu
that named a key nothing acted on would be wrong the moment either half moved.
A press asks the command whether it can run before it runs it, so a key is refused wherever the
row it names is greyed: a stream with no sound track has nothing for `M` or the volume keys to
do, and the press is left for whatever else wants it.

## How the repository's principles land in C#

`docs/development-principles.md` governs this module too. Three of its four rules
translate directly; the fourth needed a decision.

**State has one owner.** `Backend/Session.cs` owns the running state - what is publishing,
what the encoder is measuring, what the relay is carrying, which viewers are open - and the
screens read it through on every pass and keep no copy of what they found. The relay is the
clearest case of why: setup's commit gate and the viewer's roster describe the same relay,
and they cannot disagree about it because they read one field.

**One render function.** `Apply` is it. It sets every output property on every pass,
including the branches that turn something off, which is what makes a recovered relay
clear the notice a failure left behind. Nothing else writes those properties - they have
private setters and the compiler enforces it. The reader's inputs are the other half of the
split: their setters are the named writes, and `Apply` never touches them, because a render
pass that reassigned a text box would fight whoever is typing in it.

**Idempotency.** `Apply` twice over unchanged state raises no notification: the property
setters compare first, and a bound collection is rebuilt only when the rendered rows
differ. Every row type is a record for exactly that reason, so two passes over one state
produce values that compare equal rather than merely look alike.

**Assertions.** `Contracts/Assert.cs` throws in Release as well as Debug, which
`System.Diagnostics.Debug.Assert` does not - a contract that only holds in Debug is not a
contract. Message style follows the Go one: a present-tense sentence naming the invariant,
offending values in the trailing arguments.

**A round trip does not get to bend any of them.** `IBackend` is asynchronous because the
thing behind it is a socket, and a render pass that awaited one would be a window that
stops painting. The split that avoids it is the first rule applied literally: the last
form the backend answered with is state `SetupViewModel` holds, `Apply` reads it and
returns, and a draft change starts a resolve whose answer lands on a later pass. A flow
with no form yet is a state rather than a gap - every group renders its unresolved branch,
the same one a step the form does not carry renders.

Two guards keep that honest, and both are the third rule. Asking is skipped entirely while
the draft still equals the one the backend was last handed, so `Apply` costs one round
trip however many times it runs - which matters, because `ShellViewModel.Apply` renders
every destination on every pass. And the latest answer wins: each resolve cancels the one
before it and carries a request number, so an older draft's form arriving late is dropped
rather than drawn. Cancellation alone would not do it - it is cooperative, and a call can
already hold its answer when the token is set.

The answer arrives on whichever thread the transport completed on, so the write back is
marshalled through an injected dispatcher rather than a toolkit reached for in place -
`Dispatcher.UIThread.Post` in the window, a straight-through call in a test. `Session` is
handed the same one, for the same reason.

**And a round trip a reader started is a state, so it has one owner too.** Every control that
asks the backend for something - Start sharing, Stop sharing, Measure, Look again, Open full
log, a stream's grid toggle and each of its watch legs - is a `Mvvm/PendingCommand.cs`: it
holds whether the call it started is still out, refuses a second press off that same field,
and clears it in a `finally` so a call that failed past whatever the effect handles still
gives the control back.
The view draws the wait from the identical field through `Controls/Pending/Pending.cs`, which
is an attached property setting one pseudo-class, and `Design/Pending.axaml` says what that
looks like once for every control rather than per call site. So a button that looks busy is a
call that is really in flight, and the shell cannot ask for two streams because a press landed
during the round trip. Two of these used to carry a `bool` of their own and four carried
nothing at all, which is exactly the drift one owner per fact exists to stop.

The decision: **MVVM as Avalonia means it, not as it is usually written.** Compiled
bindings and `INotifyPropertyChanged` are the toolkit's idiom and fighting them produces
bad Avalonia code. What is dropped is the usual habit of letting handlers poke individual
properties. Every write goes through the one render function, so the binding layer is a
transport and never a second definition of what the window looks like.

## The viewer's arrangement

The grid is the one screen with a second window in it, and the rules it follows are worth
stating because none of them crosses the control contract. The backend describes decodes; how a
viewer arranges what it receives is this shell's whole job (`docs/ipc-api.md`).

**The arrangement is a pure function, and the panel only draws it.**
`Features/Viewer/Model/TileLayout.cs` takes aspect ratios and a box and answers rectangles. It
holds no control and no view model, so what the grid does is asserted in tests rather than looked
at in a screenshot. `Features/Viewer/View/TileGrid.cs` is the two Avalonia passes around it and
contains no arithmetic of its own.

**Every tile is one height, and it is a constraint rather than a result.** Each tile is as wide as
its own aspect ratio makes it at that height, so nothing is cropped or stretched. Letting each row
instead fill the width exactly would give each row its own height, and a row of one tile would come
out about twice the height of a row of two - which draws as one big tile beside some small ones
rather than as a grid. The height is therefore chosen once: the largest that lets every row fit the
width and the whole stack fit the box. Rows are centred in whatever width their contents leave over,
and the stack is centred in whatever height it leaves over.

**Both layout passes solve the same box, and it is the viewport.** Inside a scroll viewer, measure
is handed an unbounded height and arrange is handed back the height measure returned. Solving
against whatever each pass was given solves two different boxes, picks two different arrangements,
and places the tiles by one having measured them by the other.

**Fullscreen and pop-out are window states, not layout modes.** `LayoutMode` has two members,
Grid and Focus, and says how tiles sit relative to each other. Which window a tile is drawn in is
a separate fact, which is what lets three windows be fullscreen on three monitors at once - a
state a single app-wide fullscreen could not express. Folding either into the enum would give one
field two meanings.

**Fullscreen gives the screen to the stream, not to the app.** The window fills the monitor and
everything else comes off it: the rail, the grid, the settings panel, and the shell's three bands,
which are bound to the same fact the viewer holds (`ShellViewModel.HasChrome`). What is left is one
picture at its stream's own shape on black, letterboxed by the same solver that arranges a single
tile in a cell, so nothing is stretched to the shape of a monitor. Escape and a double click both
end it, since a filled window draws no menu to reach for, and the window returns to the state it
was in rather than to a normal one - a maximised window that came back restored would have lost a
state the reader chose. A fullscreen the desktop put the window in is left alone: the pass only
gives back what it took.

**Windows are reconciled, not opened by an event.** The view model names the streams that should
be in windows of their own; `ViewerView.axaml.cs` runs a pass that opens, closes and re-states
windows until that is true. Running it twice with unchanged state does nothing, which is the same
apply discipline everything else here follows - it is the code-behind exception only because
nothing binds a window into existence.

**A closed window reports a state, never a toggle.** Every close runs the same handler, and the
pass closes windows itself for streams the reader has already given back, so a close that toggled
would ask for the window it was reporting the end of (`TileIntent.LeavePopOut`). That is the
general shape of news arriving from outside: what a window says when it closes is what is now
true, and only the menu row and the key that mean "either way" are toggles.

**A popped-out stream keeps its slot.** The tile stays in the arrangement at its stream's shape
and draws a plate saying where the picture went, so nothing reflows when a stream pops out or
comes back. That plate holds no frame subscription and asks for no render size: the popped window
is the decode's only consumer, and a black box costing a full-size texture pool would be the one
arrangement this shell paid for twice.

**Whether a card draws the picture or the plate is the host's fact.** A popped-out stream is drawn
by two cards off one tile, the slot it keeps in the grid and the window it went to, so a card that
read the pop-out state off the tile would put the plate in both of them and the picture in neither.
The grid states it on the card it templates (`TileCard.PictureElsewhere`), every other host draws
the picture, and clearing the source is what makes the plate free rather than merely dark. The same
split is why the main window's fullscreen names a stream in its own grid: a stream that pops out
gives that window back, since a filled window drawing a plate is a screen given to a sentence about
another window.

**Levels have their own notification.** `Session.Changed` re-renders every screen and is right
for a state that moved when something happened. Levels move fifteen times a second, so they land
on `Session.Metered`, which only the meters subscribe to. The separation on the wire
(`docs/ipc-api.md`) would be worth nothing if both ends of it arrived on one signal here.

**A tile's render-size ask is quantised and debounced.** Every distinct size re-announces a pool
in the backend, and a rearranging grid moves every tile's exact size. `StreamTile` rounds the ask
up onto a ladder of heights and sends it once the size has settled, so most rearrangements ask
for the size already in force.

## What is not settled yet

**Video, on the platform whose handle type is not built.** `Features/Viewer/Tile` draws two
of the three, and which one a tile uses is read off the pool rather than off the operating
system: `StreamTile` owns the subscription and the loan, and one `ITileSurface` per handle
type owns the import.

- **Windows** - `SharedTextureSurface` imports a DXGI shared texture through
  `Compositor.TryGetCompositionGpuInterop()` and `ICompositionGpuInterop.ImportImage`,
  draws it on a `CompositionDrawingSurface`, and hands the slot back with
  `UpdateWithKeyedMutexAsync`.
- **Linux** - `DmaBufSurface` imports a dmabuf descriptor itself, with
  `eglCreateImageKHR(EGL_LINUX_DMA_BUF_EXT)` and `glEGLImageTargetTexture2DOES`, and draws
  it from an `OpenGlControlBase`. The compositor imports a shared texture and an opaque
  descriptor and not a dmabuf, which is why this one draws where the other hands over. The
  descriptors arrive over the socket the pool names rather than in the message
  (`Backend/FrameDescriptors.cs`), because a descriptor is not a number another process can
  use.
- **macOS** - IOSurface from VideoToolbox, with no first-class import handle type. The
  weakest leg, and the one to schedule last.

`NativeControlHost` plus `gst_video_overlay_set_window_handle` was the easy path and the
wrong one, and the reason is visible on the tile: the native child window draws above all
Avalonia content, so the name, the colour badge and the stats panel would all have
disappeared behind the video. Both surfaces are composition visuals for that reason, the
OpenGL one included.

A tile whose handle type has no surface refuses rather than falling back to a copy through
system memory, and says so. A fallback that worked and cost gigabytes a second is the
outcome the frame channel exists to prevent, and one that is quietly slow is worse than one
that names itself.

**GStreamer bindings.** `gstreamer-sharp` wraps the 1.12 API and is effectively
unmaintained, so the pipeline is not being rewritten in C#. The plan that avoids the
problem entirely is two processes: Go keeps the pipeline and `go-gst`, this shell keeps
the UI, and frames cross as shared GPU handles. That makes the buffer-ownership protocol -
pool ownership, per-frame fences, release-back messages, and what each side does when the
other dies - the actual design work, not the import call.

That split has a second consequence worth stating: it also settles where the publish side
lives. Capture, encode and the publish pipeline stay in Go, and this module owns the
settings form that configures them, so the shell talks to one Go process whether it is
asking for frames or for an encoder change.

The control half of that conversation is settled and defined: `api/proto/screenshare/v1`,
gRPC over a named pipe on Windows and a Unix socket elsewhere (`docs/ipc-api.md`). The frame
half is the open question above, and it is deliberately not on that API - shared GPU handles
and a buffer-ownership protocol are a second channel, and no pixel crosses the control one.

**No DevTools.** `Avalonia.Diagnostics` was dropped from the Avalonia repository in 12 and
stops at 11.3.19. The replacements on NuGet are third-party, so none is pulled in here.
