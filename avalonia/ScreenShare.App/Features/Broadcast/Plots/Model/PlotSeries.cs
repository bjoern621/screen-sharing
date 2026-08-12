using Avalonia;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.Model;

namespace ScreenShare.App.Features.Broadcast.Plots.Model;

/// <summary>
/// The sparkline geometry: readings mapped into the space the design was drawn in.
///
/// <see cref="Extent"/> is that space: X runs left to right across <see cref="WindowSeconds"/>,
/// Y runs 0 at the top to 104 at the bottom. Keeping the curve in design coordinates is what
/// lets a wider card show the same window stretched rather than a different window of it.
///
/// <b>All three series have a source, and they do not share a clock.</b> The encoder reports its
/// rate per interval against its own running time, so the egress curve is placed by that clock.
/// The other two are the relay's per-reader figures, one point per relay snapshot, placed by when
/// this shell received the snapshot - the relay puts no time on one, and the backend polls it on
/// an interval that is not on the contract, so the arrival stamp is the only clock either curve
/// can be drawn against.
///
/// <b>Both latency series describe one viewer, and it need not be the same one twice.</b> Each
/// point is the worst reader on the path at that snapshot, and worst by round trip and worst by
/// loss can be two different viewers - which is the honest reading of a plot whose question is
/// "is anybody having a bad time", and the reason the card names them as the worst rather than as
/// the stream's.
///
/// The vertical scale is the window's own peak rather than the encoder's ceiling, and that is
/// deliberate: a sparkline with no axis says "how has this moved", and scaling to a ceiling
/// the stream never approaches flattens the only thing it has to say. The ceiling is stated as
/// a label instead, and <see cref="CeilingFraction"/> places the rule marking it against that
/// same peak - which is why a stream running well under its ceiling gets no rule at all.
/// </summary>
public static class PlotSeries
{
    public static readonly Size Extent = new(480, 104);

    /// <summary>
    /// How much stream a plot covers, and the whole width is always this much.
    ///
    /// The newest reading sits at the right edge and one this old at the left, so a run younger
    /// than the window fills the right of the card and leaves the rest of it empty. It is not
    /// stretched to fill: a curve that stretched would redraw at a new density on every sample,
    /// would put "a minute ago" at a different place on the card each second, and would compress
    /// a long run until the dip it exists to show flattened. A plot with no axis has one thing to
    /// keep still, and it is where a moment sits.
    ///
    /// This is a span and not a count of readings because both series are stamped: the encoder
    /// states the running time each sample was taken at, and the session stamps each relay
    /// snapshot as it arrives. Neither cadence has to be known or assumed to place a point.
    /// </summary>
    public const double WindowSeconds = 60;

    /// <summary>
    /// How much of the height the curve is allowed, leaving the rest as headroom so a peak does
    /// not sit flush against the top edge.
    /// </summary>
    private const double Headroom = 0.85;

    /// <summary>One measurement and when it was taken, in seconds on whichever clock its series is kept on.</summary>
    private readonly record struct Reading(double At, double Value);

    /// <summary>
    /// The egress curve for one run of samples, empty where there is nothing to draw. A single
    /// sample draws nothing either: one point is a reading and not a shape, and a line through
    /// it would read as a flat stream rather than as a stream nobody has watched yet.
    /// </summary>
    public static IReadOnlyList<Point> Egress(IReadOnlyList<PublishStats> samples)
    {
        Assert.NotNull(samples, "a series is drawn from the samples that were taken");

        return Curve(Rates(samples), Now(samples));
    }

    /// <summary>
    /// The two latency curves over one run of relay snapshots, drawn from the worst reader on
    /// the given path at each snapshot.
    /// </summary>
    /// <param name="Rtt">Round trip in milliseconds, scaled to its own peak.</param>
    /// <param name="Loss">Send-side loss as a percentage, scaled to its own peak.</param>
    public sealed record LatencySeries(IReadOnlyList<Point> Rtt, IReadOnlyList<Point> Loss);

