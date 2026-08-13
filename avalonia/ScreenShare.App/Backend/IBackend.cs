using ScreenShare.Api.V1;

namespace ScreenShare.App.Backend;

/// <summary>
/// The control plane, as this shell sees it: the reads it makes to draw a screen, the effects it asks for,
/// and the stream that says what changed.
///
/// It is the seam <c>docs/ipc-api.md</c> draws.
/// Everything the three destinations show - which values a control offers, what each is called, which are
/// greyed and why, what the configuration is predicted to cost, what is publishing, what the relay is
/// carrying - arrives through here already decided.
/// The shell contributes layout, typography, colour, motion, input handling and accessibility, and nothing
/// else.
///
/// The three kinds are the boundary rule in executable form, and they are grouped below in that order.
/// <b>Reads</b> hand the shell something to draw and change nothing.
/// <b>Effects</b> are few, named for the user's intent, and are the only members that change the world.
/// <b>The stream</b> carries what changed, including what this shell did not do.
///
/// The interface exists so the flow can be driven by something other than a running backend:
/// <see cref="ControlBackend"/> over the local socket, and a fixture in a test.
/// Neither changes a line of the view layer, which is the property the rule buys.
///
/// <b>Every read is asynchronous and cancellable, and that is the shape the transport forces rather than a
/// convenience.</b> The implementation this seam exists for is a gRPC client over a named pipe or a Unix
/// socket: the answer arrives off the UI thread after a round trip that can be slow, can fail and can be
/// abandoned.
/// A synchronous signature could only be honoured by blocking the thread that draws the window, which is how
/// a shell that decides nothing still manages to freeze.
/// The stand-in answers from memory and returns an already-completed task, so the two implementations differ
/// in latency and in nothing else, and no caller is written twice.
///
/// The token is the other half of that.
/// <c>ResolveForm</c> is cheap enough to call on every keystroke, which means a keystroke routinely arrives
/// while the previous draft's answer is still in flight; cancelling the older call is what keeps the shell
/// from paying for an answer it is about to discard.
/// A caller that also has to be right when the cancellation loses the race needs a second guard of its own -
/// the token asks, it does not promise.
/// </summary>
public interface IBackend
{
    /// <summary>
    /// Raised when what the backend would answer has moved, so a caller reads again.
    ///
    /// It carries nothing, and that is the rule rather than an omission: a shell renders whole states and
    /// never applies a delta (<c>docs/ipc-api.md</c>, "Events"), so the news that something moved and the
    /// state it moved to are two different things and only the first belongs on a signal.
    /// What a caller does with it is re-read, which is the same path it takes on its first render.
    ///
    /// One thing raises it today: the encoder probe landing.
    /// <c>ResolveForm</c> reads what has been probed rather than probing, so the first forms of a session
    /// grey nothing for missing hardware and the ones after the probe do - a codec whose encoder this machine
    /// cannot run is greyed with what is missing, and until then it is offered because nothing has
    /// established otherwise.
    ///
    /// <b>It is raised on whichever thread the transport completed on.</b> A subscriber that writes a bound
    /// property marshals it back to the UI loop itself, exactly as it does with the answer to a read.
    /// </summary>
    event Action? Changed;

    /// <summary>
    /// The stored settings, as <c>GetSettings</c> answers.
    /// A shell never constructs a <see cref="Settings"/> from nothing: it receives a draft, changes the one
    /// field the user moved, and sends the whole message back.
    /// </summary>
    Task<Settings> SettingsAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Every fixed fact about this machine and the encoding model, as <c>GetCatalog</c> answers: which codecs
    /// exist and what each produces, what the screens are, what each transport carries.
    ///
    /// <b>It is read to explain, never to decide.</b> Which values a control may offer is the form's answer
    /// and is not derived from this; what a value is called is this shell's, and two of those names need a
    /// fact only the catalog carries - a codec is named by the format and the family its row states, and a
    /// screen by the size and refresh rate of the output at that index (docs/ipc-api.md).
    ///
    /// It moves once in a session, when the encoder probe lands, which is what <see cref="Changed"/>
    /// announces.
    /// </summary>
    Task<Catalog> CatalogAsync(CancellationToken cancellation = default);

