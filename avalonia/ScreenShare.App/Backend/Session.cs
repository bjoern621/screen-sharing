using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;

namespace ScreenShare.App.Backend;

/// <summary>
/// One child process that ended, and when this shell heard about it.
/// <c>At</c> is this shell's clock: the contract puts no timestamp on an exit, and a session log needs an order
/// to be readable.
/// <c>Info</c> is the backend's, unchanged.
/// </summary>
/// <param name="What">Which process ended: "publish pipeline", "viewer lab-04 over srt".</param>
public sealed record SessionExit(string What, ExitInfo Info, DateTimeOffset At);

/// <summary>
/// One relay snapshot, and when this shell received it.
/// <c>At</c> is this shell's clock, for the reason <see cref="SessionExit"/> carries one: the contract puts none
/// on a snapshot.
/// A viewer that left is dated by the poll that stopped naming it, the relay reporting who is connected and
/// never that somebody disconnected.
/// </summary>
public sealed record RelayReading(RelayStatus Status, DateTimeOffset At);

/// <summary>
/// Running state, as the backend last reported it: what is publishing, what the encoder is measuring, what the
/// relay is carrying, which viewers are open, what is being decoded.
///
/// <b>One owner of that state, holding nothing else.</b> Screens read it through on every render pass and keep
/// no copy, two cards each holding a reading of their own being a window describing two streams at once
/// (<c>avalonia/README.md</c>, "How the repository's principles land in C#").
///
/// <b>The relay is read here rather than polled anywhere.</b> This shell never talks to the relay's HTTP API:
/// the backend polls on one interval and announces each snapshot, and per-path bitrates are byte deltas between
/// two answers that a second poller would divide by an interval nobody agreed on (<c>docs/ipc-api.md</c>).
/// Setup's commit gate and the viewer's roster read one field, so they cannot describe two relays.
///
/// <b>Every field is a whole state the backend sent, never one assembled here.</b> An event replaces a field
/// rather than being applied to it (<c>docs/ipc-api.md</c>, "Events"), which makes a duplicate event harmless
/// and a dropped connection recoverable by reading again.
///
/// <see cref="Samples"/> and <see cref="RelaySamples"/> accumulate, and neither departs from that: each entry is
/// a whole state, the window is bounded, and nothing is derived from a series the backend also states.
///
/// A relay snapshot describes every path the relay carries and a plot wants one of them.
/// The path is picked at draw time and not at store time, the publish state answering which path is ours and
/// being able to move under a recorded series
/// (<c>Features/Broadcast/Model/BroadcastSnapshot.cs</c>, <c>PathOf</c>).
///
/// <see cref="Start"/> and the subscription loops are the writers, and both land on the UI loop through the
/// injected dispatcher, so a screen never reads a half-written pass.
/// </summary>
public sealed class Session
{
    /// <summary>
    /// Readings a series keeps.
    /// About one encoder sample per second per running pipeline, so roughly four minutes of stream, and bounded
    /// so a window left open overnight costs nothing.
    ///
    /// Readings rather than seconds because it bounds what this class holds rather than what anything draws: the
    /// relay poll period is not on the contract, so a span stated here would be one this side made up.
    /// What a plot covers is its own and shorter, taken against the clock each reading carries
    /// (<c>Features/Broadcast/Plots/Model/PlotSeries.cs</c>).
    /// The history outlives the plot, so a card is never why a reading was dropped.
    /// </summary>
    private const int SampleWindow = 240;

    /// <summary>
    /// Wait before a dropped stream is opened again.
    /// A reconnect and not a poll: a stream ends when the backend goes away, and the delay keeps an absent
    /// backend from being dialled in a tight loop.
    /// </summary>
    private static readonly TimeSpan ReconnectDelay = TimeSpan.FromSeconds(2);

    private readonly IBackend _backend;
    private readonly Action<Action> _dispatch;
    private readonly TimeProvider _clock;
    private readonly List<PublishStats> _samples = [];
    private readonly List<RelayReading> _relaySamples = [];
    private readonly List<SessionExit> _exits = [];

