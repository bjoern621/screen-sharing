using ScreenShare.Api.V1;

namespace ScreenShare.App.Backend;

/// <summary>
/// The control plane as this shell sees it: the reads that draw a screen, the effects, and the stream that
/// says what changed.
/// The seam <c>docs/ipc-api.md</c> draws.
/// Which values a control offers, which are greyed and why, what a configuration is predicted to cost, what
/// is publishing and what the relay carries all arrive through here already decided.
/// The shell contributes layout, typography, colour, motion, input handling, accessibility and every word on
/// screen.
///
/// Three kinds, grouped below in that order.
/// Reads hand the shell something to draw and change nothing.
/// Effects are the only members that change the world.
/// The stream carries what changed, including what this shell did not do.
///
/// An interface so the flow runs against a fixture in a test as well as against
/// <see cref="ControlBackend"/> over the local socket, and the view layer is written once.
///
/// Every read is asynchronous and cancellable because the transport is gRPC over a named pipe or a Unix
/// socket, and a synchronous signature could only be honoured by blocking the thread that draws the window.
/// A stand-in answers from memory on an already-completed task, so two implementations differ in latency and
/// in nothing else.
///
/// <c>ResolveForm</c> is cheap enough to call on every keystroke, so a keystroke routinely arrives while the
/// previous draft's answer is still in flight, and cancelling the older call is what stops the shell paying
/// for an answer it is about to discard.
/// Cancellation is cooperative and can lose that race, so a caller that must be right when it does needs a
/// guard of its own.
///
/// A failure the app has to survive arrives as <see cref="BackendUnavailableException"/> carrying the
/// backend's own prose.
/// A call this shell abandoned arrives as <see cref="OperationCanceledException"/> and reaches no screen.
/// </summary>
public interface IBackend
{
    /// <summary>
    /// Raised when what the backend would answer has moved, so a caller reads again.
    /// It carries nothing by rule: a shell renders whole states and never applies a delta
    /// (<c>docs/ipc-api.md</c>, "Events"), so a signal carries the news and a read carries the state.
    ///
    /// The encoder probe landing is one raiser.
    /// <c>ResolveForm</c> reads what has been probed rather than probing, so a form resolved before the probe
    /// greys no codec for missing hardware and one resolved after it greys with what is missing named.
    ///
    /// Raised on whichever thread the transport completed on, so a subscriber writing a bound property
    /// marshals it back to the UI loop itself.
    /// </summary>
    event Action? Changed;

    /// <summary>
    /// The stored settings, as <c>GetSettings</c> answers.
    /// A caller never constructs a <see cref="Settings"/>: it changes the one field the user moved on the
    /// draft it received and sends the whole message back.
    /// </summary>
    Task<Settings> SettingsAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Every fixed fact about this machine and the encoding model, as <c>GetCatalog</c> answers: which codecs
    /// exist and what each produces, what the screens are, what each transport carries.
    ///
    /// Read to explain, never to decide: which values a control may offer is the form's answer.
    /// Naming a codec takes the format and the family off its row, and naming a screen the size and refresh
    /// rate of the output at that index (<c>docs/ipc-api.md</c>).
    ///
    /// It moves when the encoder probe lands, which <see cref="Changed"/> announces.
    /// </summary>
    Task<Catalog> CatalogAsync(CancellationToken cancellation = default);

    /// <summary>
    /// The complete description of the screen for one draft, as <c>ResolveForm</c> answers.
    /// Side-effect free and idempotent: one draft resolves to one form, so a caller skips the round trip
    /// while the draft has not moved since the one the form on screen was resolved against.
    ///
    /// The answer carries a possibly repaired draft, which the caller adopts wholesale, and that is what
    /// keeps a greyed option and its replacement from disagreeing.
    /// A repair is a walk to the first legal value rather than a suggestion, so the returned draft resolves
    /// to the form that carried it.
    /// </summary>
    Task<Form> ResolveFormAsync(Settings draft, CancellationToken cancellation = default);

    /// <summary>
    /// Whether a stream is in force, what it is carrying, and whether the held settings have moved off it.
    /// Read when a screen mounts; the same message arrives on the stream thereafter, so a window that has
    /// just opened and one that has been open cannot show different things.
    /// </summary>
    Task<PublishState> PublishStateAsync(CancellationToken cancellation = default);

