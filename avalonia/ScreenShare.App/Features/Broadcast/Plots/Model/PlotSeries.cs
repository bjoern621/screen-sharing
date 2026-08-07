using Avalonia;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Broadcast.Plots.Model;

/// <summary>
/// The sparkline geometry: encoder samples mapped into the space the design was drawn in.
///
/// <see cref="Extent"/> is that space: X runs left to right across the window of samples held,
/// Y runs 0 at the top to 104 at the bottom. Keeping the curve in design coordinates is what
/// lets a wider card show the same shape stretched rather than a different window of it.
///
/// <b>Only one of the design's three series has a source.</b> The encoder reports its rate per
/// interval, so the egress curve is real. Round trip and buffer fill are measured by nothing in
/// the pipeline, so those two draw nothing and the card says why rather than showing a shape
/// that would read as a measurement.
///
/// The vertical scale is the samples' own peak rather than the encoder's ceiling, and that is
/// deliberate: a sparkline with no axis says "how has this moved", and scaling to a ceiling
/// the stream never approaches flattens the only thing it has to say. The ceiling is stated as
/// a label beside it instead.
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

        var rates = new List<double>(samples.Count);
        foreach (var sample in samples)
        {
            // A sample with no measurement for the rate contributes no point, rather than a
            // zero that would draw as a stream that stalled. Presence is what says which it is.
            if (sample.HasInstMbps)
            {
                rates.Add(sample.InstMbps);
            }
        }

        if (rates.Count < 2)
        {
            return [];
        }

        var peak = rates.Max();
        var points = new Point[rates.Count];
        for (var i = 0; i < rates.Count; i++)
        {
            var x = Extent.Width * i / (rates.Count - 1);

            // Inverted, because Y grows downward in the drawing space: a higher rate is a
            // smaller Y. A run whose peak is zero draws along the floor rather than dividing.
            var share = peak > 0 ? rates[i] / peak : 0;
            var y = Extent.Height - (Extent.Height * Headroom * share);

            points[i] = new Point(x, y);
        }

        return points;
    }
}
