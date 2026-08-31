using ScreenShare.Api.V1;

namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// One reading of the running stream, composed from three states the backend reports: the publish state,
/// the newest encoder sample and the relay snapshot.
/// Record, so a pass over an unchanged reading compares equal and every card below leaves its widgets alone.
///
/// <b>Figures are nullable, "not measured" being a state this screen shows.</b> It prints
/// as <see cref="Figure.NoValue"/>, and a zero is a measurement.
/// Three sources of absence arrive undistinguished, the reader's question being the same in all three: nothing
/// publishing, no first packet muxed, or a sample carrying no value for the figure.
/// What the screen draws is this reading with each gap filled from the last pass that measured it
/// (<see cref="HeldFigures"/>), so the ellipsis reaches a figure nothing has measured on this run.
///
/// <b>Round trip and loss are per reader, on the legs the relay instruments, so neither has a stream-wide
/// value.</b> <see cref="RttMs"/> and <see cref="LossPercent"/> are the worst reader's, labelled as such wherever
/// they are drawn.
/// A mean would be a figure no viewer is experiencing and would average one struggling reader away.
///
/// <b><see cref="CongestionAt"/> is permanently absent.</b> The relay states figures as they stand at each poll
/// and marks no interval, so naming where a congestion window started would be a detection this shell performed
/// and attributed to the backend.
/// Kept and shown absent rather than dropped, on the rule that greys an option instead of removing it:
/// an absent figure reads as unmeasured, a missing row as nothing to measure
/// (<c>docs/field-availability.md</c>, "The rule").
/// </summary>
public sealed record BroadcastSnapshot
{
    /// <summary>
    /// What the screen renders before the first read lands.
    /// Not live and measuring nothing, the honest reading of a session nothing has described yet.
    /// </summary>
    public static readonly BroadcastSnapshot Unread = new();

    /// <summary>Whether a stream is in force. True across a retry backoff: that stream was never stopped.</summary>
    public bool IsLive { get; init; }

    /// <summary>Whether the pipeline died on its own and a relaunch is waiting out a backoff.</summary>
    public bool IsRetrying { get; init; }

    /// <summary>Which relaunch is pending and how many the backend spends, both zero while nothing retries.</summary>
    public int Attempt { get; init; }

    public int Budget { get; init; }

    /// <summary>
    /// What ended the pipeline this relaunch follows, absent where nothing named it.
    /// Read off the state rather than off the exit event, so a window that opened mid-backoff says why as well
    /// as one that was listening when the pipeline died.
    /// </summary>
    public Text? RetryCause { get; init; }

    /// <summary>
    /// That pipeline's own last words, raw and never matched against
    /// (<c>api/proto/screenshare/v1/text.proto</c>).
    /// Empty where it said nothing.
    /// </summary>
    public string RetryMessage { get; init; } = "";

    /// <summary>Encoder's running time off the sample's own clock, zero-padded: <c>01:07:44</c>.</summary>
    public string Elapsed { get; init; } = Figure.NoValue;

    public double? EgressMbps { get; init; }

    public double? Fps { get; init; }

    /// <summary>
    /// ms this machine holds a frame between reading it off the screen and having it encoded and ready to send,
    /// measured on the running pipeline over the last interval.
    /// The one stage of a viewer's delay this side both causes and can shorten, hence the figure the publish
    /// screen promotes rather than the windows the transports hold packets for.
    /// Absent on an engine that measures none, and on the first sample of a run.
    /// </summary>
    public double? EncodeMs { get; init; }

    /// <summary>
    /// Worst round trip in ms among the viewers the relay times, absent while it times none.
    /// Names that one viewer, never the stream.
    /// </summary>
    public int? RttMs { get; init; }

    /// <summary>
    /// Worst send-side loss among the viewers the relay states one for, absent while it states none.
    /// Names that one viewer, never the stream.
    /// </summary>
    public double? LossPercent { get; init; }

    /// <summary>Readers the relay reports on this stream's path.</summary>
    public int? Viewers { get; init; }

    /// <summary>
    /// Legs this stream's viewers watch over, in the transport vocabulary: <c>"srt, rtmp"</c>.
    /// Empty while the relay names no reader on the path.
    ///
    /// SRT is the one leg the relay times, so a stream watched over anything else has viewers and no round trip,
    /// and this is what a sentence about an untimed figure names.
    ///
    /// One comma-separated string rather than a list: this record's equality keeps an unchanged pass
    /// from repainting, and a fresh list compares unequal every pass.
    /// </summary>
    public string Legs { get; init; } = "";

    /// <summary>
    /// Path this stream publishes to, empty while nothing is publishing.
    /// Every relay figure on this screen is looked up by it, so a card finding it again would be a second
    /// definition of which path is this machine's.
    /// </summary>
    public string Stream { get; init; } = "";

    public int? Cq { get; init; }

