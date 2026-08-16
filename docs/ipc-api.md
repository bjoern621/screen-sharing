# The control API

One Go process owns the product.
Every user interface is a display in front of it.

Contract: `api/proto/screenshare/v1`.
This page states the rule it encodes and what each side owes.

## The rule

**The backend decides what is true. A shell decides how to say it, and asks the backend to act.**

Without exception:

- Every value a control offers comes from the backend. Codecs, pixel formats, rate-control modes, transports, capture backends, frame rates, output resolutions, audio codecs, encoder presets, frame-memory values, monitors, watch legs. No shell holds a list of its own.
- Every greyed option and disabled field is decided in Go and arrives decided, with a **code** naming why. The four treatments in `field-availability.md` follow from the tables, not from a shell's judgement.
- Every derived figure comes from the backend: the bitrate estimate, the headroom against the uplink, the command preview.
- Every piece of state is read from the backend or received on its event stream. A shell caches nothing across a change notification.
- **Every word on screen is the shell's.** Labels, help text, option names, the paragraph behind a choice, the sentence in place of a greyed entry, how a unit is spelled and where it sits. Written where the layout is, keyed by the identifiers the backend sends (`text.proto`).
- Beyond words, a shell contributes layout, typography, colour, motion, input handling and accessibility. Its whole job.
- **Placement reaches as far as which screen a group is drawn on.** Groups and their order are the backend's. Where a shell puts them is not on the contract, which describes no screens. The Avalonia shell draws the watch group in its viewer and the rest in its publish wizard, invisible from Go (`avalonia/ScreenShare.App/Features/Fields/Model/GroupPlacement.cs`). A shell may never decide that a group exists, what is in it, or which entries are reachable.

**The two halves meet on identifiers and nowhere else.**
`hevc_nvenc`, `gstreamer`, `yuv420p`, `srt`: what the encoder, the element registry and the relay call these things.
`HEVC / H.265 · NVIDIA GPU` is one shell's answer at its width, in its typography, for its reader.
A shell looking a name up knows no more about a codec than one printing a sentence the backend sent.
It is the only side that can see the column, the tone and the language.

Three kinds of method, the rule in executable form:

| Kind | What it does | Examples |
| --- | --- | --- |
| Reads | Hand the shell something to draw. Compute, change nothing, cheap enough for a keystroke. | `GetCatalog`, `ResolveForm`, `GetPublishState`, `GetRelayStatus`, `GetMembersState` |
| Effects | Do the one thing the user asked for. The only methods that change the world. | `StartPublish`, `ApplyToStream`, `StopPublish`, `StartWatch`, `StartReceive`, `StopReceive`, `SetReceiveAudio`, `StartMonitorPreview`, `StopMonitorPreview`, `SaveSettings`, `ProbeEncoders`, `MeasureUplink`, `MeasureEncodeRate`, `OpenInBrowser`, `CreateGroup`, `JoinGroup`, `LeaveGroup` |
| Stream | Carries what changed, including what this shell did not do. | `Subscribe`, `SubscribeAudioLevels`, `SubscribePointer` |

Samples, not the whole surface.
`control.proto` carries that.
A method that is neither a read nor an effect does not belong here.
A shell that wants one is about to hold state the backend owns.

Measurements are effects, by the test rather than by category.
`ProbeEncoders`, `MeasureUplink` and `MeasureEncodeRate` each start real work, take seconds, and leave a result a later read answers from.
The probe most of all: what it finds is what every subsequent `ResolveForm` greys codecs against.
A read that replaces the answer another shell's next resolve would give, with nothing on the wire announcing it, is what this test catches.

**Membership is two effects and a read.**
`JoinGroup` draws this machine's identity in the group the settings name and states its first presence, `LeaveGroup` releases that presence and drops the identity, and `GetMembersState` answers the group as the backend last read it.
Nothing on the contract refreshes presence.
The backend states it on the loop that already polls the relay, so a method for it would be a second thing deciding when this machine is in a group (`membership.md`).

## Why

A shell with its own copy of the domain model is a chance to disagree with the encoder, invisible until it is a stream nobody can explain.
`domain-model.md` exists because a rule written twice drifts.
This contract stops a shell being the second place.