    private CancellationTokenSource? _cancel;

    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// Events arrive on whichever thread the transport completed on, and every field below is read by bindings
    /// that tolerate one writer thread.
    /// Injected rather than reached for, so this type carries no toolkit and a test passes a synchronous
    /// dispatcher.
    /// </param>
    /// <param name="clock">
    /// Stamps an exit and a relay reading with when this shell heard about it.
    /// Injected so a test advancing it by hand reads a known time.
    /// </param>
    public Session(IBackend backend, Action<Action> dispatch, TimeProvider? clock = null)
    {
        Assert.NotNull(backend, "a session reads the backend that owns the running state");
        Assert.NotNull(dispatch, "a session needs a UI loop to marshal an event back to");

        _backend = backend;
        _dispatch = dispatch;
        _clock = clock ?? TimeProvider.System;
    }

    /// <summary>
    /// Raised on the UI loop after any field below has moved.
    /// Carries nothing: the states are whole and read through, so that something changed and what it changed to
    /// are two facts and only the first belongs on a signal.
    /// </summary>
    public event Action? Changed;

    /// <summary>
    /// Raised on the UI loop after <see cref="Levels"/> or <see cref="Pointer"/> has moved, and by nothing else.
    ///
    /// <b>A second signal rather than a second reason to raise the first, and the reason is cadence.</b>
    /// Every screen re-renders on <see cref="Changed"/>, which suits a state that moves when something happened.
    /// A level moves fifteen times a second, so putting it there would re-render the shell at metering rate to
    /// move one bar.
    /// Only the meters and the pointer subscribe here.
    ///
    /// The shell-side half of why <c>SubscribeAudioLevels</c> is a stream of its own rather than an event kind
    /// (<c>docs/ipc-api.md</c>): that separation is worth nothing if both ends land on one notification here.
    /// </summary>
    public event Action? Metered;

    // --- What the backend last said ------------------------------------------------

    /// <summary>Whether a stream is in force, and what it carries. Null until the first read lands.</summary>
    public PublishState? Publish { get; private set; }

    /// <summary>
    /// Newest encoder sample. Null while nothing publishes and before the first packet is muxed.
    /// Its <c>missing</c> list names the figures it carries no measurement for, drawn as absent rather than as a
    /// stalled encoder.
    /// </summary>
    public PublishStats? Stats => _samples.Count == 0 ? null : _samples[^1];

    /// <summary>Encoder samples of this run, oldest first, bounded to <see cref="SampleWindow"/>.</summary>
    public IReadOnlyList<PublishStats> Samples => _samples;

    /// <summary>Newest relay snapshot. Null until the first read lands. An unreachable relay is a snapshot saying so.</summary>
    public RelayStatus? Relay => _relaySamples.Count == 0 ? null : _relaySamples[^1].Status;

    /// <summary>
    /// Relay snapshots of this run, oldest first, bounded the same way.
    /// <see cref="Relay"/> is the last of them rather than a field beside them, so a figure on a card and the end
    /// of the curve under it cannot disagree.
    ///
    /// Two cards ask the series different questions: the latency plot draws the shape the figures went through,
    /// and the session log derives who arrived and who left from where consecutive rosters differ
    /// (<c>Features/Broadcast/Model/Audience.cs</c>).
    /// Neither answer is accumulated here. Both are functions of whole states in order.
    /// </summary>
    public IReadOnlyList<RelayReading> RelaySamples => _relaySamples;

    /// <summary>
    /// How this shell names the values the backend sends, built from the catalog.
    /// Held here rather than fetched per screen: the catalog is one message, read once and re-announced when the
    /// encoder probe lands, and two screens fetching it could hold two versions across a probe.
    /// Before the first read it names everything off its own tables, which is everything but the two facts that
    /// are this machine's: what a codec produces, and what a screen shows.
    /// </summary>
    public Vocabulary Words { get; private set; } = Vocabulary.Empty;

    /// <summary>
    /// Legs the relay serves a player page for, which are the ones a stream opens over in a browser.
    /// Off the catalog rather than a form field, no setting standing behind it: a menu offers all of them at
    /// once, and a stored preference would be a value nothing reads.
    /// Empty until the first catalog read lands, which is a menu with nothing under it rather than one guessing
    /// at protocols.
    /// </summary>
    public IReadOnlyList<string> BrowserLegs { get; private set; } = [];

