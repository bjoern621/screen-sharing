# avalonia

The Avalonia app, and the only desktop surface there is.
Settings form, encoder and transport pickers, live-now list, tile grid.

**The shell reads, it does not find out.**
Whether the relay is up is a state the backend polls and announces (`docs/ipc-api.md`), so a poller here would be a second answer to one question.
One relay reading in the whole app, on `Backend/Session.cs`.

## Running it

```sh
task avalonia          # run it
task avalonia:build    # build into build/bin/avalonia
task avalonia:test     # the view-model suite, no relay and no backend needed
```

`task relay` first, or the app renders its failure state, which is also worth looking at.

The setup flow needs the Go backend, everything on it being resolved there.
The app starts one when nothing is listening on the control endpoint, so `task avalonia` is the whole of what a reader runs.
A backend already up is connected to rather than duplicated.
One this app started is stopped when the window closes.
No backend binary to start: the app says so and offers to look again.

### Which windowing backend runs

Wayland where the session has a compositor, X11 where it does not, decided at startup by `UseWaylandWithFallback` (`Program.cs`).
`Avalonia.Desktop` carries the X11 backend alone, so the Wayland one is a package reference of its own, and the flake's dev shell carries the libraries it resolves by soname.

Which one runs decides how the window looks on a scaled desktop.
An X11 client in a Wayland session goes through XWayland.
A compositor that scales XWayland hands the client the logical size and magnifies what it drew: a soft window beside sharp native ones, uncorrectable after the fact.
The Wayland client draws at the output's own scale, and follows a window moved between outputs of different scales.

Two things the X11 backend does that the Wayland one does not.

| X11 does | Wayland instead | Cost |
| --- | --- | --- |
| sets `WM_CLASS` | sends no `xdg_toplevel.set_app_id` | a compositor sees an empty application id, and window rules keyed on it do not match |
| draws the client-side decorations a desktop without server-side ones expects | negotiates through `zxdg_toplevel_decoration_v1` (`Features/Shell/Model/WindowChrome.cs`) | a compositor answering "server side" draws whatever it draws, which on a tiling session is nothing |

### What a capture of this machine sees

Every window this shell opens asks to be kept out of the captures this machine takes, and states it for itself when it opens (`Features/Shell/Model/CaptureExclusion.cs`).
The two are the main window and each popped-out stream.

The reason is a feedback loop closing in the captured pixels, beyond anything this code can reach.
A tile draws a decode of the screen it is drawn on, so the next capture carries the tile, which carries the capture before it.
Every round trip nests one more copy of the screen inside the picture, and the depth grows with how long the stream has run.
Nothing downstream can undo it, the nesting already being in the frames the capture handed over.

Windows stores a display affinity against the window in kernel mode.
`WDA_EXCLUDEFROMCAPTURE` leaves the window on the monitor and takes it out of what the desktop window manager composes for anything else.
That covers every Windows capture backend here: the desktop duplication and graphics capture behind `ddagrab` and `d3d11screencapturesrc`, and the screen bit blit behind `gdigrab`.
Windows before 10 version 2004 applies `WDA_MONITOR` instead, leaving an empty rectangle.
Both break the loop.

Linux is left as it is, so a tile there goes on nesting for as long as the stream runs.
X11 serves the root window to any client that asks, so a capture is a read of the whole screen and there is no per-window fact to set.
No Wayland protocol carries the request.
The compositors that can black a window out of a screencopy, Hyprland and niri among them, do it from a window rule in their own configuration, which is the user's to write.
Such a rule is keyed on the application id, and the Wayland backend sends none, so it matches this window in an X11 session alone.

## Layout

Four layers, dependency running one way: a feature reads the design system and the controls, and neither has heard of a feature.