    /// <summary>
    /// Both latency curves for one stream, empty where there is nothing to draw.
    ///
    /// They are produced together rather than by two calls, because they have to cover the same
    /// window to be read against each other: a snapshot contributes a point to both curves or to
    /// neither, so an X on one is the same moment as an X on the other. A snapshot the relay was
    /// unreachable for, one taken before this stream had a path, and one whose readers are all on
    /// legs nobody times each contribute nothing - which is the same rule the egress curve applies
    /// to a sample with no rate, and for the same reason: a zero would draw as a viewer with a
    /// perfect link rather than as a viewer nobody measured.
    ///
    /// In practice the legs that report the two report both, because SRT is the one that reports
    /// either. That is why one filter serves both curves rather than each dropping its own
    /// snapshots and the two drifting out of step.
    /// </summary>
    public static LatencySeries Latency(IReadOnlyList<RelayReading> samples, string stream)
    {
        Assert.NotNull(samples, "a latency series is drawn from the snapshots that were taken");
        Assert.NotNull(stream, "a latency series describes one stream's path");

        var rtts = new List<Reading>(samples.Count);
        var losses = new List<Reading>(samples.Count);
        foreach (var sample in samples)
        {
            var path = BroadcastSnapshot.PathOf(sample.Status, stream);
            if (BroadcastSnapshot.WorstRttMs(path) is not { } rtt
                || BroadcastSnapshot.WorstLossPercent(path) is not { } loss)
            {
                continue;
            }

            var at = Seconds(sample.At);
            rtts.Add(new Reading(at, rtt));
            losses.Add(new Reading(at, loss));
        }

        // The axis ends at the newest snapshot and not at the newest one that timed somebody, so
        // a curve of readings older than the window leaves the plot rather than sitting there
        // describing viewers who have since gone. The card then says nobody is being timed, which
        // is what is true.
        var now = samples.Count == 0 ? (double?)null : Seconds(samples[^1].At);
        var series = new LatencySeries(Curve(rtts, now), Curve(losses, now));

        Assert.That(series.Rtt.Count == series.Loss.Count,
            "the two latency curves cover the same window", series.Rtt.Count, series.Loss.Count);
        return series;
    }

    /// <summary>
    /// Where the encoder's ceiling falls on the curve above, as a fraction of the plot height,
    /// and <see cref="double.NaN"/> where it falls nowhere on it.
    ///
    /// It is derived from the same peak the curve is scaled to - the same readings through the
    /// same window - because a rule drawn at a fixed height would claim the ceiling is wherever
    /// the design put it. A stream running well under its ceiling puts that rule off the top of
    /// the plot, and then there is no rule: the label still states the figure, and an edge the
    /// reader cannot see is not a scale.
    /// </summary>
    public static double CeilingFraction(IReadOnlyList<PublishStats> samples, double? ceilingMbps)
    {
        Assert.NotNull(samples, "a ceiling is placed against the samples that were taken");

        if (ceilingMbps is not { } ceiling || ceiling <= 0)
        {
            return double.NaN;
        }

        var drawn = Windowed(Rates(samples), Now(samples));
        if (drawn.Count < 2)
        {
            return double.NaN;
        }

        var peak = Peak(drawn);
        if (peak <= 0)
        {
            return double.NaN;
        }

        // The inverse of the mapping in Curve: a rate's share of the peak, taken off the floor.
        var fraction = 1 - (Headroom * ceiling / peak);
        return fraction is >= 0 and <= 1 ? fraction : double.NaN;
    }