    /// <summary>
    /// Legs an external player can be opened on, which are the transports a player reaches by URL.
    /// Off the catalog for the reason <see cref="BrowserLegs"/> is: a player is opened per press on the leg the
    /// reader picked, and a stored preference would be a value nothing reads.
    /// Every entry is one a player on this machine opens, the roster being that receiver's own, so no row carries
    /// a verdict: whether a leg carries a given stream is answered against the stream as the viewer opens.
    /// Empty until the first catalog read lands.
    /// </summary>
    public IReadOnlyList<string> PlayerLegs { get; private set; } = [];

    /// <summary>
    /// This machine's display outputs, in the enumeration's order.
    /// Empty until the catalog lands, and empty where the outputs could not be enumerated at all, which a screen
    /// says rather than inventing a monitor at index zero.
    /// </summary>
    public IReadOnlyList<Api.V1.Monitor> Monitors { get; private set; } = [];

    /// <summary>
    /// Why this machine cannot show what a monitor holds, null where it can.
    /// Read rather than found out by asking and being refused: the wizard offers the plain list instead of
    /// opening previews the session would refuse one by one.
    /// </summary>
    public Text? NoMonitorPreview { get; private set; }

    /// <summary>External viewers open.</summary>
    public IReadOnlyList<WatchKey> Watching { get; private set; } = [];

    /// <summary>
    /// Every stream the backend is decoding for a tile, and what each pipeline turned out to be: the render chain
    /// that ran, the memory the frames were in at each end, the decoder, and whether it ran on silicon.
    /// Reported rather than asked for, which is why it is worth reading at all: a chain falls back where a machine
    /// cannot run its elements and a hardware decoder may download its own frames, so a tile drawing the chain the
    /// settings named would draw a request instead of a result.
    /// </summary>
    public IReadOnlyList<ReceiveStream> Receiving { get; private set; } = [];

    /// <summary>
    /// What every running decode is doing as of the last sample: what arrives, what came out of the decoder, what
    /// the sink did with it, how the pipeline is timed, and the counters the transport's own elements keep.
    ///
    /// <b>A sample and not a state, hence separate from <see cref="Receiving"/>.</b> What a decode is settles at
    /// negotiation and is announced when it moves.
    /// What it is doing is read off the pipeline on a clock the backend keeps, so two windows on one decode read
    /// one rate rather than each dividing by an interval of its own.
    ///
    /// Empty while nothing decodes, and until the first tick after a decode opens.
    /// A panel with nothing to print says so rather than printing the run before it.
    /// </summary>
    public IReadOnlyList<ReceiveStreamStats> ReceiveStats { get; private set; } = [];

    /// <summary>
    /// Last sample of one decode, null where none has arrived for it.
    /// Joined on the pair the receive contract keys a decode by: the relay re-serves each stream on every
    /// listener, so a name alone is not an identity.
    /// </summary>
    public ReceiveStreamStats? StatsOf(string streamName, string transport)
    {
        foreach (var stats in ReceiveStats)
        {
            if (stats.Stream.StreamName == streamName && stats.Stream.Transport == transport)
            {
                return stats;
            }
        }

        return null;
    }

    /// <summary>
    /// Every monitor the backend is reading into a picture, and whether a frame has come off each yet.
    /// A shorter row than <see cref="Receiving"/>: nothing encoded these frames, so there is no decoder to name,
    /// and nothing carried them, so there is no leg.
    /// A preview outlives the window that asked for it, as a decode does, so this is what the wizard converges
    /// against: a restarted shell finds the previews the last one opened and closes the ones nothing draws.
    /// </summary>
    public IReadOnlyList<PreviewedMonitor> PreviewedMonitors { get; private set; } = [];

    /// <summary>
    /// How loud every decode carrying audio is as of the last tick.
    /// Whole per tick and read through by the meters rather than accumulated.
    /// A decode with no audio track has no entry, a silent one an entry reading negative infinity: two facts,
    /// drawn as no meter and as an empty one.
    /// Empty while nothing is metered, the level stream being down included: a bar frozen at the last figure a
    /// dead stream carried is the one reading that is certainly wrong.
    /// </summary>
    public IReadOnlyList<AudioLevel> Levels { get; private set; } = [];

    /// <summary>
    /// Where the publishing machine's pointer is, null where nothing sends one.
    /// Null rather than an off-screen position: a publish whose cursor mode draws the pointer into the frames
    /// sends none, and an overlay drawn anyway would be a second pointer over the first.
    /// Raises <see cref="Metered"/> rather than <see cref="Changed"/>, for the reason a level does: it moves
    /// faster than any other state here, and one tile reads it.
    /// </summary>
    public PointerPosition? Pointer { get; private set; }

