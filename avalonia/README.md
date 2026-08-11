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
`task avalonia` is what puts those on the loader path, and only for the app, because they
shadow libstdc++ for anything else started from the same shell.
A `dotnet run` typed outside the task fails in Avalonia's platform init unless the machine
carries them somewhere else.

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

## Layout

Four layers, and the direction of dependency runs one way through them: a feature reads the
design system and the controls, and neither of those has ever heard of a feature.

| Path | Holds |
| --- | --- |
| `Contracts/Assert.cs` | always-on assertions, the C# counterpart of the Go `assert` package |
| `Mvvm/` | `Observable` and `DelegateCommand`: the change notification a compiled binding reads, and nothing else |
| `Design/` | the whole design system as tokens and styles - `Palette`, `Typography`, `Metrics`, `Text`, `Surfaces`, `Buttons`, `Inputs`, `Tooltips`, `Icons` |
| `Controls/` | the primitives more than one feature needs: `Chip`, `StatusPill`, `CheckItem`, the segmented control, the switch |
| `Copy/` | every word on screen: what each identifier the backend sends is called, the paragraph behind each choice, each control's heading and help, and the sentence for each statement the backend makes |
| `Features/Shell/` | the window, the title bar, the shared nav strip, the status band, and which destination is showing |
| `Backend/` | the control-plane seam: `IBackend`, the gRPC client that answers it over the local socket, and the settings write that goes through the message descriptor |
| `Features/Fields/` | the generic renderer for one group of the resolved form, and the placement table saying which destination draws which group. It is not under a feature because two of them draw form groups |
| `Features/Setup/` | the publish wizard, one step per group of the resolved form that is about sending, plus a terminal one: the step strip, the Quality form, the raw-property drawer, the cost rail, and the review |
| `Features/Broadcast/` | the live overview: the promoted figures, the live-safe actions, read-only configuration, the program preview, the per-viewer table, the sparklines |
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

Greyscale everywhere except a single red, `#E5484D`, reserved strictly for "on air" and
"something is wrong". It is the only hue on any screen and the only one a colour-blind
reader still separates from grey reliably, so spending it on state that is merely on would
cost the app its one unmistakable signal.

Two families and two weights carry everything: Inter, which is bundled with the app rather
than named and hoped for, and IBM Plex Mono. The rule holds without exception: anything
machine-generated or numeric is mono - identifiers, timers, throughput, resolutions, the enum
values the backend spells - and anything a person wrote is sans. A line that mixes both
switches family mid-run. Sizes are seven whole pixels; the half-pixel steps the mockups were
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
reaches the backend through `StartPublish`. The group is beside the tiles now and has a
`SaveSettings` of its own.

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
process that would not start, so reading that code as absence answered a press of Go live with
a sentence about the endpoint the shell had just resolved a form through. What says the backend
is absent is `Status.DebugException`, which the client library sets on a status it made from a
local failure and leaves null on one that arrived - told apart by code rather than by matching
on a sentence, which is the input that changes without anything failing to compile.

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
whether the review's switch asked for the broadcast screen, and it moves on the pass where the
stream is actually in force - a start that was accepted is not yet a stream, and navigating on
the reply would be the window claiming a state the backend has not reported.

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

What is still absent is the congestion band. The relay states its figures as they stand at each
poll and marks no interval, so a window shaded on the latency plot would be a detection this side
performed and attributed to the backend.

What went with the seeds is worth listing, because each was a mockup number that read as a
measurement: the on-air pill's timer, which stood at `00:42:18` in every window whatever was
publishing and is now the encoder's own clock, read back off the broadcast screen so the pill in
the chrome and the pill in the header cannot disagree; the sparkline's `60 s` window labels,
which named a span the plot did not cover and are now the samples' own; the dashed red congestion
band, drawn at a fixed quarter of the way across a plot with nothing in it, for a condition
nothing detects; the `vbv ceiling` rule, drawn at a constant third of the height while the curve
is scaled to the run's own peak, so it marked the ceiling only by coincidence and now is placed
against that peak or not drawn at all; and the viewer table's `every 5 s`, a period the contract
does not carry and which the backend did not use.

