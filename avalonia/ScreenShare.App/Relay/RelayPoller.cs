using ScreenShare.App.Contracts;

namespace ScreenShare.App.Relay;

/// <summary>Which relay to poll and how often. The desired state a poller converges onto.</summary>
public sealed record RelayTarget(string Host, int ApiPort, TimeSpan Interval);

/// <summary>
/// Owns the current relay status and the loop that refreshes it. This is the single
/// owner of that state: a view reads <see cref="Latest"/> through on demand and never
/// keeps a copy of what it found there
/// (docs/development-principles.md, "State is written explicitly and read continuously").
/// </summary>
public sealed class RelayPoller : IDisposable
{
    private readonly RelayClient _client;
    private readonly Action<Action> _dispatch;

    private RelayTarget? _running;
    private CancellationTokenSource? _cancel;
    private bool _disposed;

    /// <summary>The last snapshot. <see cref="RelayStatus.Unknown"/> until the first poll returns.</summary>
    public RelayStatus Latest { get; private set; } = RelayStatus.Unknown;

    /// <summary>True while a request is in flight, which is what the connecting state renders from.</summary>
    public bool IsPolling { get; private set; }

    /// <summary>Raised on the UI loop whenever <see cref="Latest"/> or <see cref="IsPolling"/> changed.</summary>
    public event Action? Changed;

    /// <param name="dispatch">
    /// Hands work to the UI loop. Injected rather than reached for, so this type stays
    /// free of a toolkit and a test can pass a synchronous dispatcher.
    /// </param>
    public RelayPoller(RelayClient client, Action<Action> dispatch)
    {
        Assert.NotNull(client, "a relay poller needs a client to poll through");
        Assert.NotNull(dispatch, "a relay poller needs a UI loop to defer to");

        _client = client;
        _dispatch = dispatch;
    }

    /// <summary>
    /// Converges the poller onto the given target. Idempotent: called twice with the same
    /// target it keeps the running loop rather than restarting it, so a render pass may
    /// call it unconditionally. A null target stops polling.
    /// </summary>
    public void Reconcile(RelayTarget? target)
    {
        Assert.That(!_disposed, "a poller is reconciled before it is disposed");
        Assert.That(target is null || target.Interval > TimeSpan.Zero,
            "a poll interval is positive", target?.Interval);

        if (target == _running)
        {
            return;
        }

        StopLoop();

        _running = target;
        if (target is null)
        {
            Publish(polling: false);
            return;
        }

        _cancel = new CancellationTokenSource();

        // The loop owns its own lifetime through the token; nothing awaits it. Every
        // failure it can meet is already folded into a status by the client.
        _ = LoopAsync(target, _cancel.Token);
    }

    /// <summary>
    /// Polls once outside the loop, for the check-now button. The loop keeps running:
    /// both write through <see cref="Publish"/>, so an overlapping answer is a newer
    /// snapshot rather than a conflict.
    /// </summary>
    public async Task CheckOnceAsync(RelayTarget target, CancellationToken cancellation = default)
    {
        Assert.NotNull(target, "a check names the relay it checks");
        Assert.That(!_disposed, "a poller is checked before it is disposed");

        Publish(polling: true);
        try
        {
            var status = await _client.FetchAsync(target.Host, target.ApiPort, cancellation).ConfigureAwait(false);
            Publish(status, polling: false);
        }
        catch (OperationCanceledException)
        {
            Publish(polling: false);
        }
    }

    private async Task LoopAsync(RelayTarget target, CancellationToken cancellation)
    {
        using var timer = new PeriodicTimer(target.Interval);
        try
        {
            do
            {
                Publish(polling: true);
                var status = await _client.FetchAsync(target.Host, target.ApiPort, cancellation).ConfigureAwait(false);
                Publish(status, polling: false);
            }
            while (await timer.WaitForNextTickAsync(cancellation).ConfigureAwait(false));
        }
        catch (OperationCanceledException)
        {
            // Reconcile or Dispose asked the loop to end. Not a failure to report.
        }
    }

    /// <summary>
    /// The one write. Every field this type owns is assigned here and nowhere else, and
    /// the notification goes out on the UI loop after all of them have landed.
    /// </summary>
    private void Publish(RelayStatus? status = null, bool polling = false)
    {
        _dispatch(() =>
        {
            var changed = IsPolling != polling;
            if (status is not null && !ReferenceEquals(status, Latest))
            {
                Latest = status;
                changed = true;
            }

            IsPolling = polling;
            if (changed)
            {
                Changed?.Invoke();
            }
        });
    }

    private void StopLoop()
    {
        if (_cancel is null)
        {
            return;
        }

        _cancel.Cancel();
        _cancel.Dispose();
        _cancel = null;
        _running = null;
    }

    /// <summary>Idempotent, as every Dispose has to be.</summary>
    public void Dispose()
    {
        if (_disposed)
        {
            return;
        }

        _disposed = true;
        StopLoop();
    }
}