| Path | Holds |
| --- | --- |
| `Contracts/Assert.cs` | always-on assertions, the C# counterpart of the Go `assert` package |
| `Mvvm/` | `Observable` and `DelegateCommand`: the change notification a compiled binding reads, and nothing else |
| `Design/` | the design system as tokens and styles: `Palette`, `Typography`, `Metrics`, `Text`, `Surfaces`, `Buttons`, `Inputs`, `Menus`, `Tooltips`, `Icons` |
| `Assets/Fonts/` | the mono family `Design/Typography.axaml` names, as files: Avalonia packages Inter and no mono, so this one is carried rather than resolved off the platform |
| `Controls/` | the primitives more than one feature needs: `Chip`, `StatusPill`, `CheckItem`, the segmented control, the switch, and `SideColumnPanel`, which puts a screen's side column beside its body or over it |
| `Copy/` | every word on screen: what each identifier is called, the paragraph behind each choice, each control's heading and help, the sentence for each statement the backend makes |
| `Features/Shell/` | the window, title bar, shared nav strip, status band, and which destination is showing |
| `Backend/` | the control-plane boundary: `IBackend`, the gRPC client answering it over the local socket, and the settings write going through the message descriptor |
| `Features/Fields/` | the generic renderer for one group of the resolved form, and the placement table saying which destination draws which group. Not under a feature because two of them draw form groups |
| `Features/Setup/` | the publish wizard, one step per sending-related group plus a terminal one: step strip, screen picker, Quality form, audio source list, raw-property card, review, and the rail every step draws beside them carrying cost, checks and saved presets |
| `Features/Broadcast/` | the live overview: promoted figures, live-safe actions, read-only configuration, the outgoing preview, the per-viewer table, the sparklines |
| `Features/Viewer/` | the tile grid and its rail: one entry per stream the relay carries, the arrangement of the ones being watched, and the panel holding how this machine receives |

### The two rules the tree encodes

**A slice per component, and MVVM inside it.**
A feature directory holds `Model/`, `ViewModel/` and `View/` for itself, plus a directory per component substantial enough to have its own three.
`Features/Setup/QualityStep/ViewModel/QualityStepViewModel.cs` is the shape.
Namespaces mirror the path exactly, so a file's name says where it sits.

**Nothing outside `Design/` states a colour, size, font or radius.**
A component asks for the role it wants (`MutedBrush`, `RadiusPanel`, `FontSizeLabel`) and the palette decides.
That keeps a light variant a second dictionary rather than a sweep through every view.

### The design language

`docs/design-language.md` states the language for the whole product, and this module is its reference implementation: `Design/` is where those numbers live.
What is this module's own is how they are attached.

**Every style table is keyed on the type rather than on a name.**

| Table | Keyed on |
| --- | --- |
| `Design/Buttons.axaml` | `Button` |
| `Design/Inputs.axaml` | `TextBox`, `NumericUpDown`, `Slider` |
| `Design/Menus.axaml` | `MenuFlyoutPresenter`, `MenuItem`, `Separator` |
| `Design/Icons.axaml` | every icon, sized and stroked |

A view that writes a bare control gets the design without asking, which stops a surface opting in by name and wearing Fluent when it forgets.

**A named theme is only ever a variant, inheriting the base and stating only its difference.**
Buttons: `ActionButton`, `FooterButton`, `DangerButton`, `PrimaryButton`, `LinkButton`, `CardButton` and the `OptionCard` built on it.
One input variant: `Controls/NumberSelect`, the number box and the button glued into one control.
One menu variant: `SelectMenuItem` in `Controls/Select`, adding what picking a row does and whether it can be picked.
A variant that restates the template is a second control, and two of them drift.

A flag is the switch in `Controls/Toggle`.

## This module is a display

It decides nothing.
Every value a control offers, every label, every greyed entry and the sentence explaining it, every warning and every derived figure comes from the Go backend.
What it contributes is layout, typography, colour, motion, input handling and accessibility.

`docs/ipc-api.md` states the rule.
`api/proto/screenshare/v1` is the contract.
Read `Form` first: `ResolveForm` takes a settings draft and returns the whole screen, and a view model renders it rather than evaluating it.

So a `switch` over a codec name, a list of rate-control modes, a hardcoded resolution ladder or a hand-written tooltip is a defect, in the way a view field mirroring a model field is.

The wizard is that argument in one place.
Every step outside the terminal one and the groups with a layout of their own is **one component**, `Features/Fields/`, instanced once per group.
They differ in nothing this module can see: each is a `FieldGroup`, a run of fields with different keys, and the renderer switches on `ControlKind` rather than on what the field means.
A capture view and an encode view written separately would be this module writing down what a capture and an encode are.

**Which steps there are is the form's answer too.**
`SetupSteps.For` derives the strip from `Form.groups`, so a group added to the contract is a step that appears with nothing here to edit, and one renamed cannot leave a hole.
Placement stays this module's: the terminal step, the groups drawn by a layout of their own (`Model/QualityLayout.cs`, `Model/AudioLayout.cs`), and which destination draws which group (`Features/Fields/Model/GroupPlacement.cs`).

