using ScreenShare.Api.V1;
using ScreenShare.App.Backend;

namespace ScreenShare.App.Tests;

/// <summary>
/// How a held call is answered so that everything the answer sets off - the awaiting call
/// resuming, the dispatch, the adoption and the render pass - has happened by the time the
/// answer returns. A test then asserts against a finished state rather than against a race it
/// hopes to win.
/// </summary>
internal static class Answers
{
    /// <summary>
    /// Completes a held call with the test framework's synchronization context off the thread.
    ///
    /// The runtime declines to resume an awaiting continuation inline on a thread carrying a
    /// context and queues the resumption to the thread pool instead - so the view model would
    /// render on one thread while the test read its properties on another, and no amount of
    /// awaiting afterwards would put an order on the two. With the context off, the resumption
    /// runs here.
    /// </summary>
    public static void Now(Action complete)
    {
        var context = SynchronizationContext.Current;
        SynchronizationContext.SetSynchronizationContext(null);
        try
        {
            complete();
        }
        finally
        {
            SynchronizationContext.SetSynchronizationContext(context);
        }
    }
}

/// <summary>
/// A backend whose resolves are answered by hand, which is the only way to write down the
/// timing the real one has: a socket lets two drafts be in flight at once and puts no order
/// on their answers, and the stand-in that answers from memory can never produce that.
///
/// It holds every resolve it is asked for and hands back the seeded form on request, in
/// whichever order the test chooses. The token each call was given is kept as well, so a test
/// can state that superseding a draft asked the older call to stop rather than merely
/// ignoring what it returned.
/// </summary>
internal sealed class DeferredBackend : IBackend
{
    private sealed record Held(Settings Draft, TaskCompletionSource<Form> Answer, CancellationToken Cancellation);

    /// <summary>The sentence an absent backend's reads fail with, as the client would write it.</summary>
    private const string Absent = "The backend is not running: nothing is listening on the control socket.";

    private readonly SeededBackend _seed = new("linux");
    private readonly List<Held> _held = [];

    /// <summary>
    /// Stands in for the encoder probe landing, which is what the real backend raises this for.
    /// Raised by hand, so a test can state that news of a moved answer makes the flow read
    /// again rather than merely redraw what it has.
    /// </summary>
    public event Action? Changed;

    /// <summary>
    /// Whether the backend is absent, in which case every read fails rather than being held:
    /// there is no call to answer later, which is the state a window that opened before its
    /// backend did finds itself in. Set back to false for a backend that has since come up.
    /// </summary>
    public bool IsAbsent { get; set; }

    /// <summary>How many resolves have been asked for, which is what an idempotent pass does not raise.</summary>
    public int Resolves => _held.Count;

    /// <summary>Announces that what the backend would answer has moved.</summary>
    public void Announce() => Changed?.Invoke();

    public Task<Catalog> CatalogAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.CatalogAsync(cancellation);

    public Task<Settings> SettingsAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.SettingsAsync(cancellation);

    public Task<Form> ResolveFormAsync(Settings draft, CancellationToken cancellation = default)
    {
        if (IsAbsent)
        {
            throw new BackendUnavailableException(Absent);
        }

        var held = new Held(draft.Clone(), new TaskCompletionSource<Form>(), cancellation);
        _held.Add(held);
        return held.Answer.Task;
    }

    // The rest of the seam is the seed's. This stand-in exists to control the timing of one
    // method, so everything it does not hold up is forwarded rather than answered twice - a
    // second set of answers here would be a second fixture to keep in step with the first.

    public Task<PublishState> PublishStateAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.PublishStateAsync(cancellation);

    public Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.RelayStatusAsync(cancellation);

    public Task<IReadOnlyList<WatchKey>> WatchingAsync(CancellationToken cancellation = default)
        => IsAbsent ? throw new BackendUnavailableException(Absent) : _seed.WatchingAsync(cancellation);

    public Task StartPublishAsync(Settings settings, CancellationToken cancellation = default)
        => _seed.StartPublishAsync(settings, cancellation);

    public Task ApplyToStreamAsync(Settings settings, CancellationToken cancellation = default)
        => _seed.ApplyToStreamAsync(settings, cancellation);

    public Task StopPublishAsync(CancellationToken cancellation = default)
        => _seed.StopPublishAsync(cancellation);

    public Task<double> MeasureUplinkAsync(CancellationToken cancellation = default)
        => _seed.MeasureUplinkAsync(cancellation);

