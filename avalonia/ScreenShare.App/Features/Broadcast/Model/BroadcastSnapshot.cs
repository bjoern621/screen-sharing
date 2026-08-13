using ScreenShare.Api.V1;

namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// One reading of the running stream, composed from what the backend reports: the publish state, the newest
/// encoder sample, and the relay snapshot.
/// A record, so a render pass over an unchanged reading compares equal and every card below it leaves its
/// widgets alone.
///
/// <b>Figures are nullable because "not measured" is a state the screen has to show.</b> It prints as
/// <see cref="Figure.NoValue"/>, and a zero is a measurement rather than an absence.
/// Three sources of absence meet here and the screen does not distinguish them, because the reader's question
/// is the same in all three: nothing is publishing, the encoder has not muxed its first packet, or the sample
/// names the figure in its own <c>missing</c> list.
///
/// <b>Round trip and loss are the relay's, and they name one viewer rather than the stream.</b> The relay
/// measures them per reader and only on the legs instrumented for it, so there is no figure here that is the
/// stream's own.
/// <see cref="RttMs"/> and <see cref="LossPercent"/> are therefore the worst reader's, and the header labels
/// them as such - the alternative was a mean across viewers, which is a number no viewer is experiencing and
/// which a single struggling reader would be averaged out of.
/// The worst is the one a publisher can act on, and the header saying "worst" is what stops it being read as
/// the stream's.
///
/// <b><see cref="CongestionAt"/> is still absent, and permanently.</b> The relay states figures as they stand
/// at each poll and marks no interval as a congestion window; deciding where one started from a series of
/// readings would be this shell inventing a detection nothing performed.
/// It is kept and shown absent rather than removed, on the same rule the settings form greys an option
/// instead of dropping it: a figure the reader is looking for reads as unmeasured, where a missing row reads
/// as nothing to measure (<c>docs/field-availability.md</c>, "The rule").
/// </summary>
public sealed record BroadcastSnapshot
{
    /// <summary>
    /// What the screen renders before the first read lands.
    /// Not live and measuring nothing, which is the honest reading of a session nothing has described yet.
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

    /// <summary>
    /// The worst round trip among the viewers the relay times, absent while it times none.
    /// It names that one viewer and not the stream.
    /// </summary>
    public int? RttMs { get; init; }

    /// <summary>
    /// The worst send-side loss among the viewers the relay states one for, absent while it states none.
    /// It names that one viewer and not the stream.
    /// </summary>
    public double? LossPercent { get; init; }

    /// <summary>How many readers the relay reports on this stream's path.</summary>
    public int? Viewers { get; init; }

    /// <summary>
    /// The legs this stream's viewers are watching over, in the transport vocabulary, empty while the relay
    /// names no reader on the path.
    ///
    /// It is the fact behind every sentence about a latency figure nobody stated.
    /// SRT is the one leg the relay times, so a stream watched over anything else has viewers and no round
    /// trip, and naming the legs it is watched over is what leaves a publisher something to change.
    ///
    /// One comma-separated string rather than a list, because this record's equality is what keeps a render
    /// pass over an unchanged reading from repainting, and a fresh list would compare unequal on every pass.
    /// </summary>
    public string Legs { get; init; } = "";

    /// <summary>
    /// The path this stream publishes to, empty while nothing is publishing.
    /// It is here because it is the key every relay figure on this screen is looked up by, and a card that
    /// had to find it again would be a second definition of which path is ours.
    /// </summary>
    public string Stream { get; init; } = "";

    public int? Cq { get; init; }

    /// <summary>The output size the stream was built for, empty where it publishes at the source size.</summary>
    public string Resolution { get; init; } = "";

    public double? VbvCeilingMbps { get; init; }

    /// <summary>
    /// Never detected: the relay states figures as they stand and marks no interval, so no congestion window
    /// has a start to name.
    /// </summary>
    public string CongestionAt { get; init; } = "";