**The watch group is drawn by the viewer, the same placement rule doing real work.**
The wizard configures what this machine *sends*.
The watch group is the legs a stream comes back on, the jitter buffers a receiver holds and the chain a tile converts frames with.
It sits beside the tiles, in a column with a commit of its own and a button to dismiss it, so a reader who only watches keeps a change from there without opening the broadcast setup or going live.

**The group is staged, so nothing in it reaches a decode until the commit does.**
Every knob a receive pipeline reads is read by the backend out of its own settings as the pipeline is built.
The one value the shell names in the call is the tile's leg, read out of those same stored settings (`Features/Viewer/Tile/Model/TileLeg.cs`).
Taking the leg off the draft instead makes half the panel take effect as it is edited and holds the other half back, so a decode could open on an unkept protocol with kept buffers.
`FormSession.Stored` is what both read.

The panel says when what it shows is not what is stored.
A staged group draws the same controls whether or not it has been kept, so without the sentence the button has nothing to mean and a repaired value nobody wrote back is invisible.
A commit that lands closes the column.
One the backend refuses leaves it open, the sentence explaining the refusal being on it.

**The commit goes down the same queue an applied field's keystroke does** (`FormSession.SaveAsync`).
The settings travel whole, so the two are writes of one message.
Sent from two places they are two unary calls with no ordering, the older snapshot landing last being a stored setting the reader had already changed.

**Both screens read one draft.**
`Backend/FormSession.cs` owns the settings being edited and the form they resolve to, for the whole window, as `Backend/Session.cs` owns the running state.
A draft per screen would be two copies of one message, and the publish commit persists the whole message, so whichever screen committed would overwrite the other's half.

Two things keep the rest honest.
A field's key is a settings group and a field in it, so a write goes through the message descriptor (`Backend/SettingsDraft.cs`) and a field added to the contract is a control that appears with nothing here to edit.
And the form's answer is adopted whole: `FormSession` replaces its draft with the one `ResolveForm` returned rather than merging, which keeps a greyed option and its replacement from disagreeing.

`Backend/ControlBackend.cs` answers `IBackend`: a gRPC client over the named pipe on Windows and the Unix socket elsewhere (`Backend/ControlEndpoint.cs`).
It names no codec, no encoder family and no rule.
A greyed option arrives greyed, carrying the sentence that says why.

**The encoder probe is asked for once, in the background.**
`ResolveForm` reads what has been probed rather than probing, a resolve running on every keystroke and the probe test-encoding on every engine.
So the first forms of a session grey no codec for missing hardware, the honest reading of a fact nobody established.
The client asks for the probe behind the handshake and raises `IBackend.Changed` when it lands.
The setup flow re-reads, and the codecs this machine cannot run come back greyed with what is missing.
Nothing about which codecs those are reaches this module.

**A backend that is not running prints a sentence.**
The reads throw `BackendUnavailableException`, the flow shows its message above the steps with a "look again" button, and no form is invented meanwhile.
No timer behind that button, so an absent socket is not hammered for as long as the window is open.

**That it is still dialling is drawn, wherever that sentence lands.**
A sentence about an absent backend does not move, so a window between two attempts would draw the same thing as one that stopped trying.
What is drawn is the turning arc a pressed control wears, beside the banner over the steps and under the viewer's notice.
A countdown to the next attempt is a number nobody acts on, where whether anything is still happening is the reader's whole question.
Nothing is announced for it, an animation running off the render clock rather than off a state, so an absent backend costs the same render passes as before.

**A backend that comes back is noticed on the session's own dialling.**
The session already dials every couple of seconds, the event stream ending when the backend goes away, so the news that it answered is in the window whether or not anything asked.
The flow reads that transition and asks again once per recovery, the case nearly every start meets: the app launches the backend and reaches it a moment later, so the flow's opening read is the one call that fails.
What the button is still for is the failure nothing else reports, a read the backend served a refusal to.

`ControlEndpoint` starts a backend when the endpoint refuses a connection and asks again until it binds (`BackendProcess`), the backend being headless and a reader who opened the app having asked for both halves.
What is left is the case the shell cannot act on: no binary beside the app or on `PATH`, or an OS that refused to run it.
The sentence is the same either way, nothing is listening on this endpoint, that being what is true whether a start was attempted or not.

