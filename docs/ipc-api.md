# The control API

One Go process owns the product. Every user interface is a display in front of it.

The contract between them is `api/proto/screenshare/v1`, and this page states the rule that contract encodes, the reasoning behind the shape it takes, and what each side owes the other.

## The rule

**The backend decides what is true. A shell decides how to say it, and asks the backend to act.**

Concretely, and without exception:

- Every value a control can offer comes from the backend. Codecs, pixel formats, rate-control modes, transports, capture backends, frame rates, output resolutions, audio codecs, encoder presets, frame-memory values, monitors, watch legs - a shell holds no list of its own for any of them.
- Every greyed option and every disabled field is decided in Go and arrives already decided, together with a **code** naming why. The four treatments in `field-availability.md` are a fixed consequence of the tables, not a per-shell judgement.
- Every derived figure - the bitrate estimate, the headroom against the uplink, the command preview - comes from the backend.
- Every piece of state is read from the backend or received on its event stream. A shell caches nothing across a change notification.
- **Every word on screen is the shell's.** Labels, help text, option names, the paragraph behind a choice, the sentence in place of a greyed entry, the shorthand a step chip repeats, how a unit is spelled and where it sits - all of it is written where the layout is, keyed by the identifiers the backend sends (`text.proto`).
- Beyond the words, a shell contributes layout, typography, colour, motion, input handling and accessibility. That is its whole job, and it is not a small one.

**The two halves meet on identifiers and nowhere else.** `hevc_nvenc`, `gstreamer`, `yuv420p` and `srt` cross the wire because they are what the encoder, the element registry and the relay themselves call these things. `HEVC / H.265 · NVIDIA GPU` is one shell's answer to how that reads at its width, in its typography, for its reader.

This is the one rule that moved. The contract used to carry the sentence as well as the fact, and the argument for it was that a shell writing its own label would have to know what the value means. That was answering the wrong question: a shell looking a name up by identifier knows no more about a codec than a shell printing one the backend sent, and it is the only side that can see the column it has to fit in, the tone it is written in, and the language its reader speaks. A backend that ships wording decides all of that from a place that can see none of it - and every shell then either accepts a sentence built for another layout or quietly writes its own, which is the fork this contract exists to prevent, arrived at from the other side.

The methods a shell may call divide into three kinds, and the division is the rule in executable form:

| Kind | What it does | Examples |
| --- | --- | --- |
| Reads | Hand the shell something to draw. They compute; they change nothing, and they are cheap enough to call on a keystroke or a mount. | `GetCatalog`, `ResolveForm`, `GetPublishState`, `GetRelayStatus` |
| Effects | Do the one thing the user asked for. The only methods that change the world. | `StartPublish`, `ApplyToStream`, `StopPublish`, `StartWatch`, `SaveSettings`, `ProbeEncoders`, `MeasureUplink` |
| Stream | Carries what changed, including what this shell did not do. | `Subscribe` |

A method that is neither a read nor one of the listed effects does not belong on this service, and a shell that wants one is a shell about to hold state the backend owns.

The measurements are effects and not a fourth kind, which is the test applied rather than a category granted. `ProbeEncoders`, `MeasureUplink` and `MeasureEncodeRate` each start real work, take seconds, and leave behind a result a later read answers from - the probe most of all, since what it finds is what every subsequent `ResolveForm` greys codecs against. `ProbeEncoders` was a `probe_encoders` flag on `GetCatalog` until the same test was applied to it: a read that replaced the answer a different shell's next resolve would give, with nothing on the wire to announce it. It is now a method of its own, and what it finds reaches every shell on the event stream.

## Why

The app already has two shells, is growing a third, and the third is meant to replace both (`avalonia/README.md`).
Three shells with three copies of the domain model is three chances to disagree with the encoder, and the disagreement is invisible until it is a stream nobody can explain.

The failure is not hypothetical. The Wails frontend today holds `util/domain.ts`, `util/deps.ts` and `util/options.ts`: labels, tooltips, coding efficiencies, mode metadata, engine rules, the dropdown lists and the greying logic, all in TypeScript, all derived from tables that live in Go and cross the wire as raw rows.
That split was defensible with one frontend. With three it means the same rule is written three times in three languages, and `docs/domain-model.md` exists precisely because a rule written twice drifts.

