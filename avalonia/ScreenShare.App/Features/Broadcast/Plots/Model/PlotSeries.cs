using Avalonia;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.Model;

namespace ScreenShare.App.Features.Broadcast.Plots.Model;

/// <summary>
/// The sparkline geometry: encoder samples mapped into the space the design was drawn in.
///
/// <see cref="Extent"/> is that space: X runs left to right across the window of samples held,
/// Y runs 0 at the top to 104 at the bottom. Keeping the curve in design coordinates is what
/// lets a wider card show the same shape stretched rather than a different window of it.
///
/// <b>All three series have a source, and they do not share a clock.</b> The encoder reports its
/// rate per interval, so the egress curve is the samples' own shape. The other two are the relay's
/// per-reader figures, one point per relay snapshot, and the relay is polled on the backend's
/// interval rather than the encoder's - so the two cards cover the same run and not the same
/// window, which is why only the egress card states a span. The design's second latency series was
/// buffer fill; nothing reports a viewer's buffer to a publisher, and loss is what the relay does
/// measure alongside the round trip, so that is what is drawn beside it.
///
/// <b>Both latency series describe one viewer, and it need not be the same one twice.</b> Each
/// point is the worst reader on the path at that snapshot, and worst by round trip and worst by
/// loss can be two different viewers - which is the honest reading of a plot whose question is
/// "is anybody having a bad time", and the reason the card names them as the worst rather than as
/// the stream's.
///
/// The vertical scale is the samples' own peak rather than the encoder's ceiling, and that is
/// deliberate: a sparkline with no axis says "how has this moved", and scaling to a ceiling
/// the stream never approaches flattens the only thing it has to say. The ceiling is stated as
/// a label instead, and <see cref="CeilingFraction"/> places the rule marking it against that
/// same peak - which is why a stream running well under its ceiling gets no rule at all.
/// </summary>
public static class PlotSeries
{
    public static readonly Size Extent = new(480, 104);

    /// <summary>
    /// How much of the height the curve is allowed, leaving the rest as headroom so a peak does
    /// not sit flush against the top edge.
    /// </summary>
    private const double Headroom = 0.85;

    /// <summary>
    /// The egress curve for one run of samples, empty where there is nothing to draw. A single
    /// sample draws nothing either: one point is a reading and not a shape, and a line through
    /// it would read as a flat stream rather than as a stream nobody has watched yet.
    /// </summary>
    public static IReadOnlyList<Point> Egress(IReadOnlyList<PublishStats> samples)
    {
        Assert.NotNull(samples, "a series is drawn from the samples that were taken");

        return Curve(Rates(samples));
    }

    /// <summary>
    /// Measured values in order, mapped into the drawing space and scaled to their own peak.
    ///
    /// Every curve on this card goes through here, so the scale a reader learns from one applies
    /// to the next: the vertical is always "against this run's own highest", never against a
    /// figure from elsewhere. A single value draws nothing either - one point is a reading and
    /// not a shape, and a line through it would read as a flat run rather than as a run nobody
    /// has watched yet.
    /// </summary>
    private static IReadOnlyList<Point> Curve(IReadOnlyList<double> values)
    {
        if (values.Count < 2)
        {
            return [];
        }

        var peak = values.Max();
        var points = new Point[values.Count];
        for (var i = 0; i < values.Count; i++)
        {
            var x = Extent.Width * i / (values.Count - 1);

            // Inverted, because Y grows downward in the drawing space: a higher value is a
            // smaller Y. A run whose peak is zero draws along the floor rather than dividing,
            // which is the right shape for a loss curve that has stayed at nothing.
            var share = peak > 0 ? values[i] / peak : 0;
            var y = Extent.Height - (Extent.Height * Headroom * share);

            points[i] = new Point(x, y);
        }

        return points;
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
    public static LatencySeries Latency(IReadOnlyList<RelayStatus> samples, string stream)
    {
        Assert.NotNull(samples, "a latency series is drawn from the snapshots that were taken");
        Assert.NotNull(stream, "a latency series describes one stream's path");

        var rtts = new List<double>(samples.Count);
        var losses = new List<double>(samples.Count);
        foreach (var sample in samples)
        {
            var path = BroadcastSnapshot.PathOf(sample, stream);
            if (BroadcastSnapshot.WorstRttMs(path) is not { } rtt
                || BroadcastSnapshot.WorstLossPercent(path) is not { } loss)
            {
                continue;
            }

            rtts.Add(rtt);
            losses.Add(loss);
        }

        var series = new LatencySeries(Curve(rtts), Curve(losses));

        Assert.That(series.Rtt.Count == series.Loss.Count,
            "the two latency curves cover the same window", series.Rtt.Count, series.Loss.Count);
        return series;
    }

    /// <summary>
    /// Where the encoder's ceiling falls on the curve above, as a fraction of the plot height,
    /// and <see cref="double.NaN"/> where it falls nowhere on it.
    ///
    /// It is derived from the same peak the curve is scaled to, because a rule drawn at a fixed
    /// height would claim the ceiling is wherever the design put it. A stream running well under
    /// its ceiling puts that rule off the top of the plot, and then there is no rule: the label
    /// still states the figure, and an edge the reader cannot see is not a scale.
    /// </summary>
    public static double CeilingFraction(IReadOnlyList<PublishStats> samples, double? ceilingMbps)
    {
        Assert.NotNull(samples, "a ceiling is placed against the samples that were taken");

        if (ceilingMbps is not { } ceiling || ceiling <= 0)
        {
            return double.NaN;
        }

        var rates = Rates(samples);
        if (rates.Count < 2)
        {
            return double.NaN;
        }

        var peak = rates.Max();
        if (peak <= 0)
        {
            return double.NaN;
        }

        // The inverse of the mapping in Egress: a rate's share of the peak, taken off the floor.
        var fraction = 1 - (Headroom * ceiling / peak);
        return fraction is >= 0 and <= 1 ? fraction : double.NaN;
    }

    /// <summary>
    /// How much stream the curve covers, in seconds, and null where it covers none. The window
    /// is the samples the session still holds rather than a fixed span, so it grows with the run
    /// and then stops at the point the oldest samples start falling off the end.
    /// </summary>
    public static double? Span(IReadOnlyList<PublishStats> samples)
    {
        Assert.NotNull(samples, "a window is measured over the samples that were taken");

        double? first = null;
        double? last = null;
        foreach (var sample in samples)
        {
            if (!sample.HasTimeSec)
            {
                continue;
            }

            first ??= sample.TimeSec;
            last = sample.TimeSec;
        }

        return first is null || last is null || last <= first ? null : last - first;
    }

    /// <summary>
    /// The measured rates of a run, in order. A sample with no measurement for the rate
    /// contributes nothing, rather than a zero that would draw as a stream that stalled -
    /// presence is what says which it is.
    /// </summary>
    private static List<double> Rates(IReadOnlyList<PublishStats> samples)
    {
        var rates = new List<double>(samples.Count);
        foreach (var sample in samples)
        {
            if (sample.HasInstMbps)
            {
                rates.Add(sample.InstMbps);
            }
        }

        return rates;
    }
}
