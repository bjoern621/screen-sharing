using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;

namespace ScreenShare.App.Backend;

/// <summary>
/// One child process that ended, with what it was and when this shell heard about it.
///
/// The arrival time is the shell's own and is stated as such: the contract carries no
/// timestamp on an exit, and a session log needs an order and a clock to be readable. What
/// happened and where the log is are the backend's, unchanged.
/// </summary>
/// <param name="What">Which process ended, e.g. "publish pipeline", "viewer lab-04 over srt".</param>
public sealed record SessionExit(string What, ExitInfo Info, DateTimeOffset At);

/// <summary>
/// The running state, as the backend last reported it: what is publishing, what the encoder
/// is measuring, what the relay is carrying, which viewers are open, and what the tile grid
/// is doing.
///
/// <b>It is the one owner of that state, and it holds nothing else.</b> The screens read it
/// through on every render pass and keep no copy of what they found, because two cards each
/// holding their own reading is how a window ends up describing two different streams at
/// once (<c>avalonia/README.md</c>, "How the repository's principles land in C#").
///
/// <b>The relay is part of that, and it is read here rather than polled anywhere.</b> This
/// shell talks to the relay's HTTP API nowhere at all: the backend polls it on one interval
/// and announces each snapshot, because the per-path bitrates are byte deltas between two
/// answers and a second poller would divide them by an interval nobody agreed on
/// (<c>docs/ipc-api.md</c>). Setup's commit gate and the viewer's roster therefore describe
/// the same relay by construction, having read the same field.
///
/// <b>Every field here is a whole state the backend sent, never one this class assembled.</b>
/// An event replaces a field; it is not applied to it. That is the contract's own rule
/// (<c>docs/ipc-api.md</c>, "Events") and it is what makes a duplicate event harmless, a
/// dropped connection recoverable by reading again, and a shell that pressed the button and a
/// shell that did not show the same thing.
///
/// What it accumulates is <see cref="Samples"/> and <see cref="RelaySamples"/>, and neither is
/// an exception. A sparkline is a history of readings by construction: each entry is a whole
/// state the backend sent, the window is bounded, and nothing is derived from a series that the
/// backend also states. The relay series earns its place on the same three counts - a snapshot
/// is whole, the bound is the one below, and the latency plot draws the shape those snapshots
/// went through rather than a figure the backend would have answered differently.
///
/// The relay series carries one thing the encoder series does not, and it is worth stating: a
/// relay snapshot describes every path the relay is carrying, and the plot wants one of them.
/// The path is picked when the series is drawn rather than when it is stored, because which path
/// is ours is the publish state's answer and it can change under a series that has already been
/// recorded (<c>Features/Broadcast/Model/BroadcastSnapshot.cs</c>, <c>PathOf</c>).
///
/// The two writes are <see cref="Start"/> and the subscription loop, both of which land on
/// the UI loop through the dispatcher they were handed, so a screen reading this never sees a
/// half-written pass.
/// </summary>
public sealed class Session
{
    /// <summary>
    /// How many samples a series keeps. Roughly one encoder sample arrives per second per
    /// running pipeline, so this is about four minutes of stream - enough for a sparkline to
    /// show a dip and short enough that a session left open overnight costs nothing.
    ///
    /// The relay series is bounded by the same number rather than by a span of its own, and it
    /// is deliberately a count of samples and not of seconds: how often the backend polls the
    /// relay is not on the contract, so a window stated in seconds here would be a period this
    /// side made up. What the two series therefore share is a length in readings, not a length
    /// in time, which is why neither plot claims the other's window.
    /// </summary>
    private const int SampleWindow = 240;

    /// <summary>
    /// How long a dropped event stream waits before it is opened again. It is a reconnect and
    /// not a poll: the stream ends when the backend goes away, and the delay only keeps a
    /// backend that is not running from being dialled in a tight loop.
    /// </summary>
    private static readonly TimeSpan ReconnectDelay = TimeSpan.FromSeconds(2);

    private readonly IBackend _backend;
    private readonly Action<Action> _dispatch;
    private readonly TimeProvider _clock;
    private readonly List<PublishStats> _samples = [];
    private readonly List<RelayStatus> _relaySamples = [];
    private readonly List<SessionExit> _exits = [];

    private CancellationTokenSource? _cancel;