**Which sentence it is depends on who wrote the status.**
A status the backend served carries prose written for a person and is shown as it arrived.
The contract gives `UNAVAILABLE` to a relay that could not be reached and to a child process that would not start.
Reading that code as absence would answer a press of Start sharing with a sentence about the endpoint the shell had just resolved a form through.
What says the backend is absent is `Status.DebugException`, which the client library sets on a status it made from a local failure and leaves null on one that arrived.

**The commit is real, and it reads four states rather than one.**
It sends the draft to `StartPublish`, which persists it and starts the encoder on it.
The reply says nothing and the stream arrives on the event stream, so the window that pressed the button and the window that did not learn it the same way.
`Features/Setup/Model/PublishGate.cs` decides, and every condition in it is a whole state some other side stated: `Form.publishable`, the backend's own sentence when it cannot describe the settings at all, `RelayStatus.reachable`, and the presence of `PublishState.live`.
None is ranked or re-decided here, and only one sentence is shown.
A settings problem gets no sentence of its own, the checks card above the button already carrying every one of them in the backend's words.

**The fourth decides what the commit does, and that decision is the whole shape of that file.**
`ApplyToStream` is the effect for a stream already on the air, so a live stream decides *what the commit does* rather than whether it can be done.
The gate says which of the two as data (`PublishGate.Commit`) instead of leaving the label, the sentence and the call to each reach their own answer.
The press reads that state once more on its own pass rather than trusting the gate the last render composed.
A stream can start or end in between, and the backend refuses each effect in precisely the state the other one is for.

**The word on the button says which it is, and what applying costs.**
`Model/CommitCopy.cs` is one row per commit, holding the label and the two halves of the sentence the stream name sits in, read by the view model and bound whole rather than a ternary at the binding site.
The apply row says in plain words that the stream restarts, because it does.
Both engines run a child built from an argv, so new settings tear the pipeline down and launch another, and every viewer loses the picture across the gap.
The broadcast screen's quality track is greyed carrying that same fact (`Features/Broadcast/Nudge`), and a button promising an uninterrupted change would be the one place in the app that lied.

The relay half is the one state the shell reads from a poll it does not run.
The backend polls for as long as it is up, records each snapshot and answers `GetRelayStatus` from it, its opening value being unreachable with no reason, the honest reading of a relay nobody has asked.
The side the contract names as owner has to do the owning, and the honest opening value is what makes a gap visible rather than plausible.

Where the window goes afterwards is the window's.
The flow raises `WentLive` and the shell moves to the broadcast screen at once.
That screen is reachable whether or not anything is publishing, so the move claims no state the backend has not reported: it draws its idle reading and fills in as the live state lands.
Every start earns the move, so the destination a commit leads to is not a setting the review asks about.

## What each screen draws

**Every figure on the broadcast screen has a source, and one with none prints an ellipsis rather than a zero.**
Composed in `Features/Broadcast/Model/BroadcastSnapshot.cs` out of three whole states: the publish state, the newest encoder sample, the relay snapshot.

**The viewer table is a row per viewer, and the relay measures them one leg at a time.**
The backend reads the relay's reader array per path and joins each entry to the per-protocol connection list its type names, so a row is an address, a join time, and whatever that leg is instrumented for (`backend/internal/relay/readers.go`).
SRT is the one the relay times a round trip and states a loss rate on.
The rest report what was sent to them and what the relay's own queue discarded.
A cell with no measurement is an ellipsis, so a viewer over RTMP reads as untimed.
The severity rule in `Features/Broadcast/Model/ViewerRow.cs` reads presence rather than value, and the header promotes the *worst* viewer's round trip and loss under a label that says so.
Buffer fill and the decoder in use are figures nobody reports to a publisher, so those columns carry what the relay does measure: what was dropped, and the leg it went out over.

**The session log names who arrived and who left, and nothing announced either.**
There is no arrival or departure event: the relay reports who is connected at each poll and says nothing about who stopped being.
So the audience lines are the difference between two consecutive rosters in the snapshot series the session holds (`Features/Broadcast/Model/Audience.cs`), derived on every render pass rather than accumulated.
An arrival carries the relay's own join time, a departure the arrival time of the poll that first did not name the reader.
A poll naming no path for this stream contributes nothing at all: reading an unreachable relay as an empty roster would log every viewer leaving each time it hiccupped.

The congestion band is absent.
The relay states its figures as they stand at each poll and marks no interval, so a shaded window on the latency plot would be a detection this side performed and attributed to the backend.

Both sparkline series are stamped, so a point is placed by when it was taken against a fixed span ending at the newest reading.
A run younger than that span fills the right of the card instead of being stretched over it.
The `vbv ceiling` rule is placed against the run's own peak or not drawn at all, the curve being scaled to that peak.