    /// <summary>
    /// Level of one decode, null where it carries no audio or is not metered.
    /// Joined on the pair the receive contract keys a decode by: the relay re-serves each stream on every
    /// listener, so a name alone is not an identity.
    /// </summary>
    public AudioLevel? LevelOf(string streamName, string transport)
    {
        foreach (var level in Levels)
        {
            if (level.Stream.StreamName == streamName && level.Stream.Transport == transport)
            {
                return level;
            }
        }

        return null;
    }

    /// <summary>
    /// Child processes that ended this session, oldest first: the publish pipeline, a viewer, a test stream.
    /// Each carries the failure as prose and the run log's path, which a screen offers to open rather than
    /// reading itself.
    /// </summary>
    public IReadOnlyList<SessionExit> Exits => _exits;

    /// <summary>
    /// Why the backend could not be reached, empty while it can.
    /// That side's own sentence, shown as it stands: a shell with nothing to talk to says so rather than drawing
    /// figures it made up.
    /// </summary>
    public string Unavailable { get; private set; } = "";

    /// <summary>Whether the first read of every state has landed, so a screen tells empty from unread.</summary>
    public bool IsLoaded { get; private set; }

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// Reads every state once, then holds the streams open.
    /// Idempotent: a second call supersedes the first rather than opening a second stream, so it doubles as the
    /// retry after the backend was found absent.
    /// The caller awaits nothing, a screen with no state yet rendering its unloaded branch rather than holding up
    /// the first paint on a socket.
    /// </summary>
    public Task Start()
    {
        _cancel?.Cancel();
        _cancel?.Dispose();
        _cancel = new CancellationTokenSource();

        // A loop per stream, because each has its own cadence and one ending is no reason to reopen another.
        // Reporting an absent backend belongs to the event loop alone.
        return Task.WhenAll(RunAsync(_cancel.Token), MeterAsync(_cancel.Token), PointerAsync(_cancel.Token));
    }

    /// <summary>
    /// Takes a roster the backend just answered with.
    /// Here because one change has no event: the stream announces a viewer that <i>ended</i> and not one that
    /// started, so a screen that opened one reads the list again and hands it over rather than keeping a copy of
    /// its own.
    /// What crosses is a whole list the backend produced, the only kind of value this class stores.
    /// </summary>
    public void Adopt(IReadOnlyList<WatchKey> watching)
    {
        Assert.NotNull(watching, "adopting a roster needs the roster the backend answered with");

        Write(() => Watching = watching);
    }

    /// <summary>Ends the subscriptions. Idempotent: with none running, it does nothing.</summary>
    public void Stop()
    {
        _cancel?.Cancel();
        _cancel?.Dispose();
        _cancel = null;
    }

    /// <summary>
    /// One pass of the lifecycle: read every state, follow the stream until it ends, read them again.
    /// The states are whole and the stream carries no history, so what happened while the connection was down is
    /// learned by reading and never by replay (<c>docs/ipc-api.md</c>, "Events").
    /// </summary>
    private async Task RunAsync(CancellationToken cancellation)
    {
        while (!cancellation.IsCancellationRequested)
        {
            try
            {
                await LoadAsync(cancellation).ConfigureAwait(false);
                await FollowAsync(cancellation).ConfigureAwait(false);
            }
            catch (OperationCanceledException)
            {
                return;
            }
            catch (BackendUnavailableException e)
            {
                Write(() => Unavailable = e.Message);
            }

            try
            {
                await Task.Delay(ReconnectDelay, cancellation).ConfigureAwait(false);
            }
            catch (OperationCanceledException)
            {
                return;
            }
        }
    }