    /// <param name="dispatch">
    /// Hands work to the UI loop. Injected rather than reached for, so this type stays free of
    /// a toolkit and a test can pass a synchronous dispatcher: events arrive on whichever
    /// thread the transport completed on, and everything below is read by bindings that only
    /// tolerate being written from one.
    /// </param>
    /// <param name="clock">
    /// Stamps an exit with when this shell heard about it. Injected so that a test which
    /// advances it by hand reads a known time rather than whatever the machine said while it
    /// ran.
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
    /// Raised on the UI loop after any field below has moved. It carries nothing: the states
    /// are whole and are read through, so the news that something changed and the thing it
    /// changed to are two different facts and only the first belongs on a signal.
    /// </summary>
    public event Action? Changed;

    /// <summary>
    /// Raised on the UI loop after <see cref="Levels"/> has moved, and never raised by anything
    /// else.
    ///
    /// <b>A second signal rather than a second reason to raise the first, and the reason is
    /// cadence.</b> Every screen re-renders on <see cref="Changed"/>, which is right for a state
    /// that moves when something happened; levels move fifteen times a second, and putting them
    /// on that signal would re-render the whole shell at metering rate to move one bar. Only the
    /// meters subscribe here, so the cost of a tick is the meters and nothing else.
    ///
    /// This is the shell-side half of why <c>SubscribeAudioLevels</c> is a stream of its own
    /// rather than an event kind (<c>docs/ipc-api.md</c>): the separation is worth nothing if
    /// both ends of it land on one notification here.
    /// </summary>
    public event Action? Metered;

    // --- What the backend last said ------------------------------------------------

    /// <summary>Whether a stream is in force, and what it is carrying. Null until the first read lands.</summary>
    public PublishState? Publish { get; private set; }

    /// <summary>
    /// The newest encoder sample, null while nothing is publishing or before the first packet
    /// is muxed. Its <c>missing</c> list names the figures it carries no measurement for, which
    /// a screen shows as absent rather than as a stalled encoder.
    /// </summary>
    public PublishStats? Stats => _samples.Count == 0 ? null : _samples[^1];

    /// <summary>The encoder samples of this run, oldest first, bounded to the sparkline's window.</summary>
    public IReadOnlyList<PublishStats> Samples => _samples;

    /// <summary>The relay snapshot. Null until the first read lands; an unreachable relay is a snapshot that says so.</summary>
    public RelayStatus? Relay => _relaySamples.Count == 0 ? null : _relaySamples[^1];

    /// <summary>
    /// The relay snapshots of this run, oldest first, bounded to the sparkline's window. The
    /// newest of them is <see cref="Relay"/>, read off the same list rather than held beside it,
    /// so the figure a card reads and the last point of the curve under it cannot disagree.
    /// </summary>
    public IReadOnlyList<RelayStatus> RelaySamples => _relaySamples;

    /// <summary>
    /// How this shell names the values the backend sends, built from the catalog.
    ///
    /// It is held here rather than fetched per screen because the catalog is one message
    /// read once and re-announced when the encoder probe lands, and two screens fetching it
    /// separately could hold two versions of it while a probe was in flight. Before the
    /// first read it names everything it can from its own tables, which is everything except
    /// the two facts that are this machine's: what a codec produces, and what a screen shows.
    /// </summary>
    public Vocabulary Words { get; private set; } = Vocabulary.Empty;

    /// <summary>The external viewers currently open.</summary>
    public IReadOnlyList<WatchKey> Watching { get; private set; } = [];

    /// <summary>
    /// Every stream the backend is decoding for a tile, and what the pipeline behind each
    /// turned out to be: the render chain that actually ran, the memory the frames were in at
    /// each end, the decoder, and whether it ran on silicon.
    ///
    /// It is reported rather than asked for, which is why it is worth reading at all. A chain
    /// falls back on a machine that cannot run its elements and a hardware decoder may download
    /// its own frames, so a tile that showed the chain the settings named would be showing a
    /// request instead of a result.
    /// </summary>
    public IReadOnlyList<ReceiveStream> Receiving { get; private set; } = [];