**The preview tile draws a frame, by one of two routes the reader picks between.**
`docs/viewer-architecture.md`, "What the broadcast preview draws", states the two, the off segment beside them and what each costs.
What is this module's is the card around them.

Both reuse the same `Features/Viewer/Tile` view model and control the viewer's grid uses rather than growing a second frame consumer.
Two frame paths would be two answers to what a dropped frame is and where a lent handle goes back.
What differs is `Tile/Model/TileSource.cs`, the contract's own oneof: a relay decode named by stream and leg, or the running publish's preview named by nothing at all.

Whether the card draws is the reader's, written by the toggle's off segment and by nothing else.
It opens drawing, and on the local route.
It does not follow the window, the one place this card and the wizard's screen picker part company.
A publisher's window stands behind the thing being shared for most of a session.
A card that stopped whenever nobody was looking would be dark at the moment a reader came back to check on it, and would pay a pool import and a reconnect to come back.
Off closes the end-to-end route's decode rather than merely clearing the tile, what it is worth being that route's reader slot.

The placeholder stays for the states where there is no picture, each saying which one it is: off, a leg that refused the decode, nothing publishing, a route with nothing running behind it, and the tile's own three.

The card's own sentence is the selected segment's, the two routes making opposite claims and one sentence for both being false under one of them.
A reader who took a perfect local preview for a healthy stream would be reading it exactly wrong, which is why the sentence is on the card and not in a comment (`Copy/Cards.cs`).

**The rail carries the preset card, and it draws two different things under one heading.**
Above are the built-in presets, promises about the picture.
What "gaming" is on this machine is a search the backend ran over its own capability tables, so a row can be unreachable and what applying it writes differs per machine (`docs/presets.md`).
Below are the saved ones, and a saved preset is a name: kept, replaced and deleted by the name it is under.
So the card is a row per promise, a row per saved preset, and a name box (`Features/Setup/Presets/`).
It sits in the rail because a preset is the whole way of publishing, which no step owns a fraction of, and the rail is the one column every step draws.

A preset is a `PublishSettings` and nothing else, so applying one replaces that group of the draft and leaves the relay and watch settings where they are (`docs/presets.md`).
Nothing is committed by it: publish settings are staged until a commit carries them, so trying a preset out costs nothing and puts nothing on the air.

The store is the one state on this boundary that no event announces.
Presets are a file the backend does not run on, so a save or a delete is followed by a read rather than by patching the list.
The re-read is offered as a button, a preset another window saved being invisible here until someone asks.
The built-in half needs none of that.
It arrives on the form, as current as everything else the resolve answered with, and applying one reads the settings off the form the window holds rather than off the row that was rendered.

Which row is marked as in force is derived on every pass, and the two halves derive it differently.
A saved preset is marked while the draft equals it field for field, a snapshot saying every field.
A built-in one is marked while the draft delivers its promise, which the backend states on the form, so a field the promise says nothing about can move without taking the mark off.
Neither is a stored selection, so there is nothing to reconcile after a restart.

**The viewer is a grid over a roster.**
The grid draws the streams the reader asked to see, from the GPU memory the backend decoded them into.
A row's `show` toggle opens a decode through `StartReceive` and the tile subscribes to its frames, each frame arriving as a slot of a lent pool that goes back only once the compositor has taken it (`docs/viewer-architecture.md`, "The buffer-ownership protocol").
Nothing about the arrangement crosses the contract and nothing could: the backend describes decodes, and a decode is not a tile.

**Nothing is measured over the picture.**
A tile draws its name on hover and a colour badge where the range needs one, and every figure about the decode is in the stats panel the tile's menu opens.

The panel is composed from two readings coming from opposite directions.
What the decode is doing is `ReceiveStats`, a sample the backend reads off the running pipeline once a second and pushes.
It carries what is arriving, what came out of the decoder, what the sink took and threw away, and the counters the transport's elements keep (`docs/viewer-architecture.md`, "What a tile reports").
What this window got and drew is the tile's own and can come from nowhere else.
A backend cannot see that a compositor was too slow to take a frame, so the dropped count is the one figure the shell reports rather than receives.

Every row carries a tooltip saying what a reading of it is evidence of, which is what separates a diagnostic from a wall of numbers.
Rows are keyed on the identifiers the two sides share, the contract's field names and the elements' own names for their counters, and every word is in `Copy/Counters.cs`, where a key with no entry renders as the key (`docs/tooltips.md`).
The panel is composed only while it is up, so a grid of tiles nobody has opened one on builds nothing.

