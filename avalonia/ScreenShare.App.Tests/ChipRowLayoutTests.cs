using ScreenShare.App.Features.Setup.StepStrip.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The row tiles the space it is handed and stops there.
/// Pins the defect a row of rounded shares carries: one share rounded to a device pixel and multiplied by the chip
/// count ends the row past the panel, the panel clips to its bounds, and the last chip loses its right border while
/// keeping the corner arcs that curve inside the cut.
/// </summary>
public sealed class ChipRowLayoutTests
{
    private static IEnumerable<(double Width, int Count, double Scale)> Rows()
    {
        foreach (var scale in new[] { 1.0, 1.25, 1.5, 2.0, 3.0 })
        {
            foreach (var count in new[] { 1, 2, 3, 5, 7, 9 })
            {
                for (var width = 800.0; width <= 840; width += 0.25)
                {
                    yield return (width, count, scale);
                }
            }
        }
    }

    [Fact]
    public void TheRowEndsWhereTheSpaceDoes()
    {
        foreach (var (width, count, scale) in Rows())
        {
            var edges = ChipRowLayout.Edges(width, count, scale);

            Assert.Equal(count + 1, edges.Count);
            Assert.Equal(0, edges[0]);
            Assert.True(edges[count] <= width, $"{edges[count]} runs past {width} at {scale}x");

            // The leftover is under one device pixel, so the row fills its space rather than stopping short of it.
            Assert.True(width - edges[count] < 1 / scale, $"{edges[count]} stops short of {width} at {scale}x");
        }
    }

    [Fact]
    public void EveryChipSitsOnWholeDevicePixels()
    {
        foreach (var (width, count, scale) in Rows())
        {
            foreach (var edge in ChipRowLayout.Edges(width, count, scale))
            {
                var pixels = edge * scale;
                Assert.True(
                    Math.Abs(pixels - Math.Round(pixels)) < 1e-9,
                    $"{edge} falls between device pixels at {scale}x");
            }
        }
    }

    [Fact]
    public void ChipsDifferByAtMostOneDevicePixel()
    {
        foreach (var (width, count, scale) in Rows())
        {
            var edges = ChipRowLayout.Edges(width, count, scale);
            var widths = Enumerable.Range(0, count).Select(i => edges[i + 1] - edges[i]).ToList();

            Assert.All(widths, chip => Assert.True(chip >= 0, $"{chip} runs backwards"));
            Assert.True(
                widths.Max() - widths.Min() <= (1 / scale) + 1e-9,
                $"two of {count} chips differ by more than a pixel at {scale}x");
        }
    }
}