    /// <summary>
    /// How loud every decode carrying audio is, as of the last tick.
    ///
    /// Whole per tick like every other state here, and read through by the meters rather than
    /// accumulated: a decode with no audio track has no entry, and a silent one has an entry
    /// reading negative infinity. The two are different facts and stay different - one draws no
    /// meter and the other an empty one.
    ///
    /// Empty while nothing is being metered, which includes the case where the level stream is
    /// down. A meter frozen at whatever was last measured would be the one reading that is
    /// certainly wrong.
    /// </summary>
    public IReadOnlyList<AudioLevel> Levels { get; private set; } = [];

    /// <summary>
    /// The level of one decode, or null where it carries no audio or is not being metered.
    ///
    /// The join is on the pair the whole receive contract keys a decode by, because the relay
    /// re-serves each stream on all its listeners and a name alone is not an identity.
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
    /// The child processes that ended this session, oldest first: the publish pipeline, a
    /// viewer, a test stream. Each carries the failure as prose and the run log's path, which
    /// is what a screen offers to open rather than reading it.
    /// </summary>
    public IReadOnlyList<SessionExit> Exits => _exits;

    /// <summary>
    /// Why the backend could not be reached, empty while it can. It is that side's own
    /// sentence, shown as it stands: a shell with nothing to talk to says so rather than
    /// drawing figures it made up.
    /// </summary>
    public string Unavailable { get; private set; } = "";

    /// <summary>Whether the first read of every state has landed, so a screen can tell empty from unread.</summary>
    public bool IsLoaded { get; private set; }

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// Reads every state once and then holds the event stream open.
    ///
    /// Idempotent: a second call supersedes the first rather than opening a second stream, so a
    /// caller may run it to retry after the backend was found absent. Nothing is awaited by the
    /// caller - the window is on its way to a first paint, and a screen with no state yet
    /// renders its unloaded branch rather than waiting for a socket.
    /// </summary>
    public Task Start()
    {
        _cancel?.Cancel();
        _cancel?.Dispose();
        _cancel = new CancellationTokenSource();

        // Two loops rather than one, because they are two streams with two cadences and one
        // ending is not a reason to reopen the other. The event stream owns the report of an
        // absent backend; the meter loop stays quiet about it and simply empties the levels.
        return Task.WhenAll(RunAsync(_cancel.Token), MeterAsync(_cancel.Token));
    }

    /// <summary>
    /// Takes a roster the backend just answered with.
    ///
    /// It exists because one change has no event: the stream announces a viewer that <i>ended</i>
    /// and not one that started, so a screen that opened one reads the list again and hands it
    /// here rather than holding a second copy of it. What crosses is still a whole list the
    /// backend produced, which is the only kind of value this class stores.
    /// </summary>
    public void Adopt(IReadOnlyList<WatchKey> watching)
    {
        Assert.NotNull(watching, "adopting a roster needs the roster the backend answered with");

        Write(() => Watching = watching);
    }

    /// <summary>Ends the subscription. Safe to call with none running.</summary>
    public void Stop()
    {
        _cancel?.Cancel();
        _cancel?.Dispose();
        _cancel = null;
    }

    /// <summary>
    /// One pass of the whole lifecycle: read every state, then follow the stream until it ends,
    /// then read them again.
    ///
    /// The re-read after a reconnect is not belt and braces. The states are whole and the stream
    /// carries no history, so everything that happened while the connection was down is learned
    /// by reading rather than by replay - which is exactly what the contract says a shell does
    /// with a dropped connection.
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
    /// Reads the three states. They are read together rather than one per screen, because the
    /// broadcast screen and the viewer describe the same session and a window whose halves were
    /// read seconds apart would show one.
    /// </summary>
    private async Task LoadAsync(CancellationToken cancellation)
    {
        var catalog = await _backend.CatalogAsync(cancellation).ConfigureAwait(false);
        var publish = await _backend.PublishStateAsync(cancellation).ConfigureAwait(false);
        var relay = await _backend.RelayStatusAsync(cancellation).ConfigureAwait(false);
        var watching = await _backend.WatchingAsync(cancellation).ConfigureAwait(false);
        var receiving = await _backend.ReceivingAsync(cancellation).ConfigureAwait(false);

        Write(() =>
        {
            Unavailable = "";
            Words = new Vocabulary(catalog);
            Publish = publish;
            TakeRelay(relay);
            Watching = watching;
            Receiving = receiving;
            IsLoaded = true;
        });
    }