The `show` toggle is one control, where the roster's is one per leg.
Which protocol a tile receives on is `viewer.tile_watch_transport`, a setting the backend resolves and repairs.
Offering it per row would be this screen deciding something the settings screen already decides.

**The roster underneath.**
Every row comes off `GetRelayStatus` and `GetViewerState`: which streams the relay carries, whether each is being served, what it says they carry, how many readers each has, what each is ingesting, and which legs this machine already has a viewer open on.
The legs a row offers are the options of the form's watch-leg field, so this module holds no list of protocols, and each arrives carrying whether it is reachable and the sentence that says why not.
A leg the availability pass ruled out keeps its place in the menu, greys and draws its reason under its name.
That treatment is the roster's own: elsewhere a choice control lists what can be picked and folds the refused entries behind a disclosure (`docs/field-availability.md`).
Which half an entry is in is Go's answer in both, and all a shell decides is whether a refused one is on screen at this moment.
The reason is in the row rather than in a tip, a disabled control in Avalonia taking no pointer and a tip on it never opening.

**An entry prints the stream's own name and opens the whole path.**
A group is a path prefix, so every row of a member's list carries the same one and it separates none of them.
Which part of a path that prefix is stays the backend's answer, a prefix being a group key's digest, so a row carries both strings and prints `own_name` while its commands take `name`.

**Two different facts, and only one of them is greyed.**
Whether this machine has a player for a protocol at all is the availability pass's answer and it does not go stale, so the row obeys it.
Whether a *given stream* can travel on a leg is answered against the stream when the viewer is opened, and the relay's snapshot can be older than the stream.
Greying from that would refuse a viewer that would have worked, so the backend refuses with the format named and the row shows that sentence.
A leg already open stays pressable either way, the press being what closes it.

The keys are the tile's own rather than the window's, and each names a state the menu names.

| Key | State |
| --- | --- |
| `F` | filling a screen |
| `O` | focused |
| `P` | in a window of its own |
| `M` | silenced |
| `S` | the stats overlay |
| `+` and `-` | the level the decode plays at |

A shortcut on the window would have to invent a rule for which tile it meant, and each candidate for that rule is a second arrangement state to keep.
Hanging them off the tile makes the pointer the rule, so each card listens for keys on the window it is drawn in and answers only while the pointer is over it (`Features/Viewer/Tile/View/TileKeys.cs`).
The card never takes the keyboard, taking it on hover being taking it out of whatever the reader was typing in.
The keys are one table, read by the press and by the gesture the menu row prints, a menu naming a key nothing acted on being wrong the moment either half moved.
A press asks the command whether it can run first, so a key is refused wherever the row it names is greyed.

## How the repository's principles land in C#

`docs/development-principles.md` governs this module too.
Three of its four rules translate directly.
The fourth takes a decision, stated at the end of this section.

**State has one owner.**
`Backend/Session.cs` owns the running state (what is publishing, what the encoder is measuring, what the relay is carrying, which viewers are open) and the screens read it through on every pass and keep no copy.
The relay is the clearest case: setup's commit gate and the viewer's roster describe one relay and cannot disagree, reading one field.

**One render function.**
`Apply` is it.
It sets every output property on every pass, including the branches that turn something off, which is what makes a recovered relay clear the notice a failure left behind.
Nothing else writes those properties: private setters, compiler-enforced.
The reader's inputs are the other half of the split: their setters are the named writes, and `Apply` never touches them, a render pass that reassigned a text box fighting whoever is typing in it.

**Idempotency.**
`Apply` twice over unchanged state raises no notification: the property setters compare first, and a bound collection is rebuilt only when the rendered rows differ.
Every row type is a record for that reason, so two passes over one state produce values that compare equal rather than merely look alike.

**Assertions.**
`Contracts/Assert.cs` throws in Release as well as Debug, which `System.Diagnostics.Debug.Assert` does not: a contract that only holds in Debug is not a contract.
Message style follows the Go one: a present-tense sentence naming the invariant, offending values in the trailing arguments.

**A round trip does not get to bend any of them.**
`IBackend` is asynchronous because the thing behind it is a socket, and a render pass that awaited one would be a window that stops painting.
The split is the first rule: the last form the backend answered with is state `SetupViewModel` holds, `Apply` reads it and returns, and a draft change starts a resolve whose answer lands on a later pass.
A flow with no form yet is a state rather than a gap: every group renders its unresolved branch, the same one a step the form does not carry renders.