**The preview tile draws a frame now, and what it draws never leaves this machine.** The
publish child copies its already-encoded video to a loopback port beside the sink that feeds
the relay, the backend decodes what arrives there, and this card subscribes to it
(`docs/viewer-architecture.md`, "What the broadcast preview draws"). It reuses the same
`Features/Viewer/Tile` view model and control the viewer's grid uses rather than growing a
second frame consumer - two frame paths would be two answers to what a dropped frame is and
where a lent handle goes back. What differs between the two tiles is `Tile/Model/TileSource.cs`,
which is the contract's own oneof: a relay decode named by stream and leg, or the running
publish's preview named by nothing at all.

**This card calls no effect, and that is the shape rather than an omission.** The preview
pipeline goes up with the publish child and down with it, so there is no `StartReceive` to send
and no decode to close; `PublishState.Live.preview` is what says whether there is a picture to
draw, read through on every pass like every other state. It went the other way once - the card
opened a decode of this machine's own stream and read it back off the relay - and that cost the
screen beside it its own figures: the preview occupied a reader slot, so a stream nobody was
watching reported a viewer and the worst-viewer plot described the publisher's own loopback.

Two things about it are still the shell's own arrangement. Whether the card is on screen is an
input the view writes on attach and detach, because the window renders every destination on
every pass and frames nobody is looking at are GPU copies nobody asked for. And the placeholder
stays for the states where there is no picture - nothing publishing, a stream the backend is not
previewing, and the tile's own three - each of them saying which one it is.

The card's own sentence carries what the change made true and what it made invisible: the
picture costs one local decode and no bandwidth and adds no viewer to the counts, and it is
taken *before* the relay, so it says nothing about what viewers receive. A reader who took a
perfect preview for a healthy stream would be reading it exactly wrong, which is why that
sentence is on the card and not in a comment (`Copy/Cards.cs`).

The review's "Save as preset" switch is the one control on that screen that still does nothing.
`SavePreset` takes a name and there is no field for one here yet, so wiring it would mean this
module inventing what a preset is called (`docs/presets.md`).

**The viewer is a grid over a roster.** The grid draws the streams the reader asked to see,
from the GPU memory the backend decoded them into: a row's `show` toggle opens a decode
through `StartReceive` and the tile subscribes to its frames, each frame arriving as a slot
of a lent pool that goes back only once the compositor has taken it
(`docs/viewer-architecture.md`, "The buffer-ownership protocol"). Nothing about the
arrangement crosses the contract and nothing could: the backend describes decodes, and a
decode is not a tile.

The figures under each picture are two kinds and they come from opposite directions. What
the pipeline turned out to be - the render chain that ran, the memory the frames were in
when they reached the sink, the decoder and whether it ran on silicon - is `ReceiveState`,
read through on every pass like every other state. What this window got and drew is the
tile's own and can come from nowhere else: a backend cannot see that a compositor was too
slow to take a frame, so the dropped count is the one figure the shell reports rather than
receives.

The `show` toggle is one control and not one per leg, unlike the roster's. Which protocol a
tile receives on is `viewer.tile_watch_transport`, a setting the backend resolves and
repairs; offering it per row would be this screen deciding something the settings screen
already decides.

**The roster underneath is unchanged.** Every row comes off
`GetRelayStatus` and `GetViewerState`: which streams the relay carries, whether each is being
served, what it says they carry, how many readers each has, what each is ingesting, and which
legs this machine already has a viewer open on. The legs a row offers are the options of the
form's watch-leg field, so this module holds no list of protocols; whether a given leg can
carry a given stream is the backend's answer when the viewer is opened, and its refusal is
shown as it stands. The relay snapshot can be older than the stream, so greying a leg here
from a stale format would refuse a viewer that would have worked.

**The spotlight, the per-tile menu, the chip row and the pop-out windows are still gone.**
They drew mockup figures beside real ones, and the thing they were mockups *of* needed
frames. The grid that came back was designed against a real decode path rather than against
a seed, which is why it is one tile shape with measured figures and none of the four. Nothing
in the contract describes any of it either, and that is the point: how a viewer arranges what
it receives is this module's job, so the backend describes no grid to open, no tiles to
report and no layout to pick (`docs/ipc-api.md`).