    /// <summary>
    /// Follows the event stream, replacing one whole state per event.
    ///
    /// The switch is over the payload the backend chose and nothing else: no event is combined
    /// with another, none is applied to what was held, and a payload this build does not name
    /// is ignored rather than asserted on - a backend on a higher minor may send one, and the
    /// contract says that is a shell finding a method missing rather than a bug.
    /// </summary>
    private async Task FollowAsync(CancellationToken cancellation)
    {
        await foreach (var change in _backend.SubscribeAsync(cancellation).ConfigureAwait(false))
        {
            Write(() => Take(change));
        }
    }

    /// <summary>
    /// Follows the level stream for as long as the session runs, reopening it after it drops.
    ///
    /// Its own loop rather than a branch of <see cref="RunAsync"/>, because the two streams end
    /// for their own reasons and neither ending should reopen the other. It reads nothing back
    /// on reconnect either: a level is an instant rather than a state that accumulated, so the
    /// next tick is the whole recovery.
    ///
    /// An absent backend is not reported from here. <see cref="RunAsync"/> owns that sentence,
    /// and a second writer of it would be two ways of saying one thing.
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
                // Said once, by the loop that owns saying it.
            }

            // The meters are emptied rather than left where the last tick put them. A bar frozen
            // at the last figure a dead stream carried is the one reading that is certainly
            // wrong, and no level is the honest state of a shell that is being told none.
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
                // A run that ended takes its samples with it. They belong to the pipeline that
                // produced them, and a sparkline that carried them across a restart would draw
                // two runs as one.
                //
                // The relay series goes with them, and for the same reason rather than a
                // weaker one: the relay keeps answering after the stream stops, but the plot
                // beside the egress curve describes this run's viewers, and readers of a path
                // that is no longer being published to are not what its next run started with.
                // The newest snapshot goes too, and it comes straight back on the next poll.
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
                // The whole reference set again, after the encoder probe filled in. It is
                // taken rather than merged, like every other state here: what changes is
                // which codecs this machine can run, and a half-applied catalog would name
                // one set and grey another.
                Words = new Vocabulary(change.Catalog);
                break;

            case Event.PayloadOneofCase.ReceiveState:
                // What the running decodes turned out to be, whole like every other state. It
                // arrives on every change, including the first frame of a stream: what a
                // pipeline negotiated is only knowable once one has left it.
                Receiving = change.ReceiveState.Streams;
                break;

            case Event.PayloadOneofCase.ViewerState:
                // The roster arrives whole, so it is taken rather than re-read. It used to be
                // fetched again on every viewer exit, because no event carried it: the backend
                // now announces it on every change, including the ones this shell did not make.
                Watching = change.ViewerState.Viewers;
                break;

            case Event.PayloadOneofCase.PublishExit:
                Ended("publish pipeline", change.PublishExit);
                break;

            case Event.PayloadOneofCase.TestStreamExit:
                Ended("test stream", change.TestStreamExit);
                break;

            case Event.PayloadOneofCase.ViewerExit:
                // A viewer that ended changes two things - the roster and, where it failed, the
                // log. Only the log is taken here: the roster is its own event, so editing it
                // from this one would be two definitions of what is open.
                if (change.ViewerExit.Exit is { } exit)
                {
                    Ended($"viewer {change.ViewerExit.Viewer.StreamName} over {change.ViewerExit.Viewer.Transport}", exit);
                }

                break;
        }
    }

    /// <summary>
    /// Records one relay snapshot, which is both the newest state and the newest point of the
    /// series. One method for both so that there is one place a snapshot enters this class, and
    /// so that a read on reconnect and an event on the stream cannot take different paths in.
    /// </summary>
    private void TakeRelay(RelayStatus relay)
    {
        Assert.NotNull(relay, "a relay snapshot is the whole state the backend answered with");

        _relaySamples.Add(relay);
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
    /// Runs one write on the UI loop and announces it. Every assignment above goes through
    /// this, so there is one place the change notification is raised and one thread the state
    /// is written from.
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
    /// Runs one level write on the UI loop and announces it to the meters alone.
    ///
    /// Deliberately not <see cref="Write"/>. It raises <see cref="Metered"/> and never
    /// <see cref="Changed"/>, so a tick costs the bars that draw it rather than every screen in
    /// the shell.
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
