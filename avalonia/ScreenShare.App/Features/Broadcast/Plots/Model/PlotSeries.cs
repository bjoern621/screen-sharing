using Avalonia;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.Model;

namespace ScreenShare.App.Features.Broadcast.Plots.Model;

/// <summary>
/// Sparkline geometry: readings mapped into the space the design was drawn in.
///
/// <see cref="Extent"/> is that space, 480 by 104: X runs across <see cref="WindowSeconds"/>,
/// Y runs 0 at the top to 104 at the bottom.
/// Design coordinates let a wider card stretch the same window instead of covering a longer one.
///
/// Three series, three clocks.
/// Egress is placed by the encoder's own running time.
/// The two relay curves are placed by when this shell received the snapshot, the relay stamping none and its poll
/// interval not being on the contract.
///
/// Each latency point is the worst reader on the path at that snapshot, and worst by round trip and worst by loss
/// can be two different viewers.
///
/// Vertical scale is the drawn window's own peak, never the encoder's ceiling: scaling to a ceiling the stream
/// never approaches flattens the movement a sparkline exists to show.
/// <see cref="CeilingFraction"/> places the rule marking that ceiling against the same peak.
/// </summary>
public static class PlotSeries
{
    public static readonly Size Extent = new(480, 104);

    /// <summary>
    /// Span every plot covers, whatever the run's length.
    /// Newest reading at the right edge, one this old at the left, so a younger run fills the right of the card
    /// and leaves the rest empty rather than stretching over it.
    /// A stretched curve puts a given moment somewhere else on the card each second, on a plot whose one fixed
    /// thing is where a moment sits.
    /// A span and not a count of readings: both series carry a stamp, so no cadence has to be assumed to place
    /// a point.
    /// </summary>
    public const double WindowSeconds = 60;

    /// <summary>Share of the height the curve gets, 0..1. The rest keeps a peak off the top edge.</summary>
    private const double Headroom = 0.85;

    /// <summary>One measurement, stamped in seconds on whichever clock its series is kept on.</summary>
    private readonly record struct Reading(double At, double Value);

    /// <summary>
    /// Egress curve for one run of samples, empty where there is no shape to draw.
    /// A single sample draws nothing: one point is a reading, and a line through it would read as a flat stream.
    /// </summary>
    public static IReadOnlyList<Point> Egress(IReadOnlyList<PublishStats> samples)
    {
        Assert.NotNull(samples, "a series is drawn from the samples that were taken");

        return Curve(Rates(samples), Now(samples));
    }

    /// <summary>Both curves over one run of relay snapshots, from the worst reader on the path at each.</summary>
    /// <param name="Rtt">Round trip in ms, scaled to its own peak.</param>
    /// <param name="Loss">Send-side loss in percent, scaled to its own peak.</param>
    public sealed record LatencySeries(IReadOnlyList<Point> Rtt, IReadOnlyList<Point> Loss);

    /// <summary>
    /// Both latency curves for one stream, empty where there is no shape to draw.
    /// Produced together, so a snapshot contributes a point to both curves or to neither and an X on one
    /// is the same moment as an X on the other.
    /// An unreachable relay, a snapshot older than this stream's path, and one whose readers are all on legs
    /// nobody times contribute nothing: a zero would draw as a viewer with a perfect link rather than as one
    /// nobody measured.
    /// SRT is the leg reporting either figure, so one filter serves both curves.
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

        // Axis ends at the newest snapshot, not at the newest one that timed somebody, so readings older than
        // the window leave the plot instead of describing viewers who have gone.
        var now = samples.Count == 0 ? (double?)null : Seconds(samples[^1].At);
        var series = new LatencySeries(Curve(rtts, now), Curve(losses, now));