    public Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.StartWatchAsync(streamName, transport, cancellation);

    public Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.StopWatchAsync(streamName, transport, cancellation);

    public Task<IReadOnlyList<ReceiveStream>> ReceivingAsync(CancellationToken cancellation = default)
        => _seed.ReceivingAsync(cancellation);

    public Task StartReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.StartReceiveAsync(streamName, transport, cancellation);

    public Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.StopReceiveAsync(streamName, transport, cancellation);

    public Task SetReceiveAudioAsync(
        string streamName, string transport, double volume, bool muted, CancellationToken cancellation = default)
        => _seed.SetReceiveAudioAsync(streamName, transport, volume, muted, cancellation);

    public Task<FrameChannel> OpenFramesAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.OpenFramesAsync(streamName, transport, cancellation);

    public Task<FrameChannel> OpenPreviewFramesAsync(CancellationToken cancellation = default)
        => _seed.OpenPreviewFramesAsync(cancellation);

    public Task OpenLogAsync(string path, CancellationToken cancellation = default)
        => _seed.OpenLogAsync(path, cancellation);

    public Task OpenLogsFolderAsync(CancellationToken cancellation = default)
        => _seed.OpenLogsFolderAsync(cancellation);

    public IAsyncEnumerable<Event> SubscribeAsync(CancellationToken cancellation = default)
        => _seed.SubscribeAsync(cancellation);

    public IAsyncEnumerable<AudioLevels> SubscribeAudioLevelsAsync(CancellationToken cancellation = default)
        => _seed.SubscribeAudioLevelsAsync(cancellation);

    /// <summary>The draft one held resolve was asked about.</summary>
    public Settings Draft(int resolve) => _held[resolve].Draft;

    /// <summary>Whether one held resolve has been asked to stop.</summary>
    public bool IsCancelled(int resolve) => _held[resolve].Cancellation.IsCancellationRequested;

    /// <summary>
    /// Answers one held resolve with the form its own draft resolves to. Everything the
    /// answer sets off has happened by the time this returns, for the reason
    /// <see cref="Answers.Now"/> states.
    /// </summary>
    public async Task AnswerAsync(int resolve)
    {
        var held = _held[resolve];
        var form = await _seed.ResolveFormAsync(held.Draft);

        Answers.Now(() => held.Answer.SetResult(form));
    }

    /// <summary>
    /// Refuses one held resolve the way an absent backend does, with the sentence the screen
    /// would show. Everything it sets off has happened by the time this returns, for the reason
    /// <see cref="Answers.Now"/> states.
    /// </summary>
    public void Fail(int resolve, string reason)
        => Answers.Now(() => _held[resolve].Answer.SetException(new BackendUnavailableException(reason)));
}

/// <summary>
/// A backend whose running state a test sets: what the relay answered, what is publishing, and
/// whether a start is accepted or refused.
///
/// It exists because the commit depends on states <see cref="SeededBackend"/> deliberately does
/// not seed - there is no relay behind that fixture and no pipeline - and every condition that
/// locks the commit is one of them, as is the one that decides which effect pressing it is. What
/// it does not do is answer a form: the resolve is the seed's, so the settings half of the gate
/// stays the one real fixture rather than a second copy of the domain.
/// </summary>
internal sealed class PublishingBackend : IBackend
{
    private readonly SeededBackend _seed = new("linux");

    public event Action? Changed
    {
        add { }
        remove { }
    }

    /// <summary>What the relay snapshot says. Reachable by default, so a test states the failure it wants.</summary>
    public RelayStatus Relay { get; set; } = new() { Reachable = true };

    /// <summary>What is publishing. Nothing by default, which the absent <c>Live</c> is what says.</summary>
    public PublishState Publish { get; set; } = new();

    /// <summary>Why a commit is refused, empty while one is accepted.</summary>
    public string Refusal { get; set; } = "";

    /// <summary>The settings every accepted start was asked for, in order.</summary>
    public List<Settings> Started { get; } = [];

    /// <summary>
    /// The settings every accepted apply was asked for, in order. Kept apart from
    /// <see cref="Started"/> because which of the two lists a commit lands in is the whole
    /// question: the backend refuses each of them in the state the other one is for.
    /// </summary>
    public List<Settings> Applied { get; } = [];

