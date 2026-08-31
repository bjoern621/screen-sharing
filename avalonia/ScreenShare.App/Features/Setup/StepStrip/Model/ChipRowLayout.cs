using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Setup.StepStrip.Model;

/// <summary>
/// Where each chip of a row begins and ends.
///
/// Boundaries accumulate along the row and each lands on a whole device pixel,
/// so arranging a chip between two of them rounds nothing outward and the last chip ends inside the panel.
/// One share rounded and multiplied instead ends the row up to a pixel per chip past that edge,
/// and a panel clipping to its bounds takes the overhang out of the last chip's border,
/// rather than out of the gap behind it.
/// The price is two chips differing by one device pixel.
/// </summary>
public static class ChipRowLayout
{
    /// <summary>Boundaries from the row's start to its end, one more than there are chips.</summary>
    public static IReadOnlyList<double> Edges(double width, int count, double scale)
    {
        Assert.That(width >= 0, "a row is as wide as the space it was handed", width);
        Assert.That(count >= 0, "a row holds zero chips or more", count);
        Assert.That(scale > 0, "a row is laid out at a scale factor", scale);

        var edges = new double[count + 1];
        for (var i = 1; i <= count; i++)
        {
            edges[i] = Math.Min(width, Math.Floor(width * i / count * scale) / scale);
        }

        Assert.That(edges[count] <= width, "the row ends inside the space it was handed", edges[count], width);
        return edges;
    }
}
