using ScreenShare.App.Features.Viewer.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The grid's arrangement.
///
/// What these lock out is a grid that quietly stops honouring its own rules: rows that do not
/// fill the width, tiles that are not their stream's shape, an arrangement that squashes tiles
/// below legibility rather than scrolling, and an order that depends on which tile happened to
/// report its aspect first. Every one of those is invisible in a screenshot of a lucky case and
/// obvious in a screenshot of an unlucky one, which is why they are asserted here rather than
/// looked at.
///
/// The solver is pure, so none of this needs a window (<c>Features/Viewer/Model/TileLayout.cs</c>).
/// </summary>
public sealed class TileLayoutTests
{
    private const double Gap = 8;

    /// <summary>The tiles of one row of an arrangement, left to right.</summary>
    private static List<TileLayout.Placement> Row(TileLayout.Arrangement arrangement, double y)
        => arrangement.Tiles.Where(t => Math.Abs(t.Y - y) < 0.001).OrderBy(t => t.X).ToList();

    /// <summary>Every distinct row top of an arrangement, top to bottom.</summary>
    private static List<double> Rows(TileLayout.Arrangement arrangement)
        => arrangement.Tiles.Select(t => t.Y).Distinct().Order().ToList();

    /// <summary>
    /// A row is centred in the width. Rows come out as wide as their contents make them, so most
    /// are narrower than the box; what is not allowed is slack piled on one side, which reads as
    /// broken alignment rather than as a margin.
    /// </summary>
    [Fact]
    public void EveryRowIsCentredInTheWidth()
    {
        var arrangement = TileLayout.Solve([16.0 / 9, 16.0 / 9, 4.0 / 3, 21.0 / 9, 1.0], 1200, 800, Gap);

        foreach (var y in Rows(arrangement))
        {
            var row = Row(arrangement, y);
            var left = row[0].X;
            var right = 1200 - (row[^1].X + row[^1].Width);

            Assert.Equal(left, right, 3);
            Assert.True(left >= -0.001, $"a row starts inside the box, not at {left}");
        }
    }

    /// <summary>
    /// Every tile is the same height, whichever row it is in.
    ///
    /// This is the rule the arrangement is built around. Letting each row fill the width would
    /// give a row of one tile about twice the height of a row of two, which draws as one big tile
    /// beside some small ones rather than as a grid of equals.
    /// </summary>
    [Theory]
    [InlineData(3)]
    [InlineData(5)]
    [InlineData(7)]
    public void EveryTileIsTheSameHeight(int count)
    {
        var arrangement = TileLayout.Solve(Enumerable.Repeat(16.0 / 9, count).ToArray(), 1280, 800, Gap);

        Assert.All(arrangement.Tiles, tile => Assert.Equal(arrangement.Tiles[0].Height, tile.Height, 3));
    }

    /// <summary>
    /// Three tiles of one shape come out the same size as each other, whichever rows they land
    /// in. It is the case the equal-height rule was written for, so it is asserted on its own.
    ///
    /// The row count is deliberately not asserted. Two rows of two and one draws each tile larger
    /// than one row of three in a box this shape, and picking the arrangement that draws the most
    /// picture is the solver's job - what matters here is that the three come out equal either
    /// way.
    /// </summary>
    [Fact]
    public void ThreeEqualTilesComeOutEqual()
    {
        var arrangement = TileLayout.Solve([16.0 / 9, 16.0 / 9, 16.0 / 9], 1280, 620, Gap);

        Assert.All(arrangement.Tiles, tile => Assert.Equal(arrangement.Tiles[0].Width, tile.Width, 3));
        Assert.All(arrangement.Tiles, tile => Assert.Equal(arrangement.Tiles[0].Height, tile.Height, 3));
    }

    /// <summary>
    /// A tile is drawn at its own aspect ratio, always. That is what makes cell widths follow the
    /// stream and row height follow the row, and it is the property that says nothing is ever
    /// stretched or cropped to make an arrangement come out even.
    /// </summary>
    [Fact]
    public void ATileKeepsItsOwnShape()
    {
        double[] aspects = [16.0 / 9, 4.0 / 3, 1.0, 21.0 / 9];
        var arrangement = TileLayout.Solve(aspects, 1200, 800, Gap);

        Assert.All(arrangement.Tiles, tile => Assert.Equal(aspects[tile.Index], tile.Width / tile.Height, 2));
    }

    /// <summary>
    /// Tiles in one row share one height, and that is the property the whole arrangement is
    /// built on.
    /// </summary>
    [Fact]
    public void ARowIsOneHeight()
    {
        var arrangement = TileLayout.Solve([16.0 / 9, 4.0 / 3, 21.0 / 9, 16.0 / 9], 1400, 900, Gap);

        foreach (var y in Rows(arrangement))
        {
            var row = Row(arrangement, y);
            Assert.All(row, tile => Assert.Equal(row[0].Height, tile.Height, 3));
        }
    }

    /// <summary>
    /// An arrangement that fits stays inside the box and does not report that it scrolls. This
    /// is the ordinary case and the one a reader is in nearly always.
    /// </summary>
    [Fact]
    public void AFittingArrangementDoesNotScroll()
    {
        var arrangement = TileLayout.Solve([16.0 / 9, 16.0 / 9, 16.0 / 9, 16.0 / 9], 1600, 900, Gap);

        Assert.False(arrangement.Scrolls);
        Assert.True(arrangement.Height <= 900);
    }