    /// <summary>
    /// Reads every state once.
    /// Together rather than one per screen: the broadcast screen and the viewer describe one session, and a
    /// window whose halves were read seconds apart would show that.
    /// </summary>
    private async Task LoadAsync(CancellationToken cancellation)
    {
        var catalog = await _backend.CatalogAsync(cancellation).ConfigureAwait(false);
        var publish = await _backend.PublishStateAsync(cancellation).ConfigureAwait(false);
        var relay = await _backend.RelayStatusAsync(cancellation).ConfigureAwait(false);
        var watching = await _backend.WatchingAsync(cancellation).ConfigureAwait(false);
        var receiving = await _backend.ReceivingAsync(cancellation).ConfigureAwait(false);
        var previewed = await _backend.PreviewedMonitorsAsync(cancellation).ConfigureAwait(false);

        Write(() =>
        {
            Unavailable = "";
            Words = new Vocabulary(catalog);
            BrowserLegs = catalog.BrowserWatchTransports;
            PlayerLegs = catalog.WatchTransports;
            Monitors = catalog.Monitors;
            NoMonitorPreview = catalog.NoMonitorPreview;
            Publish = publish;
            TakeRelay(relay);
            Watching = watching;
            Receiving = receiving;
            PreviewedMonitors = previewed;
            IsLoaded = true;
        });
    }

    /// <summary>
    /// Follows the event stream, replacing one whole state per event.
    /// No event is combined with another or applied to what was held.
    /// A payload this build does not name is ignored rather than asserted on: a backend on a higher minor may
    /// send one, which the contract calls a shell finding a method missing rather than a bug.
    /// </summary>
    private async Task FollowAsync(CancellationToken cancellation)
    {
        await foreach (var change in _backend.SubscribeAsync(cancellation).ConfigureAwait(false))
        {
            Write(() => Take(change));
        }
    }

    /// <summary>
    /// Follows where the publishing machine's pointer is, for as long as the session runs.
    /// Its own loop beside the meter's: a stream of its own on the wire, a cadence of its own, and a picture that
    /// goes on being drawn while it reconnects.
    /// The position is dropped when the stream does, a pointer frozen where a dead stream left it being the one
    /// reading that is certainly wrong.
    /// </summary>
    private async Task PointerAsync(CancellationToken cancellation)
    {
        while (!cancellation.IsCancellationRequested)
        {
            try
            {
                await foreach (var at in _backend.SubscribePointerAsync(cancellation).ConfigureAwait(false))
                {
                    Meter(() => Pointer = at);
                }
            }
            catch (OperationCanceledException)
            {
                return;
            }
            catch (BackendUnavailableException)
            {
                // RunAsync owns saying the backend is absent, so this loop says nothing.
            }

            Meter(() => Pointer = null);

            try
            {
                await Task.Delay(ReconnectDelay, cancellation).ConfigureAwait(false);
            }
            catch (OperationCanceledException)
            {
                return;
            }
        }
    }

    /// <summary>
    /// Follows the level stream for as long as the session runs, reopening it after a drop.
    /// Its own loop rather than a branch of <see cref="RunAsync"/>, the two streams ending for their own reasons
    /// and neither ending being a reason to reopen the other.
    /// Nothing is read back on reconnect: a level is an instant rather than an accumulated state, so the next
    /// tick is the whole recovery.
    /// An absent backend is not reported here. <see cref="RunAsync"/> owns that sentence.
    /// </summary>
    private async Task MeterAsync(CancellationToken cancellation)
    {
        while (!cancellation.IsCancellationRequested)
        {
            try
            {
                await foreach (var tick in _backend.SubscribeAudioLevelsAsync(cancellation).ConfigureAwait(false))
                {
                    Meter(() => Levels = tick.Levels);
                }
            }
            catch (OperationCanceledException)
            {
                return;
            }
            catch (BackendUnavailableException)
            {
                // RunAsync owns saying the backend is absent, so this loop says nothing.
            }

            // Emptied rather than frozen at the last tick: no level is the state of a shell being told none.
            Meter(() => Levels = []);

            try
            {
                await Task.Delay(ReconnectDelay, cancellation).ConfigureAwait(false);
            }
            catch (OperationCanceledException)
            {
                return;
            }
        }
    }