Two guards keep that honest, both the third rule.
Asking is skipped entirely while the draft still equals the one the backend was last handed, so `Apply` costs one round trip however many times it runs, which matters because `ShellViewModel.Apply` renders every destination on every pass.
And the latest answer wins: each resolve cancels the one before it and carries a request number, so an older draft's form arriving late is dropped rather than drawn.
Cancellation alone would not do it, being cooperative, and a call can already hold its answer when the token is set.

The answer arrives on whichever thread the transport completed on, so the write back is marshalled through an injected dispatcher rather than a toolkit reached for in place: `Dispatcher.UIThread.Post` in the window, a straight-through call in a test.
`Session` is handed the same one.

**And a round trip a reader started is a state, so it has one owner too.**
Every control that asks the backend for something is a `Mvvm/PendingCommand.cs`: Start sharing, Stop sharing, Measure, Create group, Look again, Open full log, a stream's grid toggle and each of its watch legs.
It holds whether the call it started is still out, refuses a second press off that same field, and clears it in a `finally` so a call that failed past whatever the effect handles still gives the control back.
The view draws the wait from the identical field through `Controls/Pending/Pending.cs`, an attached property setting one pseudo-class, and `Design/Pending.axaml` says what that looks like once for every control rather than per call site.
So a button that looks busy is a call really in flight, and the shell cannot ask for two streams because a press landed during the round trip.

The decision: **MVVM as Avalonia means it.**
Compiled bindings and `INotifyPropertyChanged` are the toolkit's idiom and fighting them produces bad Avalonia code.
What is dropped is the habit of letting handlers poke individual properties.
Every write goes through the one render function, so the binding layer carries its output to the view.

## The viewer's arrangement

The grid is the one screen with a second window in it, and none of its rules crosses the control contract.
The backend describes decodes.
How a viewer arranges what it receives is this shell's whole job (`docs/ipc-api.md`).

**The arrangement is a pure function, and the panel only draws it.**
`Features/Viewer/Model/TileLayout.cs` takes aspect ratios and a box and answers rectangles.
It holds no control and no view model, so what the grid does is asserted in tests rather than looked at in a screenshot.
`Features/Viewer/View/TileGrid.cs` is the two Avalonia passes around it and contains no arithmetic.

**Every tile is one height, a constraint rather than a result.**
Each tile is as wide as its own aspect ratio makes it at that height, so nothing is cropped or stretched.
Letting each row fill the width exactly would give each row its own height.
A row of one tile would then come out about twice the height of a row of two, which draws as one big tile beside some small ones rather than as a grid.
The height is chosen once: the largest that lets every row fit the width and the whole stack fit the box.
Rows are centred in whatever width their contents leave over, the stack in whatever height it leaves over.

**Both layout passes solve the same box, and it is the viewport.**
Inside a scroll viewer, measure is handed an unbounded height and arrange is handed back the height measure returned.
Solving against whatever each pass was given solves two different boxes, picks two different arrangements, and places the tiles by one having measured them by the other.

**Fullscreen and pop-out are window states.**
`LayoutMode` has two members, Grid and Focus, and says how tiles sit relative to each other.
Which window a tile is drawn in is a separate fact, which lets three windows be fullscreen on three monitors at once, a state a single app-wide fullscreen could not express.
Folding either into the enum gives one field two meanings.

**Fullscreen gives the screen to the stream.**
The window fills the monitor and everything else comes off it: rail, grid, settings panel, and the shell's three bands, bound to the same fact the viewer holds (`ShellViewModel.HasChrome`).
What is left is one picture at its stream's own shape on black, letterboxed by the same solver that arranges a single tile in a cell.
Escape ends it, and the window returns to the state it was in rather than to a normal one, a maximised window that came back restored having lost a state the reader chose.
A fullscreen the desktop put the window in is left alone: the pass only gives back what it took.

**A pass reconciles which windows are open.**
The view model names the streams that should be in windows of their own.
`ViewerView.axaml.cs` runs a pass that opens, closes and re-states windows until that is true.
Running it twice with unchanged state does nothing, the same apply discipline everything else follows.
It is the code-behind exception only because nothing binds a window into existence.