There is a second reason, and it is the one that decides the shape rather than the direction. The capture, encode and publish pipeline stays in Go, and frames will cross to the shell as shared GPU handles.
The shell therefore already talks to one Go process about frames; giving it a second, private idea of what a codec is would mean the process holding the encoder and the process drawing the encoder's settings can disagree about which encoders exist.

The rule also buys something the current design cannot have: a shell that decides nothing can be replaced outright, or driven by a test with no UI at all, because everything it displays was computed somewhere a test can reach.

**The backend and the shell always run on the same machine.** The split is development separation - one owner of the domain model, and shells that can be replaced without it - and not a step toward running a shell remotely. It is not a trust boundary either: the shell is a second module of the same application on the same box, so nothing in this contract is shaped to defend against a hostile client. A filesystem path crosses as a path. An argument for a design that begins "a shell may one day not be on this machine" is not an argument, and this page used to make one.

## What crosses, and in what shape

`api/proto/screenshare/v1` holds six files.

| File | Holds |
| --- | --- |
| `settings.proto` | `StreamSettings`, the one settings shape on the wire, and `Preset` |
| `catalog.proto` | the fixed facts: codecs, decoders, audio codecs, encoder availability, GPU paths, capture backends, transport carriage, monitors, platform |
| `text.proto` | `Text`: one statement the backend makes, as a code and the identifiers it is about. Nothing on this contract is a sentence |
| `form.proto` | `Form`: every field, option, greying, reason, note, warning and derived figure, already decided |
| `session.proto` | the running state: publish state, encoder samples, relay snapshot, viewers |
| `events.proto` | `Event`, the server-push envelope |
| `control.proto` | `ControlService`, the whole callable surface |

**What is not here is as much of the rule as what is.** The contract describes no grid, no tile, no window layout and no widget arrangement. How a viewer arranges the streams it receives is the shell's whole job, alongside layout, typography, colour, motion, input handling and accessibility - the list above is exhaustive in both directions.

The contract carried a grid until recently: `StartGrid`, `GridState`, a `grid_transports` list and a `grid_transport` setting, all describing the GTK4 window in `nativegrid/`. That window and the Wails app that launched it are obsolete, and the surface went with them rather than being renamed, because renaming it would have kept the mistake and spelled it better. A viewer that decodes through a receiving pipeline will want a watch leg and a jitter buffer again; both are declared then, against a viewer that reads them, and neither comes back as a layout.

`Form` is the one to read first, because it is what makes a shell a display.
`ResolveForm` takes a settings draft and returns the complete description of the screen: the groups, the fields in order, each field's control kind and unit, whether it is visible and enabled and why not as a `Text`, its current value, and for a select or a radio, every option with its value, its note, its enabled flag and its reason.
A shell renders that and sends back a changed `StreamSettings`. It evaluates no rule, and it writes every word.

`ResolveForm` has no side effect and is cheap enough to call on every keystroke.
It returns a possibly repaired draft alongside the form, and names the fields it repaired: where the sent draft held a value the tables forbid, the backend walked to the first legal one, exactly as `normalize` does today.
A shell adopts the returned settings wholesale, which is what keeps a greyed option and its replacement from disagreeing.

## The format, and why this one

**Protocol Buffers for the schema, gRPC for the calls, a local socket for the transport.**

The schema needs to be a machine-checkable artifact, not prose, because the whole point is that two languages derive from one definition.
That rules out a hand-written JSON contract: it would have a Go struct and a C# class, both hand-maintained, which is the drift this is meant to end.

Between the schema languages that generate both sides:

- **Protobuf** generates Go and C# from one file, both first-class. `Grpc.Tools` compiles `.proto` during the .NET build, so the C# side has no checked-in generated code and no step that can be skipped on one side only. Field numbering makes compatible evolution a mechanical property rather than a matter of care, and `buf breaking` enforces it in CI.
- **OpenAPI** describes an HTTP surface. Server-push needs a second mechanism bolted on (SSE or WebSocket), the C# generators are third-party and uneven, and the wire is JSON, which is the wrong shape for per-second encoder samples.
- **JSON-RPC over a pipe** is the smallest thing that works and has no schema at all. Enums would live in two hand-written places, which is the failure mode this exists to remove.

gRPC over protobuf then comes with the two things a hand-rolled framing would have to reinvent badly: server streaming, which is exactly the event channel, and a status model with deadlines and cancellation.

**The transport is local IPC and not a network port.**

