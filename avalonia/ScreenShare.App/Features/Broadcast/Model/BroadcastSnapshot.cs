using ScreenShare.Api.V1;

namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// One reading of the running stream, composed from what the backend reports: the publish
/// state, the newest encoder sample, and the relay snapshot. A record, so a render pass over
/// an unchanged reading compares equal and every card below it leaves its widgets alone.
///
/// <b>Figures are nullable because "not measured" is a state the screen has to show.</b> It
/// prints as <see cref="Figure.NoValue"/>, and a zero is a measurement rather than an absence.
/// Three sources of absence meet here and the screen does not distinguish them, because the
/// reader's question is the same in all three: nothing is publishing, the encoder has not
/// muxed its first packet, or the sample names the figure in its own <c>missing</c> list.
///
/// <b>Two figures are absent permanently, and that is a fact about the backend rather than
/// about this moment.</b> Nothing in the pipeline measures round-trip time or packet loss, and
/// nothing marks a congestion window, so <see cref="RttMs"/>, <see cref="LossPercent"/> and
/// <see cref="CongestionAt"/> have no source to read. They are kept and shown absent rather
/// than removed, on the same rule the settings form greys an option instead of dropping it: a
/// figure the reader is looking for reads as unmeasured, where a missing row reads as nothing
/// to measure (<c>docs/field-availability.md</c>, "The rule").
/// </summary>
public sealed record BroadcastSnapshot
{
    /// <summary>
    /// What the screen renders before the first read lands. Not live and measuring nothing,
    /// which is the honest reading of a session nothing has described yet.
    /// </summary>
    public static readonly BroadcastSnapshot Unread = new();

    /// <summary>Whether a stream is in force. True across a retry backoff too: that is a stream the reader has not stopped.</summary>
    public bool IsLive { get; init; }

    /// <summary>Whether the pipeline died on its own and a relaunch is waiting out a backoff.</summary>
    public bool IsRetrying { get; init; }

    /// <summary>Which relaunch is pending and how many the backend will spend, both zero while nothing retries.</summary>
    public int Attempt { get; init; }

    public int Budget { get; init; }

    /// <summary>How long the encoder has been running, <c>HH:MM:SS</c>, from the sample's own clock.</summary>
    public string Elapsed { get; init; } = Figure.NoValue;

    public double? EgressMbps { get; init; }

    public double? Fps { get; init; }

    /// <summary>Never measured: nothing in the pipeline reports a round trip.</summary>
    public int? RttMs { get; init; }

    /// <summary>Never measured: nothing in the pipeline reports loss.</summary>
    public double? LossPercent { get; init; }

    /// <summary>How many readers the relay reports on this stream's path.</summary>
    public int? Viewers { get; init; }

    public int? Cq { get; init; }

    /// <summary>The output size the stream was built for, empty where it publishes at the source size.</summary>
    public string Resolution { get; init; } = "";

    public double? VbvCeilingMbps { get; init; }

    /// <summary>Never detected: nothing marks a congestion window.</summary>
    public string CongestionAt { get; init; } = "";

    /// <summary>
    /// Composes one reading from the three whole states the backend last sent.
    ///
    /// Nothing here decides anything. Each figure is read out of the message that carries it,
    /// and the one lookup that is not a field access - the relay path for this stream - is a
    /// match on the name the publish state itself states.
    /// </summary>
    public static BroadcastSnapshot Of(PublishState? publish, PublishStats? stats, RelayStatus? relay)
    {
        if (publish is null)
        {
            return Unread;
        }

        // Live is present exactly while a stream is in force, and a live one always carries the
        // settings it was built from - both are message presence rather than flags, so there is
        // no combination here to reconcile. A retry hangs off the live stream, which is what
        // makes "an attempt belongs to a retry" something this reading cannot get wrong.
        var live = publish.Live;
        var settings = live?.Publish;
        var retry = live?.Retry;

        return new BroadcastSnapshot
        {
            IsLive = live is not null,
            IsRetrying = retry is not null,
            Attempt = retry?.Attempt ?? 0,
            Budget = retry?.Budget ?? 0,
            Elapsed = Clock(stats),
            EgressMbps = Measured(stats, sample => sample.HasInstMbps, sample => sample.InstMbps),
            Fps = Measured(stats, sample => sample.HasFps, sample => sample.Fps),
            Viewers = Readers(relay, settings?.Name),
            Cq = settings?.Cq,
            Resolution = settings?.OutputResolution ?? "",
            VbvCeilingMbps = settings?.MaxrateMbps,
        };
    }

    /// <summary>
    /// One figure of a sample, absent where the sample carries no measurement for it. Presence is
    /// the contract's own answer to "is this a measured zero", so it is honoured rather than
    /// second-guessed by testing the value against zero. It used to be a list of field names the
    /// sample carried alongside the figures; the names are gone and the question is now asked of
    /// the field itself.
    /// </summary>
    private static double? Measured(PublishStats? stats, Func<PublishStats, bool> has, Func<PublishStats, double> read)
        => stats is null || !has(stats) ? null : read(stats);

    /// <summary>The encoder's own running time, as the pill's zero-padded timer.</summary>
    private static string Clock(PublishStats? stats)
    {
        var seconds = Measured(stats, sample => sample.HasTimeSec, sample => sample.TimeSec);
        return seconds is null ? Figure.NoValue : TimeSpan.FromSeconds(seconds.Value).ToString(@"hh\:mm\:ss");
    }

    /// <summary>
    /// How many viewers the relay reports on the published stream's path, absent while there is
    /// no snapshot, no stream, or no path by that name yet - a stream that has just started
    /// publishes before the relay's next poll sees it.
    /// </summary>
    private static int? Readers(RelayStatus? relay, string? stream)
    {
        if (relay is null || !relay.Reachable || string.IsNullOrEmpty(stream))
        {
            return null;
        }

        foreach (var path in relay.Paths)
        {
            if (path.Name == stream)
            {
                return path.Readers;
            }
        }

        return null;
    }
}