**A closed window reports the state it landed in.**
Every close runs the same handler, and the pass closes windows itself for streams the reader has already given back, so a close that toggled would ask for the window it was reporting the end of (`TileIntent.LeavePopOut`).
That is the general shape of news arriving from outside, and only the menu row and the key that mean "either way" are toggles.

**A popped-out stream keeps its slot.**
The tile stays in the arrangement at its stream's shape and draws a plate saying where the picture went, so nothing reflows when a stream pops out or comes back.
That plate holds no frame subscription and asks for no render size: the popped window is the decode's only consumer, and a black box costing a full-size texture pool would be the one arrangement this shell paid for twice.

**Whether a card draws the picture or the plate is the host's fact.**
A popped-out stream is drawn by two cards off one tile, the slot it keeps in the grid and the window it went to.
A card reading the pop-out state off the tile would put the plate in both and the picture in neither.
The grid states it on the card it templates (`TileCard.PictureElsewhere`), every other host draws the picture, and clearing the source is what makes the plate free rather than merely dark.
The same split is why the main window's fullscreen names a stream in its own grid.
A stream that pops out gives that window back, a filled window drawing a plate being a screen given to a sentence about another window.

**Levels have their own notification.**
`Session.Changed` re-renders every screen and is right for a state that moved when something happened.
Levels move fifteen times a second, so they land on `Session.Metered`, which only the meters subscribe to.
The separation on the wire (`docs/ipc-api.md`) would be worth nothing if both ends of it arrived on one signal here.

**A tile's render-size ask is quantised and debounced.**
Every distinct size re-announces a pool in the backend, and a rearranging grid moves every tile's exact size.
`StreamTile` rounds the ask up onto a ladder of heights and sends it once the size has settled, so most rearrangements ask for the size already in force.

## Open ends

**Video, one surface per handle type.**
Which surface a tile uses is read off the pool rather than off the operating system: `StreamTile` owns the subscription and the loan, and one `ITileSurface` per handle type owns the import.

On Windows, `SharedTextureSurface` imports a DXGI shared texture through `Compositor.TryGetCompositionGpuInterop()` and `ICompositionGpuInterop.ImportImage`, draws it on a `CompositionDrawingSurface`, and hands the slot back with `UpdateWithKeyedMutexAsync`.
On Linux, `DmaBufSurface` imports a dmabuf descriptor itself, with `eglCreateImageKHR(EGL_LINUX_DMA_BUF_EXT)` and `glEGLImageTargetTexture2DOES`, and draws it from an `OpenGlControlBase`.
The compositor's own import covers a shared texture and an opaque descriptor, so this one draws where the other hands over.
The descriptors arrive over the socket the pool names rather than in the message (`Backend/FrameDescriptors.cs`), a descriptor not being a number another process can use.
macOS has IOSurface from VideoToolbox, which carries no first-class import handle type, so no `ITileSurface` covers it.

`NativeControlHost` plus `gst_video_overlay_set_window_handle` is the wrong path, and the reason is visible on the tile.
The native child window draws above all Avalonia content, so the name, the colour badge and the stats panel would disappear behind the video.
Both surfaces are composition visuals for that reason, the OpenGL one included.

A tile whose handle type has no surface refuses rather than falling back to a copy through system memory, and says so.
A fallback that worked and cost gigabytes a second is the outcome the frame channel exists to prevent, and one that is quietly slow is worse than one that names itself.

**GStreamer bindings.**
`gstreamer-sharp` wraps the 1.12 API and is unmaintained, so the pipeline stays out of C#.
Two processes avoid it entirely: Go keeps the pipeline and `go-gst`, this shell keeps the UI, and frames cross as shared GPU handles.
The design work is the buffer-ownership protocol rather than the import call: pool ownership, release-back messages, how each side knows a frame's pixels have landed, and what each side does when the other dies.
No fence crosses: Windows pairs the handle with a keyed mutex, and the Linux export returns only once the device copy has finished (`docs/glossary.md`).

That split also settles where the publish side lives.
Capture, encode and the publish pipeline stay in Go, and this module owns the settings form that configures them, so the shell talks to one Go process whether it is asking for frames or for an encoder change.

The control half is `api/proto/screenshare/v1`, gRPC over a named pipe on Windows and a Unix socket elsewhere (`docs/ipc-api.md`).
The frame half is a channel of its own: shared GPU handles and a buffer-ownership protocol, with no pixel crossing the control one.

**No DevTools.**
`Avalonia.Diagnostics` is not part of Avalonia 12, and the replacements on NuGet are third-party, so none is pulled in here.