    /// <summary>
    /// The window's readings mapped into the drawing space: placed horizontally by when they were
    /// taken, and scaled vertically to their own peak.
    ///
    /// Every curve on this card goes through here, so the scale a reader learns from one applies
    /// to the next: the vertical is always "against this window's own highest", never against a
    /// figure from elsewhere, and the horizontal is always "how long ago". A single reading draws
    /// nothing - one point is a reading and not a shape, and a line through it would read as a
    /// flat run rather than as a run nobody has watched yet.
    /// </summary>
    private static IReadOnlyList<Point> Curve(IReadOnlyList<Reading> readings, double? now)
    {
        var drawn = Windowed(readings, now);
        if (drawn.Count < 2 || now is not { } anchor)
        {
            return [];
        }

        var peak = Peak(drawn);

        var points = new Point[drawn.Count];
        for (var i = 0; i < drawn.Count; i++)
        {
            // Right edge is now. A reading a whole window old sits on the left edge, and the
            // width between them is time rather than a share of however many readings there are.
            var x = Extent.Width * (1 - ((anchor - drawn[i].At) / WindowSeconds));

            // Inverted, because Y grows downward in the drawing space: a higher value is a
            // smaller Y. A window whose peak is zero draws along the floor rather than dividing,
            // which is the right shape for a loss curve that has stayed at nothing.
            var share = peak > 0 ? drawn[i].Value / peak : 0;
            var y = Extent.Height - (Extent.Height * Headroom * share);

            points[i] = new Point(x, y);
        }

        Assert.That(points[0].X >= 0 && points[^1].X <= Extent.Width,
            "a drawn curve stays inside the window", points[0].X, points[^1].X);
        return points;
    }

    /// <summary>
    /// The readings the window covers, oldest first: every reading no older than
    /// <see cref="WindowSeconds"/> before <paramref name="now"/> and on the same clock as the
    /// newest of them. Empty where the series ended before the window began.
    ///
    /// It walks back from the newest rather than filtering the whole series, because that is what
    /// handles the one discontinuity a series can carry. A pipeline that dies and is relaunched
    /// leaves the stream live, so its samples are appended to the ones the previous pipeline took
    /// and its running clock starts again at zero. The walk stops where time goes backwards, and
    /// the plot then draws the run the newest reading belongs to instead of two runs laid over
    /// each other.
    /// </summary>
    private static List<Reading> Windowed(IReadOnlyList<Reading> readings, double? now)
    {
        if (readings.Count == 0 || now is not { } anchor || anchor - readings[^1].At > WindowSeconds)
        {
            return [];
        }

        var first = readings.Count - 1;
        while (first > 0
               && readings[first - 1].At <= readings[first].At
               && anchor - readings[first - 1].At <= WindowSeconds)
        {
            first--;
        }

        var drawn = new List<Reading>(readings.Count - first);
        for (var i = first; i < readings.Count; i++)
        {
            drawn.Add(readings[i]);
        }

        return drawn;
    }

    /// <summary>
    /// Where the egress axis ends: the running time of the newest sample that states one, and
    /// null where no sample does. It is the newest sample rather than the newest one carrying a
    /// rate, so an encoder that goes on reporting without a rate slides its curve off the plot
    /// instead of freezing it at the last shape it had.
    /// </summary>
    private static double? Now(IReadOnlyList<PublishStats> samples)
    {
        for (var i = samples.Count - 1; i >= 0; i--)
        {
            if (samples[i].HasTimeSec)
            {
                return samples[i].TimeSec;
            }
        }

        return null;
    }

    /// <summary>One instant on the shell's clock, as seconds, which is the unit both axes are placed in.</summary>
    private static double Seconds(DateTimeOffset at) => (at - DateTimeOffset.UnixEpoch).TotalSeconds;

    private static double Peak(List<Reading> readings)
    {
        var peak = 0d;
        foreach (var reading in readings)
        {
            if (reading.Value > peak)
            {
                peak = reading.Value;
            }
        }

        return peak;
    }

    /// <summary>
    /// The measured rates of a run, in order, each at the moment the encoder took it. A sample
    /// that carries no rate contributes nothing, rather than a zero that would draw as a stream
    /// that stalled - presence is what says which it is. So does one that carries no time: the
    /// horizontal position is a moment, and a rate with no moment cannot be placed on it.
    /// </summary>
    private static List<Reading> Rates(IReadOnlyList<PublishStats> samples)
    {
        var rates = new List<Reading>(samples.Count);
        foreach (var sample in samples)
        {
            if (sample.HasInstMbps && sample.HasTimeSec)
            {
                rates.Add(new Reading(sample.TimeSec, sample.InstMbps));
            }
        }

        return rates;
    }
}