    /// <summary>
    /// The box is filled rather than merely fitted: the arrangement uses most of the height it
    /// was given. A solver that always chose one row would pass every test above and leave two
    /// thirds of the window empty, which is the failure this one exists for.
    /// </summary>
    [Fact]
    public void AFittingArrangementFillsTheBox()
    {
        var arrangement = TileLayout.Solve([16.0 / 9, 16.0 / 9, 16.0 / 9, 16.0 / 9], 1600, 900, Gap);

        Assert.True(arrangement.Height > 900 * 0.8, $"{arrangement.Height} of 900 used");
    }

    /// <summary>
    /// Below legibility the grid scrolls instead of shrinking. Twenty tiles in a short box
    /// cannot all be legible, and the answer is a taller arrangement rather than a squashed one.
    /// </summary>
    [Fact]
    public void TooManyTilesScrollRatherThanShrink()
    {
        var aspects = Enumerable.Repeat(16.0 / 9, 20).ToArray();
        var arrangement = TileLayout.Solve(aspects, 900, 400, Gap);

        Assert.True(arrangement.Scrolls);
        Assert.All(arrangement.Tiles, tile => Assert.True(tile.Height >= TileLayout.MinRowHeight));
        Assert.True(arrangement.Height > 400);
    }

    /// <summary>
    /// A single tile takes the whole box at its own shape rather than being stretched to it.
    /// </summary>
    [Fact]
    public void OneTileIsNotStretched()
    {
        var arrangement = TileLayout.Solve([16.0 / 9], 1600, 900, Gap);

        var tile = Assert.Single(arrangement.Tiles);
        Assert.Equal(16.0 / 9, tile.Width / tile.Height, 2);
        Assert.True(tile.Height <= 900);
    }

    /// <summary>
    /// A tile whose stream has not said what shape it is gets the assumed one, and an impossible
    /// aspect - a pipeline reporting a zero dimension - lands on the same answer rather than
    /// dividing the arrangement by zero.
    /// </summary>
    [Theory]
    [InlineData(0)]
    [InlineData(-1)]
    [InlineData(double.NaN)]
    public void AnUnknownShapeIsTheAssumedOne(double aspect)
    {
        var arrangement = TileLayout.Solve([aspect], 1600, 900, Gap);

        var tile = Assert.Single(arrangement.Tiles);
        Assert.Equal(TileLayout.UnknownAspect, tile.Width / tile.Height, 2);
    }

    /// <summary>
    /// Results come back in the caller's order however the search sorted them, so no caller has
    /// to undo the sort and no tile can be handed another tile's rectangle.
    /// </summary>
    [Fact]
    public void PlacementsComeBackInTheCallersOrder()
    {
        double[] aspects = [1.0, 21.0 / 9, 4.0 / 3, 16.0 / 9];
        var arrangement = TileLayout.Solve(aspects, 1200, 800, Gap);

        Assert.Equal([0, 1, 2, 3], arrangement.Tiles.Select(t => t.Index));
    }

    /// <summary>
    /// The same tiles arrange the same way twice. The search sorts by aspect and ties keep their
    /// input order, so an arrangement cannot depend on which equal-shaped tile was reached first.
    /// </summary>
    [Fact]
    public void TheSameTilesArrangeTheSameWay()
    {
        double[] aspects = [16.0 / 9, 16.0 / 9, 16.0 / 9, 4.0 / 3, 4.0 / 3];

        var first = TileLayout.Solve(aspects, 1300, 850, Gap);
        var second = TileLayout.Solve(aspects, 1300, 850, Gap);

        Assert.Equal(first.Tiles, second.Tiles);
    }

    /// <summary>
    /// Nothing to arrange is an empty arrangement rather than a division by a count of zero, and
    /// a box with no room in it is the same answer for the same reason: a window that has not
    /// been measured yet arranges nothing rather than crashing on its first pass.
    /// </summary>
    [Theory]
    [InlineData(0, 0)]
    [InlineData(1200, 0)]
    [InlineData(0, 800)]
    public void AnUnmeasuredBoxArrangesNothing(double width, double height)
    {
        var arrangement = TileLayout.Solve([16.0 / 9, 4.0 / 3], width, height, Gap);

        Assert.Empty(arrangement.Tiles);
        Assert.Equal(0, arrangement.Height);
    }

    /// <summary>No tiles is no arrangement, whatever the box.</summary>
    [Fact]
    public void NoTilesIsNoArrangement()
    {
        var arrangement = TileLayout.Solve([], 1600, 900, Gap);

        Assert.Empty(arrangement.Tiles);
        Assert.False(arrangement.Scrolls);
    }

    /// <summary>
    /// Tiles never overlap. Every other property here could hold while two rectangles sat on top
    /// of each other, and a grid whose tiles overlap is the one failure a reader cannot miss.
    /// </summary>
    [Fact]
    public void TilesDoNotOverlap()
    {
        double[] aspects = [16.0 / 9, 4.0 / 3, 21.0 / 9, 1.0, 16.0 / 9, 3.0 / 4];
        var arrangement = TileLayout.Solve(aspects, 1300, 900, Gap);

        var tiles = arrangement.Tiles;
        for (var a = 0; a < tiles.Count; a++)
        {
            for (var b = a + 1; b < tiles.Count; b++)
            {
                var overlaps =
                    tiles[a].X < tiles[b].X + tiles[b].Width - 0.001 &&
                    tiles[b].X < tiles[a].X + tiles[a].Width - 0.001 &&
                    tiles[a].Y < tiles[b].Y + tiles[b].Height - 0.001 &&
                    tiles[b].Y < tiles[a].Y + tiles[a].Height - 0.001;

                Assert.False(overlaps, $"tiles {a} and {b} overlap");
            }
        }
    }
}