    /// <summary>
    /// The latest relay snapshot.
    /// Read rather than fetched: the backend owns the polling, so several shells reading it do not multiply
    /// the requests and the byte-delta bitrates stay computed against one steady interval.
    ///
    /// An unreachable relay is a snapshot whose <c>reachable</c> is false carrying the reason, never a failed
    /// call.
    /// </summary>
    Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default);

    /// <summary>
    /// The external viewers open, one entry per stream and the transport it is received over.
    /// A stream name alone is no identity, because the relay re-serves each stream on all its listeners.
    /// </summary>
    Task<IReadOnlyList<WatchKey>> WatchingAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Every stream the backend is decoding, with what the pipeline behind each turned out to be.
    /// Read when a screen mounts; the same state arrives on the stream thereafter, including when a stream's
    /// first frame settles what it negotiated.
    /// </summary>
    Task<IReadOnlyList<ReceiveStream>> ReceivingAsync(CancellationToken cancellation = default);

    /// <summary>
    /// The configurations the user saved, as <c>ListPresets</c> answers, with the notice saying why the store
    /// holds fewer than it did.
    ///
    /// The one read here whose answer never arrives on the stream.
    /// Presets are a file rather than state the backend runs on, so no event carries them and a caller reads
    /// again after saving or deleting one, and again to see what another window did.
    /// The read is idempotent and cheap, which is what makes asking again the whole remedy.
    ///
    /// A store that could not be read is not a failed call.
    /// The list comes back empty carrying the notice, because nothing-was-saved and
    /// nothing-readable-remained are different facts.
    /// </summary>
    Task<PresetStore> PresetsAsync(CancellationToken cancellation = default);

    // --- Effects ------------------------------------------------------------------
    //
    // Every effect that changes state the backend holds answers with nothing.
    // What the state became arrives on the stream, the one path into the display: a shell that read the
    // answer here and a shell that only listened would be two ways of learning one fact (docs/ipc-api.md,
    // "Events").
    // The two preset effects are the written-down exception, for the reason PresetsAsync gives: the file they
    // change is described by no event, so the caller re-reads it instead.

    /// <summary>
    /// Persists the settings and starts the encoder on them.
    ///
    /// The whole draft crosses rather than a reference to something the backend holds, because the draft is
    /// what the reader configured and the stored settings are only what the last commit left behind.
    ///
    /// Refused while a stream is already in force, a retry backoff included, and a combination no engine can
    /// build is refused before anything is launched.
    /// Both arrive as a <see cref="BackendUnavailableException"/> carrying the backend's own sentence.
    /// </summary>
    Task StartPublishAsync(Settings settings, CancellationToken cancellation = default);

    /// <summary>
    /// Persists the settings and restarts the running stream on them.
    /// How an edit reaches a live pipeline, and a separate method from a start because the two are different
    /// intentions about different worlds: put a stream on the air, and change the one already there.
    /// The whole draft crosses, for the reason <see cref="StartPublishAsync"/> gives.
    ///
    /// It names a transition on purpose, and is the one effect here that a repeat does not leave alone.
    /// Both engines run a child built from an argv and neither takes a value back once it is running, so
    /// applying tears the pipeline down and launches another, and a second call is a second restart rather
    /// than a state that already holds.
    /// Written down as one of two departures in <c>docs/development-principles.md</c>, "Effects across a
    /// process boundary".
    /// Nothing on this contract is live-safe, which is why the broadcast screen's quality track is inert and
    /// carries the reason rather than being wired to this.
    ///
    /// With nothing publishing there is no pipeline to apply to, and the backend refuses rather than quietly
    /// starting one: a stream the reader had stopped coming back is not what they asked for.
    /// A combination no engine can build is refused before anything is torn down, so the stream goes on
    /// carrying what it has.
    /// Both arrive as a <see cref="BackendUnavailableException"/> carrying the backend's own sentence.
    /// </summary>
    Task ApplyToStreamAsync(Settings settings, CancellationToken cancellation = default);

    /// <summary>
    /// Persists the settings and touches nothing that is running.
    ///
    /// How a setting reaches the backend without putting a stream on the air, which is what the viewer's
    /// watch settings need: how a tile receives and what converts its frames govern the next decode this
    /// machine opens, and a reader who is watching and not sending has no publish to persist them through.
    /// A decode already running keeps the pipeline it started with, since both engines build a child from an
    /// argv and neither takes a value back afterwards.
    ///
    /// It names a state and is safe to repeat: saving settings that are already the held ones is that state
    /// holding rather than a second write, so a caller whose answer went missing asks again
    /// (<c>docs/development-principles.md</c>, "Effects across a process boundary").
    /// </summary>
    Task SaveSettingsAsync(Settings settings, CancellationToken cancellation = default);

    /// <summary>
    /// Stores one way of publishing under a name, replacing whatever was saved under it.
    ///
    /// A preset is a <see cref="PublishSettings"/> and nothing else (<c>docs/presets.md</c>).
    /// Where the relay is belongs to a deployment and how this machine watches belongs to a viewer, so a
    /// preset carrying either would be the thing that breaks on the machine it was copied to.
    ///
    /// It names a state and is safe to repeat.
    /// The name is the identity, so a second save of the same settings under the same name leaves the store
    /// holding what it held, and saving over a preset is the same call as making one.
    ///
    /// An empty name is refused as a <see cref="BackendUnavailableException"/> carrying the backend's own
    /// sentence, as is a store that could not be written.
    ///
    /// No event follows: the caller reads <see cref="PresetsAsync"/> again.
    /// </summary>
    Task SavePresetAsync(string name, PublishSettings settings, CancellationToken cancellation = default);

    /// <summary>
    /// Removes the preset saved under a name.
    ///
    /// A name the store does not hold is refused rather than shrugged at, which is the backend's decision and
    /// the one place presets depart from naming a state (<c>backend/internal/control/effects.go</c>).
    /// So deleting a preset another window has already removed answers with a sentence, and reading the store
    /// again is what puts the screen back in step.
    ///
    /// No event follows: the caller reads <see cref="PresetsAsync"/> again.
    /// </summary>
    Task DeletePresetAsync(string name, CancellationToken cancellation = default);

    /// <summary>Ends the stream, running or waiting out a retry backoff.</summary>
    Task StopPublishAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Measures this machine's real upload throughput, in Mbit/s, so a guessed uplink figure can be replaced
    /// by one that was observed.
    ///
    /// An effect, and the one that answers with something: it uploads a payload and times it, changing no
    /// state the backend holds, so there is nothing for an event to carry.
    /// What the caller does with the figure is a settings write like any other, which is what keeps a
    /// measured uplink and a typed one one value.
    ///
    /// It runs the real thing and takes seconds.
    /// Refused outright while a stream is publishing, as a <see cref="BackendUnavailableException"/> carrying
    /// the backend's own sentence, because a measurement would compete with the stream for the line.
    /// </summary>
    Task<double> MeasureUplinkAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Opens an external viewer for one stream over one transport.
    /// A leg that cannot carry the stream's format is refused with the format named, rather than opening a
    /// viewer that connects and receives nothing.
    /// </summary>
    Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default);

    Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default);

    /// <summary>
    /// Opens the relay's own player page for one stream in the machine's default browser, over one of the
    /// legs the relay serves a page for (<see cref="ScreenShare.Api.V1.Catalog.BrowserWatchTransports"/>).
    /// A leg the stream's format does not cross is refused the way <see cref="StartWatchAsync"/> refuses one.
    ///
    /// Nothing is opened that can be closed again.
    /// The tab belongs to the browser, so no viewer state grows a member and there is no counterpart call: a
    /// control for it names an action rather than a state, and shows no tick.
    ///
    /// A repeat opens the page again, a departure from the idempotency the rest of this contract holds to
    /// (<c>docs/development-principles.md</c>).
    /// The effect lands in a program neither side owns, so there is no state a second call could read back to
    /// decide it had already happened.
    /// </summary>
    Task OpenInBrowserAsync(string streamName, string transport, CancellationToken cancellation = default);

    /// <summary>
    /// Opens a decode for one stream on one leg, inside the backend.
    /// The tile path's counterpart of <see cref="StartWatchAsync"/>, and the difference is where the frames
    /// end up: a watch launches a player window the backend does not draw in, a receive decodes into the
    /// backend, from where the frame channel hands the frames to this shell.
    ///
    /// What it opens is a decode and not a tile.
    /// Nothing about a grid, a layout or a window crosses here, because how a viewer arranges what it
    /// receives is this shell's whole job (<c>docs/ipc-api.md</c>).
    ///
    /// A leg that cannot carry the stream's format is refused with the format named, as a
    /// <see cref="BackendUnavailableException"/> carrying the backend's own sentence.
    ///
    /// <paramref name="toneMap"/> asks for an HDR stream to be rolled down into the range a standard display
    /// shows, and is part of what the decode is built from rather than a value written to a running one.
    /// So this call is also how the answer is changed: a decode running with the other one is rebuilt, and
    /// one already running with this one is the state this names and costs nothing.
    /// It is stored nowhere and lives exactly as long as the decode, because a preference kept per stream
    /// would outlive the stream it was made about.
    ///
    /// A machine with no element that rolls the range down builds the decode without one and reports that it
    /// did, rather than refusing.
    /// </summary>
    Task StartReceiveAsync(
        string streamName, string transport, bool toneMap = false, CancellationToken cancellation = default);

    /// <summary>
    /// Closes one running decode.
    /// A pair nothing is decoding is not an error: the state it names already holds.
    /// </summary>
    Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default);

    /// <summary>
    /// Sets how loud one decode plays and whether it plays at all.
    /// <paramref name="volume"/> is linear gain from zero, one being the stream unchanged.
    /// Above one amplifies, and the backend bounds it rather than letting a slider clip the output device.
    /// <paramref name="muted"/> silences the branch without losing the volume, so unmuting returns to the
    /// level the reader chose.
    ///
    /// The loudness belongs to the decode, not to a window drawing it.
    /// The audio branch is one element inside one pipeline, playing on the machine the shell is on rather
    /// than travelling over the frame channel (<c>docs/viewer-architecture.md</c>), so two windows on one
    /// decode share it and a per-window volume would be several controls over one thing.
    ///
    /// Mute travels with the volume because the two are one state: sent apart, a caller that muted and then
    /// set a volume would have to know whether the second undid the first.
    ///
    /// Safe to repeat and safe to send early.
    /// A decode already at that loudness is a state that holds, and a volume set before the decoder has
    /// exposed an audio pad is applied when it does.
    /// A decode that is not running is refused as a <see cref="BackendUnavailableException"/>, that being a
    /// request about something absent.
    /// </summary>
    Task SetReceiveAudioAsync(string streamName, string transport, double volume, bool muted, CancellationToken cancellation = default);

    /// <summary>
    /// Subscribes to the frames of a decode that is already running.
    ///
    /// It opens no decode, which is why it can be refused: a caller runs <see cref="StartReceiveAsync"/>
    /// first and subscribes once that has answered.
    /// The two staying separate is what lets a decode outlive every tile drawing it, so a window that closed
    /// does not stop a stream and one that opened again finds it running.
    ///
    /// Several subscriptions to one decode each get a pool of their own, so a consumer that is slow to draw
    /// cannot hold a buffer another one is waiting for.
    /// </summary>
    Task<FrameChannel> OpenFramesAsync(string streamName, string transport, CancellationToken cancellation = default);

    /// <summary>
    /// Subscribes to the frames of the running publish's local preview: the stream this machine is sending,
    /// decoded from a copy the publish child writes to a loopback port rather than read back off the relay
    /// (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws").
    ///
    /// It names nothing, and both halves of that are the point.
    /// There is at most one publish, so the preview needs no identity of its own.
    /// Its frames crossed no protocol, so a synthetic leg would put a transport in the table every consumer
    /// reads that nothing could act on.
    ///
    /// It opens no pipeline.
    /// The publish is what brings the preview up, so a call made with nothing publishing is refused as a
    /// <see cref="BackendUnavailableException"/> carrying the backend's own sentence.
    /// A caller reads <c>PublishState</c> to know whether there is one to ask for.
    /// </summary>
    Task<FrameChannel> OpenPreviewFramesAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Reads one of this machine's monitors into a picture the frame channel can hand over, so the setup
    /// wizard offers a screen by what is on it rather than by its number.
    /// <paramref name="monitor"/> is the index the catalog enumerates the outputs under.
    ///
    /// Nothing is encoded and nothing is sent anywhere.
    /// The capture element feeds the render chain inside the backend and the handles come here, so this costs
    /// one screen read and no bandwidth.
    ///
    /// An effect, and the frame channel opens none of it, which is the division
    /// <see cref="StartReceiveAsync"/> draws: a subscription finds a picture or is refused.
    ///
    /// Idempotent.
    /// A monitor already being previewed is the state this asks for, so a second call changes nothing.
    /// A machine whose session cannot read one screen apart from another refuses every call, which a caller
    /// reads off <c>Catalog.NoMonitorPreview</c> instead of discovering by asking.
    /// </summary>
    Task StartMonitorPreviewAsync(int monitor, CancellationToken cancellation = default);

    /// <summary>
    /// The monitors the backend is reading, and whether a frame has come off each one.
    ///
    /// Read when a shell connects, for the reason the running decodes are: a preview outlives the window that
    /// asked for it, so a shell that crashed with screens being read leaves them running and this is how the
    /// next one finds them.
    /// The same shape arrives on the event stream whenever it moves.
    /// </summary>
    Task<IReadOnlyList<PreviewedMonitor>> PreviewedMonitorsAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Closes one monitor's preview.
    /// A monitor nothing is previewing is not an error: the state it names already holds.
    /// </summary>
    Task StopMonitorPreviewAsync(int monitor, CancellationToken cancellation = default);

    /// <summary>
    /// Subscribes to the frames of a monitor preview that is already running.
    /// It opens no capture, for the reason <see cref="OpenFramesAsync"/> opens no decode.
    /// A monitor nothing is previewing is refused as a <see cref="BackendUnavailableException"/>, and a
    /// caller reads the preview state to know whether to ask at all.
    /// </summary>
    Task<FrameChannel> OpenMonitorFramesAsync(int monitor, CancellationToken cancellation = default);

    /// <summary>
    /// Opens one run log in the machine's default application.
    /// <paramref name="path"/> is one the backend handed out on an <see cref="ExitInfo"/> and is never
    /// constructed here: the backend writes these files and rotates them, so it is the only side that knows
    /// which of them still exist under the name it gave out.
    ///
    /// A repeat opens the log again, the departure <see cref="OpenInBrowserAsync"/> states.
    /// </summary>
    Task OpenLogAsync(string path, CancellationToken cancellation = default);

    /// <summary>
    /// Opens the directory holding the run logs.
    /// A repeat opens it again, the departure <see cref="OpenInBrowserAsync"/> states.
    /// </summary>
    Task OpenLogsFolderAsync(CancellationToken cancellation = default);

    // --- Stream -------------------------------------------------------------------

    /// <summary>
    /// What changed, for as long as the caller holds the enumeration.
    ///
    /// Every event carries a whole state, never a delta.
    /// A caller receiving <c>PublishState</c> renders it rather than applying it to something it was holding,
    /// so a duplicate event is harmless and a dropped connection is recovered from by reading the state
    /// again, not by replaying history.
    ///
    /// It carries the changes this shell did not make, which is the whole reason it exists: another window's
    /// stop, a pipeline that died on its own, a viewer that closed, an encoder probe that landed.
    /// </summary>
    IAsyncEnumerable<Event> SubscribeAsync(CancellationToken cancellation = default);

    /// <summary>
    /// How loud every decode carrying audio is, at fifteen ticks a second coalesced to the newest, for as
    /// long as the caller holds the enumeration.
    ///
    /// A second stream rather than an event kind, and the difference is cadence.
    /// <see cref="SubscribeAsync"/> carries whole states when something changed; a level changes
    /// continuously, and folding it in would push the receive state at metering rate and make every consumer
    /// of that state re-render for a figure none of them reads.
    ///
    /// Each tick is the whole set, like every other state here.
    /// A decode carrying no audio has no entry, and a silent one has an entry reading negative infinity: two
    /// facts a tile draws differently, as no meter at all and as an empty meter.
    ///
    /// One enumeration covers every decode, so a tile appearing needs no second subscription and a tile
    /// leaving needs no cancellation.
    /// </summary>
    IAsyncEnumerable<AudioLevels> SubscribeAudioLevelsAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Where the publishing machine's pointer is, for as long as the caller holds the enumeration.
    ///
    /// A stream of its own for the reason the levels are one, and one degree more so: the whole point of
    /// sending a position instead of drawing it into the picture is that it costs no frame, so it moves at
    /// its own rate rather than the stream's.
    ///
    /// It carries positions only while a publish whose cursor mode sends them is running, and stays open and
    /// silent otherwise.
    /// The mode can change under a subscription, and a shell that had to resubscribe would be one holding a
    /// pointer from the mode before.
    /// </summary>
    IAsyncEnumerable<PointerPosition> SubscribePointerAsync(CancellationToken cancellation = default);
}
