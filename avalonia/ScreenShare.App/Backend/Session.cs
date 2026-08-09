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
/// The one thing it accumulates is <see cref="Samples"/>, and that is not an exception. A
/// sparkline is a history of readings by construction: each sample is a whole state, the
/// window is bounded, and nothing is derived from the series that the backend also states.
///
/// The two writes are <see cref="Start"/> and the subscription loop, both of which land on
/// the UI loop through the dispatcher they were handed, so a screen reading this never sees a
/// half-written pass.
/// </summary>
public sealed class Session
{
    /// <summary>
    /// How many encoder samples the series keeps. Roughly one arrives per second per running
    /// pipeline, so this is about four minutes of stream - enough for a sparkline to show a
    /// dip and short enough that a session left open overnight costs nothing.
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
    public RelayStatus? Relay { get; private set; }

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

        return RunAsync(_cancel.Token);
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

        Write(() =>
        {
            Unavailable = "";
            Words = new Vocabulary(catalog);
            Publish = publish;
            Relay = relay;
            Watching = watching;
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

    private void Take(Event change)
    {
        Assert.NotNull(change, "an event stream delivers events");

        switch (change.PayloadCase)
        {
            case Event.PayloadOneofCase.PublishState:
                // A run that ended takes its samples with it. They belong to the pipeline that
                // produced them, and a sparkline that carried them across a restart would draw
                // two runs as one.
                if (change.PublishState.Live is null)
                {
                    _samples.Clear();
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
                Relay = change.RelayStatus;
                break;

            case Event.PayloadOneofCase.Catalog:
                // The whole reference set again, after the encoder probe filled in. It is
                // taken rather than merged, like every other state here: what changes is
                // which codecs this machine can run, and a half-applied catalog would name
                // one set and grey another.
                Words = new Vocabulary(change.Catalog);
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

    private void Ended(string what, ExitInfo info)
    {
        Assert.That(what.Length > 0, "an ended process is named");

        _exits.Add(new SessionExit(what, info, _clock.GetUtcNow()));
    }

    /// <summary>
    /// Re-reads the open viewers after one ended. It is a read and not an edit for the reason
    /// every other state here is whole: the backend owns which viewers are open, and a list
    /// this class removed an entry from would be a second opinion about it.
    /// </summary>
    private async Task RefreshWatchingAsync()
    {
        try
        {
            var watching = await _backend.WatchingAsync().ConfigureAwait(false);
            Write(() => Watching = watching);
        }
        catch (BackendUnavailableException)
        {
            // The stream will have ended too, and the reconnect above re-reads everything.
        }
        catch (OperationCanceledException)
        {
        }
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
}