| Platform | Endpoint |
| --- | --- |
| Windows | named pipe `\\.\pipe\screenshare-control-v1`, DACL restricted to the owning user |
| Linux, macOS | Unix socket `$XDG_RUNTIME_DIR/screenshare/control-v1.sock`, mode `0600`, falling back to the user's config directory where `XDG_RUNTIME_DIR` is unset |

No TCP listener, not even on loopback: a loopback port is reachable by every process on the machine and by anything the browser can be persuaded to fetch, and this service starts screen captures.
The socket path is the only discovery mechanism; a shell that cannot open it reports that the backend is not running.
Both ends use gRPC over that stream - Go dials the pipe with a custom dialer, .NET with `SocketsHttpHandler.ConnectCallback` - so nothing about the service definition changes with the platform.

**Video frames do not cross this API.** They are the separate problem `avalonia/README.md` states: shared GPU handles, a buffer-ownership protocol, per-frame fences, release-back messages, and what each side does when the other dies. This contract carries control and description; it will name the frame channel once that channel exists, and it will not carry pixels.

## Errors

The repository's two error kinds (`development-principles.md`) cross this boundary differently, and the difference is the whole error model.

An **Umgebungsfehler** - a condition the app must survive - is a gRPC status. It is expected, it is the user's to see, and its message is prose written for a person:

| Status | Means |
| --- | --- |
| `INVALID_ARGUMENT` | the request names something that cannot exist: an unknown transport, a stream name that is empty |
| `FAILED_PRECONDITION` | the request is well formed and the world is not ready for it: already publishing, nothing to apply settings to, a measurement while a stream is live |
| `UNAVAILABLE` | the relay could not be reached, or the child process could not be started |
| `NOT_FOUND` | a named preset, log or stream does not exist |
| `RESOURCE_EXHAUSTED` | a bounded resource was over-asked, such as the test-stream count |

An **Entwicklungsfehler** - a broken internal contract - never crosses. `assert` panics in the backend, as it does everywhere else in this repository. A shell that could receive a bug as a status would start handling bugs, and a handled bug is a bug that ships.

One consequence is worth stating on its own: **an unreachable relay is not a call failure.** `GetRelayStatus` succeeds and returns a snapshot whose `reachable` is false, carrying the reason, because "the relay is down" is a thing the screen has to say rather than a thing the call failed at.

## Events

Two rules keep the event stream from becoming a second definition of the state.

**Every event carries a whole state, never a delta.** A shell receiving `PublishState` renders it; it does not apply it to something it was holding. A duplicate event is therefore harmless and a dropped connection is recovered from by reading state again, not by replaying history.

**A shell that acted still waits for the event.** `StopPublish` answers with an empty message. What the state became arrives on the stream. One path into the display is what stops the window that pressed the button and the window that did not from showing different things - which is the same reason `emitPublishState` exists in the current backend.

## Versioning

The package is `screenshare.v1` and the directory says so, which is what makes a `v2` a new directory rather than an edit.

Within v1, evolution is additive: new fields take new numbers, no number is reused, no field changes type or meaning, and `buf breaking` checks it against the previous commit.
`Hello` is the first call on a connection and settles the major version before any other method is reached, so a mismatch is a sentence naming both versions rather than a field that silently arrives empty.
The minor version is informational: a shell built against a lower minor works, and one built against a higher minor may find a method missing.

## Fields the contract leads

Defining the API first means the contract can name something the backend does not do yet. Two such places have since been closed, and neither left a shell to be edited:

- **`Catalog.audio_sources` was one.** The audio source was a settings field whose values no Go table enumerated: the frontend knew `none` and `desktop` because someone typed them into `util/domain.ts`, and why Windows and macOS could not have the second because someone typed that into `util/deps.ts` as well. Which capture sources a machine offers is the platform's answer, so the list is now `platform.AudioSources`, a table of rows rather than a slice of strings: each names the settings value, the operating systems whose sessions serve it, what serves it on each, and what an operating system that does not serve it is missing (`domain-model.md`, "The second-track capture sources").
  The move was worth making twice over, because the first attempt did not make it. Two strings copied from TypeScript into a Go function that ignored its `platform.Info` argument answered the same on every operating system, which is a constant wearing a platform's name; the table answers differently because the engines do. Both of them open desktop audio as the PulseAudio/PipeWire monitor of the default sink and neither has anything to open where no such server runs, so Linux serves it and the other two are told what they are missing.
  This field carries what this machine serves, which is one entry shorter away from Linux. The reason a source is out of reach is a sentence a screen shows, so it arrives on the form's own option instead, and the same rows decide both: the form offers every declared source and greys the ones the machine does not serve, because a general concept a machine blocks is taught by a greyed entry and its reason rather than by a control that is quietly one item shorter (`field-availability.md`, "The rule").
  Nothing here touches the machine. A source is declared and not enumerated, so a resolve reads a table and pays nothing for it. Should a probe ever enumerate real devices, it is cached for the process lifetime and the cached read is kept separate from the probing one, the way `ControlService`'s backend divides `Encoders` from `CachedEncoders`: a resolve reads what is known now, and an unenumerated machine is one nothing is greyed on rather than one with no audio.