The second reason decides the shape rather than the direction.
Capture, encode and publish stay in Go, and frames cross as shared GPU handles.
The shell already talks to one Go process about frames.
A second, private idea of what a codec is would let the process holding the encoder and the process drawing its settings disagree about which encoders exist.

The rule also buys replaceability: a shell that decides nothing can be swapped outright, or driven by a test with no UI, everything it displays having been computed where a test can reach.

**The backend and the shell always run on the same machine.**
Development separation, not a step toward a remote shell, and not a trust boundary: two modules of one application on one box.
Nothing here is shaped against a hostile client, and a filesystem path crosses as a path.

## What crosses, and in what shape

| File | Holds |
| --- | --- |
| `settings.proto` | `PublishSettings`, `ViewerSettings`, `RelaySettings`, and `Preset` |
| `catalog.proto` | the fixed facts: codecs, decoders, audio codecs, encoder availability, GPU paths, capture backends, transport carriage, monitors, platform |
| `text.proto` | `Text`: one statement, as a code and the identifiers it is about. Nothing here is a sentence |
| `form.proto` | `Form`: every field, option, greying, reason, note, warning and derived figure, already decided |
| `session.proto` | the running state: publish state, encoder samples, relay snapshot, viewers |
| `events.proto` | `Event`, the server-push envelope |
| `control.proto` | `ControlService`, the whole callable surface |
| `frame.proto` | `FrameService`: handles, loans, release-backs. No pixels, no tile |

**What is not here is as much of the rule as what is.**
No grid, no tile, no window layout, no widget arrangement.
How a viewer arranges what it receives is the shell's job, on the list "The rule" gives it, which is exhaustive both ways.

What the contract carries instead is a viewer that reads.
`StartReceive` and `StopReceive` open and close a decode for one stream on one leg.
Receive state travels on the event stream.
`ViewerSettings` carries the watch leg, the jitter buffer and the render chain it is built from.
Facts about receiving, none of them a tile, an arrangement or a window.
A method that opened a *window* is the one thing this contract may not have.

`SetReceiveAudio` and `SubscribeAudioLevels` sit on the same side of that line, worth saying because a volume slider and a level meter are things a reader sees on a tile.
They name the decode's audio branch: one pipeline holds one, it plays on this machine, so loudness is the decode's property and not any window's.
Two tiles on one decode share the branch, so a per-tile volume would be several controls over one element.
The tile is where the slider is *drawn*.
What it sets is not the shell's.

`SubscribeAudioLevels` is a second stream, not an event kind.
`Subscribe` carries whole states when something changed, and a level changes continuously.
Folding it in would push the receive state at metering rate and re-render every consumer for a figure none of them reads.
Frequency alone is no reason to leave this service: the frame channel exists for frames, and a level is not one.
Each tick carries the whole set, fifteen a second, coalesced to the newest, so a reader that fell behind receives the present rather than a queue of the past.

`Form` is the one to read first: what makes a shell a display.
`ResolveForm` takes the three settings drafts and returns the whole screen: groups, fields in order, each field's control kind and unit, whether it is visible and enabled and why not as a `Text`, its value, a fresh installation's value, and for every control offering entries (select, radio, and the number carrying a ladder) each option with its value, note, enabled flag and reason.
The default travels per field for the reason every value does: a shell offering to put a group back would otherwise hold a table of defaults, which is the domain written twice.
Built-in presets ride here too, one entry each: what applying it would produce here, or a `Text` saying why nothing reaches it, and whether the draft already delivers it (`presets.md`).
On the form rather than on a method of their own, all three answers being functions of the draft exactly as a greying is.
Fetched separately they would be a verdict older than the settings beside them.
A shell renders that and sends back a changed draft.
It evaluates no rule, and writes every word.

**A field's key names its message.**
`publish.encoder`, `viewer.render_chain`, `relay.host`.
The key is the shell's only handle on where a value is written.
A bare name across three messages would need a lookup to say which descriptor it meant, which is one rule stated twice.
One resolve over all three also keeps a cross-message greying possible: a tile leg that cannot carry the publish codec is one call's answer, not two calls compared by a shell.

Three messages because they answer to different owners.
`PublishSettings` is what a preset copies between machines (`presets.md`).
`ViewerSettings` is this machine's drivers and this user's watching, and a render chain copied elsewhere may not register there.
`RelaySettings` is the one both need, the relay being publish destination and subscribe source.
One host stored twice is two hosts able to disagree.