    private void Take(Event change)
    {
        Assert.NotNull(change, "an event stream delivers events");

        switch (change.PayloadCase)
        {
            case Event.PayloadOneofCase.PublishState:
                // A run that ended takes its samples with it: they belong to the pipeline that produced them, and
                // a sparkline carrying them across a restart draws two runs as one.
                //
                // The relay series goes for the same reason, not a weaker one.
                // The relay keeps answering, but the plot beside the egress curve describes this run's viewers,
                // and readers of a path nothing publishes to are not what the next run starts with.
                // The newest snapshot goes too, and the next poll brings one back.
                if (change.PublishState.Live is null)
                {
                    _samples.Clear();
                    _relaySamples.Clear();
                }

                Publish = change.PublishState;
                break;

            case Event.PayloadOneofCase.PublishStats:
                _samples.Add(change.PublishStats);
                if (_samples.Count > SampleWindow)
                {
                    _samples.RemoveRange(0, _samples.Count - SampleWindow);
                }

                break;

            case Event.PayloadOneofCase.RelayStatus:
                TakeRelay(change.RelayStatus);
                break;

            case Event.PayloadOneofCase.Catalog:
                // The whole reference set again, after the encoder probe filled in.
                // Taken rather than merged: what moved is which codecs this machine can run, and a half-applied
                // catalog would name one set and grey another.
                Words = new Vocabulary(change.Catalog);
                BrowserLegs = change.Catalog.BrowserWatchTransports;
                PlayerLegs = change.Catalog.WatchTransports;
                Monitors = change.Catalog.Monitors;
                NoMonitorPreview = change.Catalog.NoMonitorPreview;
                break;

            case Event.PayloadOneofCase.ReceiveState:
                // Arrives on every change, the first frame of a stream included: what a pipeline negotiated is
                // knowable only once a frame has left it.
                Receiving = change.ReceiveState.Streams;
                break;

            case Event.PayloadOneofCase.ReceiveStats:
                // The counters are the pipeline's own running totals, so there is nothing to add up, and a decode
                // that ended drops out of the next tick.
                ReceiveStats = change.ReceiveStats.Streams;
                break;

            case Event.PayloadOneofCase.MonitorPreviewState:
                // Arrives when a preview opens or closes, and again on the first frame off each, which is what
                // turns one from opening into live.
                PreviewedMonitors = change.MonitorPreviewState.Monitors;
                break;

            case Event.PayloadOneofCase.ViewerState:
                // The roster, announced on every change including the ones this shell did not make, so it is
                // taken rather than re-read.
                Watching = change.ViewerState.Viewers;
                break;

            case Event.PayloadOneofCase.PublishExit:
                Ended("publishing", change.PublishExit);
                break;

            case Event.PayloadOneofCase.TestStreamExit:
                Ended("test stream", change.TestStreamExit);
                break;

            case Event.PayloadOneofCase.ViewerExit:
                // A viewer that ended moves the roster and, where it failed, the log.
                // Only the log is taken here: the roster has an event of its own, and editing it from this one
                // would be two definitions of what is open.
                if (change.ViewerExit.Exit is { } exit)
                {
                    Ended($"viewer {change.ViewerExit.Viewer.StreamName} over {change.ViewerExit.Viewer.Transport}", exit);
                }

                break;
        }
    }

    /// <summary>
    /// Records one relay snapshot, the newest state and the newest point of the series at once.
    /// One entry path for both, so a read on reconnect and an event on the stream cannot come in differently.
    /// </summary>
    private void TakeRelay(RelayStatus relay)
    {
        Assert.NotNull(relay, "a relay snapshot is the whole state the backend answered with");

        _relaySamples.Add(new RelayReading(relay, _clock.GetUtcNow()));
        if (_relaySamples.Count > SampleWindow)
        {
            _relaySamples.RemoveRange(0, _relaySamples.Count - SampleWindow);
        }

        Assert.That(ReferenceEquals(Relay, relay), "the newest snapshot is the one just taken");
    }

    private void Ended(string what, ExitInfo info)
    {
        Assert.That(what.Length > 0, "an ended process is named");

        _exits.Add(new SessionExit(what, info, _clock.GetUtcNow()));
    }

    /// <summary>
    /// Runs one write on the UI loop and announces it.
    /// Every assignment above goes through it, so one place raises the notification and one thread writes the
    /// state.
    /// </summary>
    private void Write(Action write)
    {
        _dispatch(() =>
        {
            write();
            Changed?.Invoke();
        });
    }

    /// <summary>
    /// Runs one metering write on the UI loop and announces it to the meters alone.
    /// Not <see cref="Write"/>: it raises <see cref="Metered"/> and never <see cref="Changed"/>, so a tick costs
    /// the bars drawing it rather than every screen in the shell.
    /// </summary>
    private void Meter(Action write)
    {
        _dispatch(() =>
        {
            write();
            Metered?.Invoke();
        });
    }
}
