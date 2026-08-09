using System.Net;
using System.Text;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;

namespace ScreenShare.App.Tests;

/// <summary>Answers every request from a delegate, so a poll needs no socket.</summary>
internal sealed class StubHandler(Func<HttpRequestMessage, HttpResponseMessage> answer) : HttpMessageHandler
{
    public Uri? LastUri { get; private set; }

    protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
    {
        LastUri = request.RequestUri;
        return Task.FromResult(answer(request));
    }

    public static HttpResponseMessage Json(string body) => new(HttpStatusCode.OK)
    {
        Content = new StringContent(body, Encoding.UTF8, "application/json"),
    };

    public static HttpResponseMessage Status(HttpStatusCode code) => new(code);
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

    private readonly SeededBackend _seed = new("linux");
    private readonly List<Held> _held = [];

    /// <summary>
    /// Stands in for the encoder probe landing, which is what the real backend raises this for.
    /// Raised by hand, so a test can state that news of a moved answer makes the flow read
    /// again rather than merely redraw what it has.
    /// </summary>
    public event Action? Changed;

    /// <summary>How many resolves have been asked for, which is what an idempotent pass does not raise.</summary>
    public int Resolves => _held.Count;

    /// <summary>Announces that what the backend would answer has moved.</summary>
    public void Announce() => Changed?.Invoke();

    public Task<Catalog> CatalogAsync(CancellationToken cancellation = default)
        => _seed.CatalogAsync(cancellation);

    public Task<Settings> SettingsAsync(CancellationToken cancellation = default)
        => _seed.SettingsAsync(cancellation);

    public Task<Form> ResolveFormAsync(Settings draft, CancellationToken cancellation = default)
    {
        var held = new Held(draft.Clone(), new TaskCompletionSource<Form>(), cancellation);
        _held.Add(held);
        return held.Answer.Task;
    }

    // The rest of the seam is the seed's. This stand-in exists to control the timing of one
    // method, so everything it does not hold up is forwarded rather than answered twice - a
    // second set of answers here would be a second fixture to keep in step with the first.

    public Task<PublishState> PublishStateAsync(CancellationToken cancellation = default)
        => _seed.PublishStateAsync(cancellation);

    public Task<RelayStatus> RelayStatusAsync(CancellationToken cancellation = default)
        => _seed.RelayStatusAsync(cancellation);

    public Task<IReadOnlyList<WatchKey>> WatchingAsync(CancellationToken cancellation = default)
        => _seed.WatchingAsync(cancellation);

    public Task StartPublishAsync(Settings settings, CancellationToken cancellation = default)
        => _seed.StartPublishAsync(settings, cancellation);

    public Task StopPublishAsync(CancellationToken cancellation = default)
        => _seed.StopPublishAsync(cancellation);

    public Task<double> MeasureUplinkAsync(CancellationToken cancellation = default)
        => _seed.MeasureUplinkAsync(cancellation);

    public Task StartWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.StartWatchAsync(streamName, transport, cancellation);

    public Task StopWatchAsync(string streamName, string transport, CancellationToken cancellation = default)
        => _seed.StopWatchAsync(streamName, transport, cancellation);

    public Task OpenLogAsync(string path, CancellationToken cancellation = default)
        => _seed.OpenLogAsync(path, cancellation);

    public Task OpenLogsFolderAsync(CancellationToken cancellation = default)
        => _seed.OpenLogsFolderAsync(cancellation);

    public IAsyncEnumerable<Event> SubscribeAsync(CancellationToken cancellation = default)
        => _seed.SubscribeAsync(cancellation);

    /// <summary>The draft one held resolve was asked about.</summary>
    public Settings Draft(int resolve) => _held[resolve].Draft;

    /// <summary>Whether one held resolve has been asked to stop.</summary>
    public bool IsCancelled(int resolve) => _held[resolve].Cancellation.IsCancellationRequested;

    /// <summary>
    /// Answers one held resolve with the form its own draft resolves to. Everything the
    /// answer sets off - the awaiting call resuming, the dispatch, the adoption and the
    /// render pass - has happened by the time this returns, so a test asserts against a
    /// finished state rather than against a race it hopes to win.
    /// </summary>
    public async Task AnswerAsync(int resolve)
    {
        var held = _held[resolve];
        var form = await _seed.ResolveFormAsync(held.Draft);

        // Completed with the test framework's synchronization context off the thread, which
        // is what buys the paragraph above. The runtime declines to resume an awaiting
        // continuation inline on a thread carrying a context and queues the resumption to
        // the thread pool instead - so the flow would render on one thread while the test
        // read its properties on another, and no amount of awaiting afterwards would put an
        // order on the two. With the context off, the resumption runs here.
        var context = SynchronizationContext.Current;
        SynchronizationContext.SetSynchronizationContext(null);
        try
        {
            held.Answer.SetResult(form);
        }
        finally
        {
            SynchronizationContext.SetSynchronizationContext(context);
        }
    }

    /// <summary>
    /// Refuses one held resolve the way an absent backend does, with the sentence the screen
    /// would show. Everything it sets off has happened by the time this returns, for the reason
    /// <see cref="AnswerAsync"/> states.
    /// </summary>
    public void Fail(int resolve, string reason)
    {
        var context = SynchronizationContext.Current;
        SynchronizationContext.SetSynchronizationContext(null);
        try
        {
            _held[resolve].Answer.SetException(new BackendUnavailableException(reason));
        }
        finally
        {
            SynchronizationContext.SetSynchronizationContext(context);
        }
    }
}

/// <summary>
/// A backend whose running state a test sets: what the relay answered, what is publishing, and
/// whether a start is accepted or refused.
///
/// It exists because the commit depends on states <see cref="SeededBackend"/> deliberately does
/// not seed - there is no relay behind that fixture and no pipeline - and every condition that
/// locks the Go live button is one of them. What it does not do is answer a form: the resolve is
/// the seed's, so the settings half of the gate stays the one real fixture rather than a second
/// copy of the domain.
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

    /// <summary>Why a start is refused, empty while one is accepted.</summary>
    public string Refusal { get; set; } = "";

    /// <summary>The settings every accepted start was asked for, in order.</summary>
    public List<Settings> Started { get; } = [];

    public Task StartPublishAsync(Settings settings, CancellationToken cancellation = default)
    {
        if (Refusal.Length > 0)
        {
            // Faulted rather than thrown, so the caller's await is what raises it - which is the
            // path the gRPC client puts it on.
            return Task.FromException(new BackendUnavailableException(Refusal));
        }

        Started.Add(settings);
        return Task.CompletedTask;
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

    public Task OpenLogAsync(string path, CancellationToken cancellation = default)
        => _seed.OpenLogAsync(path, cancellation);

    public Task OpenLogsFolderAsync(CancellationToken cancellation = default)
        => _seed.OpenLogsFolderAsync(cancellation);

    public IAsyncEnumerable<Event> SubscribeAsync(CancellationToken cancellation = default)
        => _seed.SubscribeAsync(cancellation);
}

/// <summary>A clock the test advances by hand, so a byte delta divides by a known interval.</summary>
internal sealed class StubClock : TimeProvider
{
    private DateTimeOffset _now = DateTimeOffset.UnixEpoch;

    public override DateTimeOffset GetUtcNow() => _now;

    public void Advance(TimeSpan by) => _now += by;
}