`ResolveForm` has no side effect and is cheap on every keystroke.
It returns a possibly repaired draft alongside the form and names the fields it repaired: where the draft held a value the tables forbid, the backend walked to the first legal one (`form/repair.go`).
A shell adopts the returned settings wholesale, so a greyed option and its replacement cannot disagree.

## The format, and why this one

**Protocol Buffers for the schema, gRPC for the calls, a local socket for the transport.**

The schema has to be a machine-checkable artifact, two languages deriving from one definition being the whole point.
That rules out a hand-written JSON contract: a Go struct and a C# class, both hand-maintained, which is the drift this ends.

Between the schema languages that generate both sides:

- **Protobuf** generates Go and C# from one file, both first-class. `Grpc.Tools` compiles `.proto` during the .NET build, so C# has no checked-in generated code and no step skippable on one side only. Field numbering makes compatible evolution mechanical, and `buf breaking` enforces it in CI.
- **OpenAPI** describes an HTTP surface. Server-push needs a second mechanism bolted on, the C# generators are third-party and uneven, and JSON is the wrong shape for per-second encoder samples.
- **JSON-RPC over a pipe** is the smallest thing that works and has no schema. Enums would live in two hand-written places, the failure mode this removes.

gRPC then brings the two things a hand-rolled framing would reinvent badly: server streaming, which is exactly the event channel, and a status model with deadlines and cancellation.

**Local IPC, not a network port.**

| Platform | Endpoint |
| --- | --- |
| Windows | named pipe `\\.\pipe\screenshare-control-v1`, DACL restricted to the owning user |
| Linux, macOS | Unix socket `$XDG_RUNTIME_DIR/screenshare/control-v1.sock`, mode `0600`, falling back to the config directory where `XDG_RUNTIME_DIR` is unset |

No TCP listener, not even loopback.
A loopback port is reachable by every process on the machine and by anything the browser can be persuaded to fetch, and this service starts screen captures.
The socket path is the only discovery mechanism.
A shell that cannot open it starts the backend and asks again, the backend being headless and a user who opened the app having asked for both halves.
Still unreachable, it reports that the backend is not running and names the endpoint.
A shell's arrangement, not a contract rule: a backend already listening is connected to rather than duplicated, and one the shell did not start is not one it stops.
Both ends run gRPC over that stream, Go with a custom dialer and .NET with `SocketsHttpHandler.ConnectCallback`, so the service definition is the same on every platform.

**Frames do not cross this API.**
The frame channel is a second gRPC service on the same socket, carrying handle metadata and release-backs.
Pixels live in shared GPU memory the handle names and never enter a message.
One socket avoids reinventing framing, versioning and cancellation for a metadata stream, and changes nothing about the rule.

`FrameService` has one method, and its shape is the protocol.
`Frames` is bidirectional because the backend lends a slot and may not write into it again until the consumer hands it back, so the release travels on the call the loan did.
A release on a second call could outlive its subscription and free a slot of a pool that is gone.
One call per decode, so a consumer that dies mid-frame is one dead stream rather than every stream stalling.

It is also where "no tile on this contract" is easiest to break and is not: a subscription names a decode something else opened and never opens one.
The render size is a count of pixels rather than a layout.
Pool ownership, generations and what each side does when the other dies: `viewer-architecture.md`, "The buffer-ownership protocol".