    /// <summary>
    /// A commit that has been asked for and not answered, held open by a test.
    ///
    /// It is what makes the round trip an interval a test can read the screen in the middle of.
    /// Every other answer here is immediate, which is the honest default - it keeps the tests
    /// about what the screen says rather than about timing - but the whole point of a control
    /// that reports it is waiting is what it looks like while the backend has not replied.
    /// </summary>
    private TaskCompletionSource? _held;

    /// <summary>Holds every commit open from here on, so one can be read while it is in flight.</summary>
    public void HoldStarts() => _held = new TaskCompletionSource();

    /// <summary>
    /// Answers the held commit. Everything it sets off has happened by the time this returns,
    /// for the reason <see cref="Answers.Now"/> states.
    /// </summary>
    public void AnswerStarts()
    {
        var held = _held ?? throw new InvalidOperationException("no start is being held");

        _held = null;
        Answers.Now(held.SetResult);
    }

    public Task StartPublishAsync(Settings settings, CancellationToken cancellation = default)
        => Commit(Started, settings);

    public Task ApplyToStreamAsync(Settings settings, CancellationToken cancellation = default)
        => Commit(Applied, settings);

    /// <summary>
    /// One commit, recorded where a test will look for it.
    ///
    /// The two effects differ in nothing this fixture does - both carry the whole draft, both
    /// answer with nothing, and both are refused or held on the same terms - so the one thing a
    /// test reads off it is which list the settings landed in.
    /// </summary>
    private Task Commit(List<Settings> into, Settings settings)
    {
        if (Refusal.Length > 0)
        {
            // Faulted rather than thrown, so the caller's await is what raises it - which is the
            // path the gRPC client puts it on.
            return Task.FromException(new BackendUnavailableException(Refusal));
        }

        into.Add(settings);
        return _held?.Task ?? Task.CompletedTask;
    }

    public Task<PublishState> PublishStateAsync(CancellationToken cancellation = default)
        => Task.FromResult(Publish);

    public Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default)
        => Task.FromResult(Relay);

    // The rest is the seed's, for the reason DeferredBackend forwards: a second set of answers
    // here would be a second fixture to keep in step with the first.

    public Task<Catalog> CatalogAsync(CancellationToken cancellation = default)
        => _seed.CatalogAsync(cancellation);

    public Task<Settings> SettingsAsync(CancellationToken cancellation = default)
        => _seed.SettingsAsync(cancellation);

    public Task<Form> ResolveFormAsync(Settings draft, CancellationToken cancellation = default)
        => _seed.ResolveFormAsync(draft, cancellation);

    public Task<IReadOnlyList<WatchKey>> WatchingAsync(CancellationToken cancellation = default)
        => _seed.WatchingAsync(cancellation);

    public Task StopPublishAsync(CancellationToken cancellation = default)
        => _seed.StopPublishAsync(cancellation);

    public Task<double> MeasureUplinkAsync(CancellationToken cancellation = default)
        => _seed.MeasureUplinkAsync(cancellation);

    public Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.StartWatchAsync(streamName, transport, cancellation);

    public Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.StopWatchAsync(streamName, transport, cancellation);

    public Task<IReadOnlyList<ReceiveStream>> ReceivingAsync(CancellationToken cancellation = default)
        => _seed.ReceivingAsync(cancellation);

    public Task StartReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.StartReceiveAsync(streamName, transport, cancellation);

    public Task StopReceiveAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.StopReceiveAsync(streamName, transport, cancellation);

    public Task SetReceiveAudioAsync(
        string streamName, string transport, double volume, bool muted, CancellationToken cancellation = default)
        => _seed.SetReceiveAudioAsync(streamName, transport, volume, muted, cancellation);

    public Task<FrameChannel> OpenFramesAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.OpenFramesAsync(streamName, transport, cancellation);

    public Task<FrameChannel> OpenPreviewFramesAsync(CancellationToken cancellation = default)
        => _seed.OpenPreviewFramesAsync(cancellation);

    public Task OpenLogAsync(string path, CancellationToken cancellation = default)
        => _seed.OpenLogAsync(path, cancellation);

    public Task OpenLogsFolderAsync(CancellationToken cancellation = default)
        => _seed.OpenLogsFolderAsync(cancellation);

    public IAsyncEnumerable<Event> SubscribeAsync(CancellationToken cancellation = default)
        => _seed.SubscribeAsync(cancellation);

    public IAsyncEnumerable<AudioLevels> SubscribeAudioLevelsAsync(CancellationToken cancellation = default)
        => _seed.SubscribeAudioLevelsAsync(cancellation);
}