`⇧S` is the tile's own key rather than the window's. A shortcut on the window would have to
invent a rule for which tile it meant; hanging it off the tile makes the pointer that rule,
which is why a press on a tile takes the keyboard and nothing is drawn for it.

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
asks the backend for something - Go live, Stop, Measure, Look again, Open full log, a stream's
grid toggle and each of its watch legs - is a `Mvvm/PendingCommand.cs`: it holds whether the
call it started is still out, refuses a second press off that same field, and clears it in a
`finally` so a call that failed past whatever the effect handles still gives the control back.
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
width and the whole stack fit the box. Rows are centred in whatever width their contents leave over.

**Both layout passes solve the same box, and it is the viewport.** Inside a scroll viewer, measure
is handed an unbounded height and arrange is handed back the height measure returned. Solving
against whatever each pass was given solves two different boxes, picks two different arrangements,
and places the tiles by one having measured them by the other.

**Fullscreen and pop-out are window states, not layout modes.** `LayoutMode` has two members,
Grid and Focus, and says how tiles sit relative to each other. Which window a tile is drawn in is
a separate fact, which is what lets three windows be fullscreen on three monitors at once - a
state a single app-wide fullscreen could not express. Folding either into the enum would give one
field two meanings.

**Windows are reconciled, not opened by an event.** The view model names the streams that should
be in windows of their own; `ViewerView.axaml.cs` runs a pass that opens, closes and re-states
windows until that is true. Running it twice with unchanged state does nothing, which is the same
apply discipline everything else here follows - it is the code-behind exception only because
nothing binds a window into existence.

**A popped-out stream keeps its slot.** The tile stays in the arrangement at its stream's shape
and draws a plate saying where the picture went, so nothing reflows when a stream pops out or
comes back. That plate holds no frame subscription and asks for no render size: the popped window
is the decode's only consumer, and a black box costing a full-size texture pool would be the one
arrangement this shell paid for twice.

**Levels have their own notification.** `Session.Changed` re-renders every screen and is right
for a state that moved when something happened. Levels move fifteen times a second, so they land
on `Session.Metered`, which only the meters subscribe to. The separation on the wire
(`docs/ipc-api.md`) would be worth nothing if both ends of it arrived on one signal here.

**A tile's render-size ask is quantised and debounced.** Every distinct size re-announces a pool
in the backend, and a rearranging grid moves every tile's exact size. `StreamTile` rounds the ask
up onto a ladder of heights and sends it once the size has settled, so most rearrangements ask
for the size already in force.

## What is not settled yet

**Video, on the two platforms whose handle type is not built.** Windows renders frames
today: `Features/Viewer/Tile` imports a DXGI shared texture through
`Compositor.TryGetCompositionGpuInterop()` and `ICompositionGpuInterop.ImportImage`, draws
it on a `CompositionDrawingSurface`, and hands the slot back with
`UpdateWithKeyedMutexAsync`. `NativeControlHost` plus
`gst_video_overlay_set_window_handle` was the easy path and the wrong one, and the reason
is visible on the tile: the native child window draws above all Avalonia content, so the
figures under each picture would have disappeared behind the video.

What is left is the other two handle types:

- **Linux** - dmabuf, which `vah264dec` already produces. Avalonia lists dmabuf import as
  planned rather than shipped, so the near-term route is `OpenGlControlBase` plus
  `eglCreateImageKHR(EGL_LINUX_DMA_BUF_EXT)`. Format modifiers and a sync-file fence have
  to travel with the frame; the contract already carries the first two, and nothing
  produces or reads them yet.
- **macOS** - IOSurface from VideoToolbox, with no first-class import handle type. The
  weakest leg, and the one to schedule last.

A tile on either refuses rather than falling back to a copy through system memory, and
says which of the two it is. A fallback that worked and cost gigabytes a second is the
outcome the frame channel exists to prevent, and one that is quietly slow is worse than
one that names itself.

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