**A subscription names one of three pictures, each carrying what tells one of its own kind apart.**
`FrameSubscribe` carries a `StreamRef` (a stream and the leg it crossed the relay over), `PublishPreview` (the local decode of what this machine sends), or `MonitorPreview` (one of this machine's screens, read live).
The preview message is empty and needs to be: `PublishState.live` is singular, so "the running publish's preview" is already a complete identity, and a name or port on it would be the consumer restating what it read off the state.
The monitor message carries one index for the mirror-image reason: the index is what the catalog enumerates screens under and what `PublishSettings.monitor` holds, so a size or name would be the consumer sending the catalog back.

Separate arms rather than legs in the transport table, two of the three crossing no protocol at all.
The publish child copies encoded video to a loopback port and the backend decodes it there (`viewer-architecture.md`, "What the broadcast preview draws").
A monitor preview is read off the screen and never encoded.
A synthetic transport entry would tell every consumer of that table that some protocol carries them.

**Which effect opens each keeps the frame channel from deciding anything.**
A relay decode: `StartReceive`.
The publish preview: the publish itself, the loopback port having to be in the child's argv, so there is no `StartPreview` for a shell to call.
A monitor preview: `StartMonitorPreview`.
All three the same division: the frame channel finds a picture or is refused with `FAILED_PRECONDITION`, and never starts one.
What tells a shell there is one to ask for is a state it reads first: `PublishState.Live.preview`, present exactly while a preview runs and carrying the port and what the pipeline turned out to be, and `MonitorPreviewState`.

**The monitor preview is an effect and the publish preview is not, and the difference is who owns the pipeline.**
A publish preview exists because a publish does.
A method to start one would be a method whose only correct implementation is to refuse.
A monitor preview exists because somebody wants to look at a screen, which no other state implies.
So it is asked for, it goes on reading until something stops it, and what runs reaches every shell on the event stream.
Not a read, for the reason `ProbeEncoders` is not: it starts work that outlives the call.

Where a session cannot read one output apart from another, every such call is refused, and `Catalog.no_monitor_preview` says so beforehand.
The ordinary shape of a capability here: the fact is stated, the shell offers what it allows, and the refusal exists for the caller that asked anyway.

## Errors

The repository's two error kinds (`development-principles.md`) cross differently, and that difference is the whole error model.

An **Umgebungsfehler**, a condition the app must survive, is a gRPC status: expected, the user's to see, its message prose written for a person.

| Status | Means |
| --- | --- |
| `INVALID_ARGUMENT` | names something that cannot exist: an unknown transport, an empty stream name |
| `FAILED_PRECONDITION` | well formed, world not ready: already publishing *a different pipeline*, nothing to apply to, a measurement while live |
| `UNAVAILABLE` | the relay could not be reached, or no child carried the stream: one that would not start, and one that started and exited before a frame left it |
| `NOT_FOUND` | a named preset, log or stream does not exist |
| `RESOURCE_EXHAUSTED` | a bounded resource over-asked, such as the test-stream count |

An **Entwicklungsfehler**, a broken internal contract, never crosses.
`assert` panics in the backend, as everywhere else here.
A shell that could receive a bug as a status would start handling bugs, and a handled bug ships.

**A failure that is a state is not a status.**
A call that succeeded and left something not working reports it on the state it belongs to, as a `Text` beside the raw words behind it.
`ExitInfo.cause` and `ReceiveExit.cause` on a run that ended, `ReceiveStream.failure` on a decode carrying no picture, `PublishState.Retry.cause` on each attempt of a backoff, `TestStreamSlot.cause` per slot, `MembersState.refusal` on presence the group service would not take.
The code is what a shell writes a sentence for, and the string beside it stays raw so a child's own words reach the reader unedited (`text.proto`).
On the status instead, the reason would belong to the one call that asked, and a shell reading the state afterwards would find a tile that is dark and nothing saying why.

**A served status is shown as the backend wrote it, and no code means "the backend is absent".**
A shell learns that from the connection failing: the client library makes its own status for a local failure.
Which code that wears is the platform's business, an absent named pipe being `INTERNAL` on Windows and an unbound socket `UNAVAILABLE`.
Reading `UNAVAILABLE` as absence turns a relay that refused a publish into a sentence about the endpoint, on a connection the same shell had just resolved a form through.

**An unreachable relay is not a call failure.**
`GetRelayStatus` succeeds and returns a snapshot whose `reachable` is false, carrying the reason.
"The relay is down" is a thing the screen says, not a thing the call failed at.

The same holds one level down.
A path's `reader_roster` is joined from the per-protocol connection lists, one call each.
A list that refuses leaves its readers named with every figure absent rather than failing the snapshot: the relay answered the question the snapshot is about, and a listener switched off has no list at all.
So `reachable` states whether the relay answered, and presence on each figure states whether that figure was measured.
Two facts, never collapsed into one.

## Events

Two rules keep the event stream from becoming a second definition of the state.

**Every event carries a whole state, never a delta.**
A shell receiving `PublishState` renders it.
It does not apply it to something it held.
A duplicate is harmless, and a dropped connection is recovered from by reading state again rather than replaying history.

**A shell that acted still waits for the event.**
`StopPublish` answers empty.
What the state became arrives on the stream.
One path into the display is what stops the window that pressed the button and the window that did not from showing different things.

## Versioning

Package `screenshare.v1`, and the directory says so, which makes a `v2` a new directory rather than an edit.

Within v1, evolution is additive: new fields take new numbers, no number is reused, no field changes type or meaning, and `buf breaking` checks against the previous commit.

**Before the first release, a rename is a rename.**
A field may be renamed in place, keeping number and type, with `FIELD_SAME_NAME` overridden for that one commit.
Nothing is deployed and both sides build from this tree, so a new number and a tombstone would spend the compatibility mechanism to preserve compatibility with nobody.
The exception ends at the first release, written here rather than left as an unexplained override in CI.

`Hello` is the first call on a connection and settles the major version before any other method, so a mismatch is a sentence naming both versions rather than a field that arrives empty.
The minor version is informational: a lower minor works, a higher one may find a method missing.

## Fields the contract leads

Defining the API first lets the contract name something the backend does not do yet.
A control the shell invented is a lie on screen.
A control the contract declares and the backend disables is a fact.
A control the backend later implements needs no shell edit.

`PublishSettings.output_resolution` is the case worth keeping.
Declared before anything could honour it: both shells drew it, the Go pipeline had no scaling stage, and `ResolveForm` answered with the field disabled and that as its reason.
The scaling stage landed (a `scale` filter on the ffmpeg software path, the size on the device conversion's own filter where frames never leave the GPU, `videoscale` plus a size on the encoder input caps for GStreamer), and the field became ordinary with nothing above the backend changed.
One case survives as a greying: a pair whose device path carries no conversion at all, the encoder reading captured surfaces directly, has nothing that can resize, so the scaled entries grey with the frame memory named as the way across.

## What each side owes

**The backend owes** one `ControlService` implementation, and every rule beside the tables it derives from: greyings, repairs, dropdown construction, prediction, the viewability verdict, the preset search.
They live in `backend/internal/form`, so `domain-model.md`'s list of consumers has one entry rather than one per shell.

It owes a **code** rather than a sentence for each verdict, and the identifiers behind it.
A greying arriving as prose is a rule and a wording travelling together, and the wording is the half no backend gets right for a surface it has never seen.

**A shell owes** a name for every identifier and a sentence for every statement.
A value with no name renders as the raw identifier, honest and still a defect.
A statement with no sentence renders as its code, which is worse and is meant to be.
Both visible rather than swallowed, because that is what gets them written.
In the Avalonia shell: `ScreenShare.App/Copy`.

It owes the render-function discipline (`development-principles.md`): one function per component, every output set on every pass, nothing cached that the backend owns.

It owes a `SaveSettings` for every write to a group the form marks `applied`, and none for the others.
Those are the settings the backend reads on a schedule of its own instead of being handed them by an effect.
The relay's address is the case that exists: a shell holding one for a commit would leave the backend dialling the address the reader had just replaced, and the publish that would have carried it is refused for exactly that reason.
At the cadence of a settled value rather than a keystroke, `ResolveForm` per keystroke being cheap and `SaveSettings` per keystroke being a file write, so a text control writes when the reader leaves it.
Which call a reader's move produces, and when, is `settings-editing.md`.
And it owes showing a disabled field rather than inventing an enabled one.

It also owes **asking for the encoder probe once.**
`ResolveForm` reads what has been probed rather than probing: a resolve runs on every keystroke and the probe test-encodes on every engine.
So on a machine nothing has asked about, no codec is greyed for missing hardware: the honest reading of a fact nobody established, and also a dropdown offering QSV with no Intel GPU.
A shell calls `ProbeEncoders` once, in the background, and goes on drawing what it has.

**Reading again is not owed, and that is the point of the split.**
`ProbeEncoders` announces the whole catalog on the event stream, so the shell that asked and the shell that did not are told the same thing at the same time, by the mechanism every other change uses.
What comes back is the backend's answer in every particular.
A shell learns only that the answer moved.

**Neither side owes** a compatibility shim for the other's absence.
A shell that cannot reach a backend and cannot start one says so.
A backend without a shell keeps publishing.