        Assert.That(series.Rtt.Count == series.Loss.Count,
            "the two latency curves cover the same window", series.Rtt.Count, series.Loss.Count);
        return series;
    }

    /// <summary>
    /// Where the encoder's ceiling falls on the egress curve, 0 at the top to 1 at the bottom, and
    /// <see cref="double.NaN"/> where it falls off the plot.
    /// Placed against the same peak the curve is scaled to, a rule at a fixed height marking the ceiling only
    /// by coincidence.
    /// A stream running well under its ceiling gets no rule, and the label states the figure instead.
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

        // Inverse of Curve's mapping: a rate's share of the peak, measured off the floor.
        var fraction = 1 - (Headroom * ceiling / peak);
        return fraction is >= 0 and <= 1 ? fraction : double.NaN;
    }

    /// <summary>
    /// Window's readings in drawing space: placed horizontally by when they were taken, scaled vertically to their
    /// own peak.
    /// Every curve on the card goes through here, so a vertical read off one is "against this window's own
    /// highest" on the next, and a horizontal read is always "how long ago".
    /// Fewer than two readings draw nothing.
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
            // Right edge is the anchor, left edge one window before it, so spacing is elapsed time and not a share
            // of however many readings there are.
            var x = Extent.Width * (1 - ((anchor - drawn[i].At) / WindowSeconds));

            // Y grows downward in the drawing space, so a higher value is a smaller Y.
            // A window whose peak is zero draws along the floor instead of dividing, the right shape for a loss
            // curve that stayed at nothing.
            var share = peak > 0 ? drawn[i].Value / peak : 0;
            var y = Extent.Height - (Extent.Height * Headroom * share);

            points[i] = new Point(x, y);
        }

        Assert.That(points[0].X >= 0 && points[^1].X <= Extent.Width,
            "a drawn curve stays inside the window", points[0].X, points[^1].X);
        return points;
    }

    /// <summary>
    /// Readings the window covers, oldest first: none older than <see cref="WindowSeconds"/> before
    /// <paramref name="now"/>, none after it, and all on the same clock as <paramref name="now"/>.
    /// Empty where the series ended before the window began, and where it belongs to a run the anchor does not.
    ///
    /// Walks back from the newest rather than filtering the series, to cut at the one discontinuity a series
    /// carries.
    /// A relaunched pipeline leaves the stream live, so its samples are appended to the previous pipeline's and
    /// its running clock starts again at zero.
    /// The walk stops where time goes backwards, so the plot draws the run the newest reading belongs to instead
    /// of two runs laid over each other.
    ///
    /// A newest reading past the anchor is that same relaunch seen from the other side: the anchor is on the new
    /// run's clock and every reading on the old one's,
    /// the run that just started having been measured a time and no rate.
    /// Those readings draw nothing until the new run has a shape of its own, rather than being placed against
    /// a clock that never counted them.
    /// </summary>
    private static List<Reading> Windowed(IReadOnlyList<Reading> readings, double? now)
    {
        if (readings.Count == 0 || now is not { } anchor
            || anchor - readings[^1].At is < 0 or > WindowSeconds)
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

        // Ends only: the walk keeps the range ascending, so the oldest and the newest bound the rest.
        Assert.That(anchor - drawn[0].At <= WindowSeconds && drawn[^1].At <= anchor,
            "a windowed reading was taken inside the window it is drawn in", anchor, drawn[0].At, drawn[^1].At);
        return drawn;
    }

    /// <summary>
    /// End of the egress axis: running time of the newest sample stating one, null where none does.
    /// The newest sample rather than the newest carrying a rate, so an encoder reporting without a rate slides
    /// its curve off the plot instead of freezing it at its last shape.
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

    /// <summary>One instant on the shell's clock as seconds, the unit both axes are placed in.</summary>
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
    /// Measured rates of a run in Mb/s, in order, each at the moment the encoder took it.
    /// A sample carrying no rate contributes nothing rather than a zero, which would draw as a stall.
    /// Presence tells the two apart.
    /// A sample carrying no time contributes nothing either, a rate with no moment being unplaceable on the axis.
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