    /// <summary>
    /// The complete description of the screen for one draft, as <c>ResolveForm</c> answers.
    /// Idempotent and side-effect free: the same draft resolves to the same form, and a caller may therefore
    /// skip the round trip entirely when the draft has not moved since the one the form on screen was
    /// resolved against.
    ///
    /// The answer carries a possibly repaired draft, which the caller adopts wholesale.
    /// That is what keeps a greyed option and its replacement from disagreeing, and the repair is a walk to
    /// the first legal value rather than a suggestion, so the draft that comes back resolves to the form that
    /// carried it.
    /// </summary>
    Task<Form> ResolveFormAsync(Settings draft, CancellationToken cancellation = default);

    /// <summary>
    /// Whether a stream is in force, what it is carrying, and whether the held settings have moved off it.
    /// Read when a screen mounts; the same message arrives on the stream thereafter, which is what stops a
    /// window that has just opened and one that has been open from showing different things.
    /// </summary>
    Task<PublishState> PublishStateAsync(CancellationToken cancellation = default);

    /// <summary>
    /// The latest relay snapshot.
    /// It is read rather than fetched: the backend owns the polling, so several shells reading this do not
    /// multiply the requests, and the byte-delta bitrates stay computed against one steady interval instead
    /// of against whatever cadence each shell chose.
    ///
    /// An unreachable relay is a snapshot whose <c>reachable</c> is false carrying the reason, and never a
    /// failure: "the relay is down" is a thing the screen has to say.
    /// </summary>
    Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default);

    /// <summary>
    /// The external viewers currently open, one entry per stream and transport it is received over.
    /// The stream name alone is not an identity, because the relay re-serves each stream on all its
    /// listeners.
    /// </summary>
    Task<IReadOnlyList<WatchKey>> WatchingAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Every stream the backend is decoding for a tile, with what the pipeline behind each turned out to be.
    /// Read when a screen mounts; the same state arrives on the stream thereafter, including when a stream's
    /// first frame settles what it negotiated.
    /// </summary>
    Task<IReadOnlyList<ReceiveStream>> ReceivingAsync(CancellationToken cancellation = default);

    /// <summary>
    /// The configurations the user saved, as <c>ListPresets</c> answers, and the notice saying why the store
    /// holds fewer than it did.
    ///
    /// <b>Nothing announces that this moved.</b> Presets are a file rather than state the backend runs on: no
    /// event carries them, so a caller that saved or deleted one reads again to see what the store holds now,
    /// and so does one that wants to see what another window did.
    /// It is the one read here whose answer never arrives on the stream, and the read being idempotent and
    /// cheap is what makes asking again the whole of the remedy.
    ///
    /// A store that could not be read is not a failed call.
    /// The list comes back empty carrying the notice, because empty-because-nothing-was-saved and
    /// empty-because-nothing-readable- remained are different facts and the difference is the reader's to
    /// see.
    /// </summary>
    Task<PresetStore> PresetsAsync(CancellationToken cancellation = default);

    // --- Effects ------------------------------------------------------------------
    //
    // Every one of them answers with nothing.
    // What the state became arrives on the stream, which is the one path into the display: a shell that read
    // the answer here and a shell that only listened would otherwise be two ways of learning one fact
    // (docs/ipc-api.md, "Events").
    // The two preset effects are the written-down exception, for the reason PresetsAsync gives: the store
    // they change is a file no event describes, so the caller re-reads it instead.

    /// <summary>
    /// Persists the settings and starts the encoder on them.
    ///
    /// The whole draft crosses rather than a reference to something the backend holds, because the flow's
    /// draft is the thing the reader configured and the backend's stored settings are only what the last
    /// commit left behind.
    /// It is refused while a stream is already in force - a retry backoff included - and a combination no
    /// engine can build is refused before anything is launched, both as a
    /// <see cref="BackendUnavailableException"/> carrying the backend's own sentence.
    ///
    /// It answers with nothing, like every other effect: that a stream is now live arrives on the stream, so
    /// the window that pressed the button and the window that did not learn it the same way.
    /// </summary>
    Task StartPublishAsync(Settings settings, CancellationToken cancellation = default);

    /// <summary>
    /// Persists the settings and restarts the running stream on them.
    /// It is how an edit reaches a live pipeline, and it is a separate method from a start because the two
    /// are different intentions about different worlds: put a stream on the air, and change the one that is
    /// already there.
    ///
    /// The whole draft crosses, for the reason <see cref="StartPublishAsync"/> gives: this is how the
    /// settings the reader configured reach the backend at all, and half of them arriving would leave the
    /// other half at whatever the last commit stored.
    ///
    /// <b>It names a transition on purpose, and is the one effect here that a repeat does not leave
    /// alone.</b> Both engines run a child built from an argv and neither takes a value back once it is
    /// running, so applying tears the pipeline down and launches another - a second call is a second restart
    /// rather than a state that already holds (<c>docs/development-principles.md</c>, "Effects across a
    /// process boundary", which lists this as one of its two written-down departures).
    /// Nothing on this contract is live-safe, which is why the broadcast screen's quality track is inert and
    /// carries the reason rather than being wired to this.
    ///
    /// With nothing publishing there is no pipeline to apply to, and the backend refuses rather than quietly
    /// starting one: a reader who pressed apply asked for the stream they were watching to change, and a
    /// stream they had stopped coming back is a different thing than the one they asked for.
    /// That refusal arrives as a <see cref="BackendUnavailableException"/> carrying the backend's own
    /// sentence, as does a combination no engine can build - which is refused before anything is torn down,
    /// so the stream goes on carrying what it has.
    ///
    /// It answers with nothing, like every other effect: what the stream became arrives on the stream.
    /// </summary>
    Task ApplyToStreamAsync(Settings settings, CancellationToken cancellation = default);

    /// <summary>
    /// Persists the settings and touches nothing that is running.
    ///
    /// It is how a setting reaches the backend without putting a stream on the air, which is what the
    /// viewer's watch settings need: how a tile receives and what converts its frames govern the next decode
    /// this machine opens, and a reader who is watching and not sending has no publish to persist them
    /// through.
    /// Both engines build a child from an argv and neither takes a value back afterwards, so a decode already
    /// running keeps the pipeline it started with - which is the same reason <see cref="ApplyToStreamAsync"/>
    /// is a separate method from <see cref="StartPublishAsync"/>.
    ///
    /// <b>It names a state and is safe to repeat.</b> Saving settings that are already the held ones is that
    /// state holding, not a second write, so a caller whose answer went missing asks again
    /// (<c>docs/development-principles.md</c>, "Effects across a process boundary").
    ///
    /// It answers with nothing, like every other effect.
    /// That the held settings moved arrives on the event stream, so a second window learns it the same way
    /// this one does.
    /// </summary>
    Task SaveSettingsAsync(Settings settings, CancellationToken cancellation = default);

    /// <summary>
    /// Stores one way of publishing under a name, replacing whatever was saved under it.
    ///
    /// <b>A preset is a <see cref="PublishSettings"/> and nothing else</b> (<c>docs/presets.md</c>).
    /// Where the relay is belongs to a deployment and how this machine watches belongs to a viewer, so
    /// neither travels in one: a preset carrying either would be the thing that breaks on the machine it was
    /// copied to.
    ///
    /// <b>It names a state and is safe to repeat.</b> The name is the identity, so a second save of the same
    /// settings under the same name leaves the store holding what it already held.
    /// A name nothing was saved under yet is a new preset, and every other name is the one it replaces -
    /// which is what makes saving over a preset the same call as making one.
    ///
    /// An empty name is refused as a <see cref="BackendUnavailableException"/> carrying the backend's own
    /// sentence, as is a store that could not be written.
    ///
    /// It answers with nothing, and no event follows it: the caller reads <see cref="PresetsAsync"/> again.
    /// </summary>
    Task SavePresetAsync(string name, PublishSettings settings, CancellationToken cancellation = default);

    /// <summary>
    /// Removes the preset saved under a name.
    ///
    /// <b>A name the store does not hold is refused rather than shrugged at</b>, which is the backend's
    /// decision and is the one place presets depart from naming a state (<c>internal/control/effects.go</c>).
    /// So a delete of a preset another window has already removed answers with a sentence rather than
    /// silently; reading the store again is what puts the screen back in step.
    ///
    /// It answers with nothing, and no event follows it: the caller reads <see cref="PresetsAsync"/> again.
    /// </summary>
    Task DeletePresetAsync(string name, CancellationToken cancellation = default);

    /// <summary>Ends the stream, whether it is running or waiting out a retry backoff.</summary>
    Task StopPublishAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Measures this machine's real upload throughput, in Mbit/s, so a guessed uplink figure can be replaced
    /// by one that was observed.
    ///
    /// <b>An effect, and the one that answers with something.</b> The rule above holds for the state-changing
    /// calls, whose result arrives on the stream; this changes no state the backend holds - it uploads a
    /// payload and times it - so there is nothing for an event to carry and the figure comes back here.
    /// What the caller does with it is a settings write like any other, which is what keeps a measured uplink
    /// and a typed one one value.
    ///
    /// It runs the real thing and takes seconds, and the backend refuses it outright while a stream is
    /// publishing, because a measurement would compete with the stream for the line.
    /// That refusal arrives as a <see cref="BackendUnavailableException"/> carrying the backend's own
    /// sentence, which is the one worth showing.
    /// </summary>
    Task<double> MeasureUplinkAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Opens an external viewer for one stream over one transport.
    /// A leg that cannot carry the stream's format is refused with the format named, rather than opening a
    /// viewer that connects and receives nothing - so the refusal is a sentence worth showing.
    /// </summary>
    Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default);

    /// <summary>Closes one open viewer.</summary>
    Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default);

    /// <summary>
    /// Opens the relay's own player page for one stream in the machine's default browser, over one of the
    /// legs the relay serves a page for (<see cref="ScreenShare.Api.V1.Catalog.BrowserWatchTransports"/>).
    /// A leg the stream's format does not cross is refused the way <see cref="StartWatchAsync"/> refuses one.
    ///
    /// <b>Nothing is opened that can be closed again.</b> The tab belongs to the browser, so no viewer state
    /// grows a member and there is no counterpart to this call: a control for it names an action rather than
    /// a state, and shows no tick.
    /// </summary>
    Task OpenInBrowserAsync(string streamName, string transport, CancellationToken cancellation = default);

    /// <summary>
    /// Opens a decode for one stream on one leg, inside the backend.
    /// It is the tile path's counterpart of <see cref="StartWatchAsync"/>, and the difference is where the
    /// frames end up: a watch launches a player window the backend does not draw in, and a receive decodes
    /// into the backend, from where the frame channel hands the frames to this shell.
    ///
    /// <b>What it opens is a decode and not a tile.</b> Nothing about a grid, a layout or a window crosses
    /// here, because how a viewer arranges what it receives is this shell's whole job
    /// (<c>docs/ipc-api.md</c>).
    ///
    /// A leg that cannot carry the stream's format is refused with the format named, as a
    /// <see cref="BackendUnavailableException"/> carrying the backend's own sentence.
    ///
    /// <b><paramref name="toneMap"/> is part of what the decode is built from.</b> It asks for an HDR stream
    /// to be rolled down into the range a standard display shows, which is an element in the pipeline rather
    /// than a value written to a running one - so this call is also how the answer is changed, and a decode
    /// running with the other one is rebuilt.
    /// It is stored nowhere: a preference kept per stream would outlive the stream it was made about, so it
    /// lives exactly as long as the decode.
    ///
    /// A machine with no element that rolls the range down builds the decode without one and reports that it
    /// did, rather than refusing.
    /// </summary>
    Task StartReceiveAsync(
        string streamName, string transport, bool toneMap = false, CancellationToken cancellation = default);

    /// <summary>
    /// Closes one running decode.
    /// A stream nothing is decoding is not an error: a stop is what the reader asked for and it is already
    /// true.
    /// </summary>
    Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default);

    /// <summary>
    /// Sets how loud one decode plays and whether it plays at all.
    ///
    /// <b>The loudness belongs to the decode, not to a window drawing it.</b> The audio branch is one element
    /// inside one pipeline, playing on the machine the shell is on rather than travelling over the frame
    /// channel (<c>docs/viewer-architecture.md</c>).
    /// Two windows on one decode share that element, so a per-window volume would be several controls over
    /// one thing, each showing a value the others had overwritten.
    ///
    /// Mute is sent with the volume rather than as a call of its own, because the two are one state: sent
    /// apart, a caller that muted and then set a volume would have to know whether the second undid the
    /// first.
    ///
    /// Safe to repeat and safe to send early.
    /// A decode already at that loudness is a state that holds, and a volume set before the decoder has
    /// exposed an audio pad is applied when it does.
    /// A decode that is not running at all is refused as a <see cref="BackendUnavailableException"/> - that
    /// is a request about something absent.
    ///
    /// It answers with nothing, like every other effect: what the loudness became arrives on the stream,
    /// inside the receive state.
    /// </summary>
    Task SetReceiveAudioAsync(string streamName, string transport, double volume, bool muted, CancellationToken cancellation = default);

    /// <summary>
    /// Subscribes to the frames of a decode that is already running.
    ///
    /// It opens no decode, which is why it can be refused: the caller runs <see cref="StartReceiveAsync"/>
    /// first and subscribes once that has answered.
    /// The two staying separate is what lets a decode outlive every tile drawing it - a window that closed
    /// does not stop a stream, and one that opened again finds it running.
    ///
    /// Several tiles may subscribe to one decode and each gets a pool of its own, so a tile that is slow to
    /// draw cannot hold a buffer another one is waiting for.
    /// </summary>
    Task<FrameChannel> OpenFramesAsync(string streamName, string transport, CancellationToken cancellation = default);

    /// <summary>
    /// Subscribes to the frames of the running publish's local preview: the stream this machine is sending,
    /// decoded from a copy the publish child writes to a loopback port rather than read back off the relay
    /// (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws").
    ///
    /// <b>It names nothing, and both halves of that are the point.</b> There is at most one publish, so the
    /// preview needs no identity of its own; and its frames crossed no protocol, so it has no leg to be named
    /// by - a synthetic one would put a transport in the table every consumer reads and nothing could act on
    /// it.
    ///
    /// It opens no pipeline.
    /// What brings the preview up is the publish itself, so a call made with nothing publishing is refused as
    /// a <see cref="BackendUnavailableException"/> carrying the backend's own sentence.
    /// A caller reads <c>PublishState</c> to know whether there is one to ask for rather than asking to find
    /// out.
    /// </summary>
    Task<FrameChannel> OpenPreviewFramesAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Reads one of this machine's monitors into a picture the frame channel can hand over, so the setup
    /// wizard offers a screen by what is on it rather than by its number.
    ///
    /// <b>Nothing is encoded and nothing is sent anywhere.</b> The capture element feeds the render chain
    /// inside the backend and the handles come here, so this costs one screen read and no bandwidth at all.
    ///
    /// It is an effect and the frame channel opens none of it, which is the same division
    /// <see cref="StartReceiveAsync"/> draws: a subscription finds a picture or is refused, and a caller asks
    /// for the preview first.
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
    /// Read once when a shell connects, for the reason the running decodes are: a preview outlives the window
    /// that asked for it, so a shell that crashed with screens being read leaves them running, and this is
    /// how the next one finds them.
    /// Afterwards the same shape arrives on the event stream whenever it moves.
    /// </summary>
    Task<IReadOnlyList<PreviewedMonitor>> PreviewedMonitorsAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Closes one monitor's preview.
    /// A monitor nothing is previewing is not an error: a stop is what the reader asked for and it is already
    /// true.
    /// </summary>
    Task StopMonitorPreviewAsync(int monitor, CancellationToken cancellation = default);

    /// <summary>
    /// Subscribes to the frames of a monitor preview that is already running.
    ///
    /// It opens no capture, for the reason <see cref="OpenFramesAsync"/> opens no decode.
    /// A monitor nothing is previewing is refused as a <see cref="BackendUnavailableException"/>, and a
    /// caller reads the preview state to know whether to ask at all.
    /// </summary>
    Task<FrameChannel> OpenMonitorFramesAsync(int monitor, CancellationToken cancellation = default);

    /// <summary>
    /// Opens one run log in the machine's default application.
    /// The path is one the backend handed out on an <see cref="ExitInfo"/> and a shell does not construct
    /// one: the backend writes these files and rotates them, so it is the only side that knows which of them
    /// still exist under the name it gave out.
    /// </summary>
    Task OpenLogAsync(string path, CancellationToken cancellation = default);

    /// <summary>Opens the directory holding the run logs.</summary>
    Task OpenLogsFolderAsync(CancellationToken cancellation = default);

    // --- Stream -------------------------------------------------------------------

    /// <summary>
    /// What changed, for as long as the caller holds the enumeration.
    ///
    /// <b>Every event carries a whole state, never a delta.</b> A caller receiving <c>PublishState</c>
    /// renders it; it does not apply it to something it was holding.
    /// A duplicate event is therefore harmless and a dropped connection is recovered from by reading the
    /// state again, not by replaying history.
    ///
    /// It carries the changes this shell did not make, which is the whole reason it exists: another window's
    /// stop, a pipeline that died on its own, a viewer that closed, an encoder probe that landed.
    /// </summary>
    IAsyncEnumerable<Event> SubscribeAsync(CancellationToken cancellation = default);

    /// <summary>
    /// How loud every decode carrying audio is, at a fixed cadence, for as long as the caller holds the
    /// enumeration.
    ///
    /// <b>A second stream rather than an event kind, and the difference is cadence.</b>
    /// <see cref="SubscribeAsync"/> carries whole states when something changed; a level changes
    /// continuously, and folding it in would push the receive state at metering rate and make every consumer
    /// of that state re-render for a figure none of them reads.
    ///
    /// Each tick is the whole set, like every other state here.
    /// A decode carrying no audio has no entry, and a silent one has an entry reading negative infinity - two
    /// different facts that a tile draws differently: no meter at all, and an empty meter.
    ///
    /// One enumeration covers every decode, so a tile appearing needs no second subscription and a tile
    /// leaving needs no cancellation.
    /// </summary>
    IAsyncEnumerable<AudioLevels> SubscribeAudioLevelsAsync(CancellationToken cancellation = default);

    /// <summary>
    /// Where the publishing machine's pointer is, for as long as the enumeration is held.
    ///
    /// A stream of its own for the reason the levels are one, and one degree more so: the whole point of
    /// sending a position instead of drawing it into the picture is that it costs no frame, so it moves at
    /// its own rate rather than the stream's.
    ///
    /// It carries positions only while a publish whose cursor mode sends them is running, and stays open and
    /// silent otherwise: the mode can change under a subscription, and a shell that had to resubscribe would
    /// be one holding a pointer from the mode before.
    /// </summary>
    IAsyncEnumerable<PointerPosition> SubscribePointerAsync(CancellationToken cancellation = default);
}