**`StreamSettings.output_resolution` was the other, and it is the case worth keeping on this page.** It was declared before anything could honour it: both shells drew an output resolution, the Go pipeline had no scaling stage, and `ResolveForm` answered with the field disabled and that as its reason. The scaling stage has since landed - a `scale` filter on the ffmpeg software path, the size on the device conversion's own filter where the frames never leave the GPU, and `videoscale` plus a size on the encoder input caps for GStreamer - and the field became an ordinary one.

Nothing above the backend changed in the move. The ladder the dropdown offers is the one it always offered, the shell that drew it greyed is the shell that draws it live, and what decides which of the two is on screen is still the backend's answer and never the shell's knowledge. One case survives as a greying rather than a feature: a pair whose device path carries no conversion at all - the encoder reading captured surfaces directly - has nothing on that path that can resize, so the scaled entries grey with the frame memory named as the way across.

That is the discipline the contract-first order buys. A control the shell invented and the backend has never heard of is a lie on screen; a control the contract declares and the backend disables is a fact; and a control the backend later implements needs no shell to be edited.

## What each side owes

**The backend owes** one `ControlService` implementation, and every rule that today lives in `frontend/src/util` moved into Go beside the tables it derives from: the greyings and repairs of `deps.ts`, the dropdown construction of `options.ts`, the prediction of `estimate.ts`, the viewability verdicts, and the preset search.
Once moved, `domain-model.md`'s list of what derives from the tables has one consumer instead of one per shell.

It owes a **code** rather than a sentence for every one of those verdicts, and the identifiers behind it. A greying that arrived as prose would be a rule and a wording travelling together, and the wording is the half no backend can get right for a surface it has never seen.

**A shell owes** a name for every identifier the backend can send, and a sentence for every statement it can make. A value with no name renders as the raw identifier, which is honest and still a defect; a statement with no sentence renders as its code, which is worse and is meant to be. Both are visible on screen rather than swallowed, because that is what gets them written. In the Avalonia shell this is `ScreenShare.App/Copy`.

It owes the render-function discipline it already follows (`development-principles.md`): one function per component mapping received messages to widget state, every output set on every pass, nothing cached that the backend owns.
It owes debouncing, because `ResolveForm` on every keystroke is cheap but `SaveSettings` on every keystroke is a file write.
And it owes the honesty of showing a disabled field rather than inventing an enabled one.

It also owes **asking for the encoder probe once.**
`ResolveForm` reads what has been probed rather than probing, for the reason the method's own contract states: a resolve is called on every keystroke and the probe test-encodes on every engine.
The consequence is a difference somebody has to close rather than one that can be ignored. On a machine nothing has asked about, no codec is greyed for missing hardware - which is the honest reading of a fact nobody established, and is also a dropdown offering QSV on a machine with no Intel GPU.
So a shell calls `ProbeEncoders` once, in the background, and goes on drawing what it has.

**Reading again is not owed, and that is the point of the split.** The probe used to be a flag on `GetCatalog`, and a shell that asked for it was the only shell that learned the answer: every other one went on greying nothing, or - worse - watched its next resolve start greying codecs with nothing having told it why.
`ProbeEncoders` announces the whole catalog on the event stream, so the shell that asked and the shell that did not are told the same thing at the same time, by the same mechanism every other change already uses.
What comes back is still the backend's answer in every particular - which encoders are missing, and the sentence naming what is missing about each. A shell learns only that the answer moved.

**Neither side owes** a compatibility shim for the other's absence. A shell without a backend shows that the backend is not running. A backend without a shell keeps publishing.
