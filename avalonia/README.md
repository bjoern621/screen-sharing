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
task avalonia:test     # 70 tests, no relay and no backend needed
```

`task relay` first, or the app renders its failure state, which is also worth looking at.

The setup flow needs the Go backend running, since everything on it is resolved there. The app
starts one itself when nothing is listening on the control endpoint, so `task avalonia` is the
whole of what a reader has to run; a backend already up - a `task dev` run, a second window -
is connected to rather than duplicated, and one this app started is stopped when the window
closes. Where there is no backend binary to start, the app says so and offers to look again,
which is the other state worth looking at.

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
| `Features/Setup/` | the publish wizard, one step per group of the resolved form plus a terminal one: the step strip, the generic form renderer most of the steps are, the Quality form, the raw-property drawer, the cost rail, and the review |
| `Features/Broadcast/` | the live overview: the promoted figures, the live-safe actions, read-only configuration, the per-viewer table, the sparklines |
| `Features/Viewer/` | the relay roster: one row per stream the relay carries, and a toggle per watch leg |

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
argument in one place. Every step but two is **one component**, `Features/Setup/Fields/`,
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
renamed cannot leave a hole. What is still this module's is placement - the terminal step, and
the one group drawn by a layout of its own (`Model/QualityLayout.cs`).

Two things keep the rest honest. A field's key is a settings group and a field in it, so a write
goes through the message descriptor (`Backend/SettingsDraft.cs`) and a field added to the
contract is a control that appears and works with nothing here to edit. And the form's answer
is adopted whole: `SetupViewModel` replaces its draft with the one `ResolveForm` returned
rather than merging, which is what keeps a greyed option and its replacement from disagreeing.

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

**Go live is real, and it is gated on four states rather than on one.** It sends the
draft to `StartPublish`, which persists it and starts the encoder on it; the reply says nothing
and the stream that resulted arrives on the event stream, so the window that pressed the button
and the window that did not learn it the same way. What locks the button is
`Features/Setup/Model/PublishGate.cs`, and every condition in it is a whole state some other
side stated: `Form.publishable` for the settings, the backend's own sentence when it cannot
describe them at all, the presence of `PublishState.live` for a stream already on the air, and
`RelayStatus.reachable` for somewhere to send to. None of them is ranked or re-decided here, and
only one sentence is shown - a reader fixes them in that order anyway. A settings problem gets no
sentence of its own at all, because the preflight card beside the button already carries every
one of them in the backend's words; paraphrasing a diagnostic would be this module writing a rule
down twice.

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
and a figure with no source prints an ellipsis rather than a zero. That covers round trip, packet
loss and buffer fill, which nothing in the pipeline reports, and it is why the viewer table is a
reader count with a sentence instead of a row per viewer.

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

The tile is still a placeholder rather than a frame, for the reason "What is not settled yet"
gives below, and the figures over it are real.

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