    /// <summary>Output size the stream was built for, empty where it publishes at the source size.</summary>
    public string Resolution { get; init; } = "";

    /// <summary>
    /// Rate the running encoder is held to, absent where the encode is bounded by nothing.
    /// The backend's reading rather than the ceiling field beside it: which figure bounds an encode, and whether
    /// one does at all, follows the rate-control mode and the encoder element.
    /// </summary>
    public double? VbvCeilingMbps { get; init; }

    /// <summary>
    /// Never filled: the relay marks no interval, so no congestion window has a start to name.
    /// Nothing draws it either, a caption placed over a band the plot cannot shade being a window the shell would
    /// be naming on its own (<c>Plots/View/PlotsView.axaml</c>).
    /// </summary>
    public string CongestionAt { get; init; } = "";

    /// <summary>
    /// Composes one reading from the three whole states the backend last sent.
    /// Nothing here decides anything: each figure is read out of the message carrying it.
    /// The one lookup that is not a field access, the relay path, matches on the name the publish state itself
    /// states.
    /// </summary>
    public static BroadcastSnapshot Of(PublishState? publish, PublishStats? stats, RelayStatus? relay)
    {
        if (publish is null)
        {
            return Unread;
        }

        // Live is present exactly while a stream is in force and always carries the settings it was built from,
        // both as message presence rather than as flags, so there is no combination to reconcile.
        // A retry hangs off the live stream, so an attempt cannot be read outside the retry it belongs to.
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
            RetryCause = retry?.Cause,
            RetryMessage = retry?.Message ?? "",
            Elapsed = Clock(stats),
            EgressMbps = Measured(stats, sample => sample.HasInstMbps, sample => sample.InstMbps),
            Fps = Measured(stats, sample => sample.HasFps, sample => sample.Fps),
            EncodeMs = Measured(stats, sample => sample.HasTransitMs, sample => sample.TransitMs),
            RttMs = WorstRttMs(path) is { } rtt ? (int)Math.Round(rtt) : null,
            LossPercent = WorstLossPercent(path),
            Viewers = path?.Readers,
            Legs = LegsOf(path),
            Stream = stream,
            Cq = settings?.Cq,
            Resolution = settings?.OutputResolution ?? "",
            VbvCeilingMbps = live is { HasRateCeilingMbps: true } ? live.RateCeilingMbps : null,
        };
    }

    /// <summary>
    /// Relay entry for one stream.
    /// Null on no snapshot, no stream, an unreachable relay, or no path by that name yet: a stream that has just
    /// started publishes before the relay's next poll sees it.
    /// Every relay figure on this screen goes through here, so the viewer count, the rows under it and
    /// the latency plot describe one path (<c>docs/development-principles.md</c>, "A fact lives in one table").
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

    /// <summary>Highest round trip on the path's roster, null where no reader on it is timed.</summary>
    public static double? WorstRttMs(RelayPath? path) => Worst(path, reader => reader.HasRttMs ? reader.RttMs : null);

    /// <summary>Highest send-side loss on the path's roster, null where no reader states one.</summary>
    public static double? WorstLossPercent(RelayPath? path)
        => Worst(path, reader => reader.HasLossPercent ? reader.LossPercent : null);

    /// <summary>
    /// Distinct legs the path's readers are on, in roster order.
    /// A reader the relay named no protocol for takes no part: an unnamed leg listed as an empty one puts a gap
    /// in the sentence naming them.
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

        return string.Join(", ", legs.Select(Copy.Words.Transport));
    }

    /// <summary>
    /// Largest value of one figure across the readers reporting it.
    /// A reader that reports none takes no part: an untimed viewer is not a viewer at zero, and counting it as one
    /// makes every roster holding an RTMP viewer look perfect.
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
    /// Presence is the contract's own answer to "is this a measured zero", so it is read rather than second-guessed
    /// by comparing the value against zero.
    /// </summary>
    private static double? Measured(PublishStats? stats, Func<PublishStats, bool> has, Func<PublishStats, double> read)
        => stats is null || !has(stats) ? null : read(stats);

    /// <summary>Encoder's running time as the pill's zero-padded timer, the ellipsis before the first sample.</summary>
    /// <remarks>
    /// Hours are totalled rather than formatted.
    /// The hh specifier is a span's hours component, 0..23, and drops the days beside it, so a share running
    /// over a day would read 01:00:00 at the 25-hour mark and start the clock again.
    /// </remarks>
    private static string Clock(PublishStats? stats)
    {
        var seconds = Measured(stats, sample => sample.HasTimeSec, sample => sample.TimeSec);
        if (seconds is null)
        {
            return Figure.NoValue;
        }
        var elapsed = TimeSpan.FromSeconds(seconds.Value);
        return $"{(int)elapsed.TotalHours:00}:{elapsed.Minutes:00}:{elapsed.Seconds:00}";
    }
}