    /// <summary>
    /// Composes one reading from the three whole states the backend last sent.
    ///
    /// Nothing here decides anything.
    /// Each figure is read out of the message that carries it, and the one lookup that is not a field access
    /// - the relay path for this stream - is a match on the name the publish state itself states.
    /// </summary>
    public static BroadcastSnapshot Of(PublishState? publish, PublishStats? stats, RelayStatus? relay)
    {
        if (publish is null)
        {
            return Unread;
        }

        // Live is present exactly while a stream is in force, and a live one always carries the settings it
        // was built from - both are message presence rather than flags, so there is no combination here to
        // reconcile.
        // A retry hangs off the live stream, which is what makes "an attempt belongs to a retry" something
        // this reading cannot get wrong.
        var live = publish.Live;
        var settings = live?.Publish;
        var retry = live?.Retry;

        var stream = settings?.Name ?? "";
        var path = PathOf(relay, stream);

        return new BroadcastSnapshot
        {
            IsLive = live is not null,
            IsRetrying = retry is not null,
            Attempt = retry?.Attempt ?? 0,
            Budget = retry?.Budget ?? 0,
            Elapsed = Clock(stats),
            EgressMbps = Measured(stats, sample => sample.HasInstMbps, sample => sample.InstMbps),
            Fps = Measured(stats, sample => sample.HasFps, sample => sample.Fps),
            RttMs = WorstRttMs(path) is { } rtt ? (int)Math.Round(rtt) : null,
            LossPercent = WorstLossPercent(path),
            Viewers = path?.Readers,
            Legs = LegsOf(path),
            Stream = stream,
            Cq = settings?.Cq,
            Resolution = settings?.OutputResolution ?? "",
            VbvCeilingMbps = settings?.MaxrateMbps,
        };
    }

    /// <summary>
    /// The relay's entry for one stream, null while there is no snapshot, no stream, an unreachable relay, or
    /// no path by that name yet - a stream that has just started publishes before the relay's next poll sees
    /// it.
    ///
    /// Every relay figure on this screen goes through here: the header's viewer count, the rows under it and
    /// the latency plot beside them all describe the same path because they ask one function which path that
    /// is (<c>docs/development-principles.md</c>, "A fact lives in one table").
    /// </summary>
    public static RelayPath? PathOf(RelayStatus? relay, string stream)
    {
        if (relay is null || !relay.Reachable || string.IsNullOrEmpty(stream))
        {
            return null;
        }

        foreach (var path in relay.Paths)
        {
            if (path.Name == stream)
            {
                return path;
            }
        }

        return null;
    }

    /// <summary>
    /// The highest round trip on the path's roster, and null where no reader on it is timed.
    /// The worst rather than the mean, for the reason the class comment gives.
    /// </summary>
    public static double? WorstRttMs(RelayPath? path) => Worst(path, reader => reader.HasRttMs ? reader.RttMs : null);

    /// <summary>The highest send-side loss on the path's roster, and null where no reader states one.</summary>
    public static double? WorstLossPercent(RelayPath? path)
        => Worst(path, reader => reader.HasLossPercent ? reader.LossPercent : null);

    /// <summary>
    /// The distinct legs the path's readers are on, in the order the roster names them.
    /// A reader the relay named no protocol for takes no part: an unnamed leg is not a leg, and listing it as
    /// an empty one would put a gap in the sentence that names them.
    /// </summary>
    private static string LegsOf(RelayPath? path)
    {
        if (path is null)
        {
            return "";
        }

        var legs = new List<string>();
        foreach (var reader in path.ReaderRoster)
        {
            if (reader.Transport.Length > 0 && !legs.Contains(reader.Transport))
            {
                legs.Add(reader.Transport);
            }
        }

        return string.Join(", ", legs);
    }

    /// <summary>
    /// The largest of one figure across the readers that report it.
    /// A reader that does not report it takes no part: an untimed viewer is not a viewer with a round trip of
    /// zero, and counting it as one would make every roster with an RTMP viewer in it look perfect.
    /// </summary>
    private static double? Worst(RelayPath? path, Func<RelayReader, double?> figure)
    {
        if (path is null)
        {
            return null;
        }

        double? worst = null;
        foreach (var reader in path.ReaderRoster)
        {
            if (figure(reader) is { } measured && (worst is null || measured > worst))
            {
                worst = measured;
            }
        }

        return worst;
    }

    /// <summary>
    /// One figure of a sample, absent where the sample carries no measurement for it.
    /// Presence is the contract's own answer to "is this a measured zero", so it is honoured rather than
    /// second-guessed by testing the value against zero.
    /// It used to be a list of field names the sample carried alongside the figures; the names are gone and
    /// the question is now asked of the field itself.
    /// </summary>
    private static double? Measured(PublishStats? stats, Func<PublishStats, bool> has, Func<PublishStats, double> read)
        => stats is null || !has(stats) ? null : read(stats);

    /// <summary>The encoder's own running time, as the pill's zero-padded timer.</summary>
    private static string Clock(PublishStats? stats)
    {
        var seconds = Measured(stats, sample => sample.HasTimeSec, sample => sample.TimeSec);
        return seconds is null ? Figure.NoValue : TimeSpan.FromSeconds(seconds.Value).ToString(@"hh\:mm\:ss");
    }
}
