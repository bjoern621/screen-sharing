using ScreenShare.App.Features.Viewer.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The failures a screenshot shows only when the case is unlucky: rows off centre, a tile that is not its
/// stream's shape, tiles squashed below legibility, an order that follows whichever aspect arrived first.
/// The solver is pure, so none of it needs a window (<c>Features/Viewer/Model/TileLayout.cs</c>).
/// </summary>
public sealed class TileLayoutTests
{
    private const double Gap = 8;

    private static List<TileLayout.Placement> Row(TileLayout.Arrangement arrangement, double y)
        => arrangement.Tiles.Where(t => Math.Abs(t.Y - y) < 0.001).OrderBy(t => t.X).ToList();

    private static List<double> Rows(TileLayout.Arrangement arrangement)
        => arrangement.Tiles.Select(t => t.Y).Distinct().Order().ToList();

    /// <summary>
    /// A row is as wide as its contents, so the leftover is split rather than piled on one side, which
    /// reads as broken alignment.
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
    /// Three tiles in this box come out two over one and are bounded by the width, so the leftover height is
    /// a margin above and below rather than an empty half-window under the tiles.
    /// </summary>
    [Fact]
    public void TheStackIsCentredInTheHeight()
    {
        var arrangement = TileLayout.Solve([16.0 / 9, 16.0 / 9, 16.0 / 9], 1200, 1000, Gap);

        var above = Rows(arrangement)[0];
        var below = 1000 - arrangement.Tiles.Max(t => t.Y + t.Height);

        Assert.False(arrangement.Scrolls);
        Assert.True(above > 1, $"the rows are short of the box, not {above} from its top");
        Assert.Equal(above, below, 3);
    }

    /// <summary>Centring is a margin over the leftover, never a gap the tiles are pushed down by.</summary>
    [Fact]
    public void AnArrangementThatFillsTheHeightStartsAtTheTop()
    {
        var arrangement = TileLayout.Solve([16.0 / 9, 16.0 / 9, 16.0 / 9, 16.0 / 9], 1600, 900, Gap);

        Assert.Equal(0, Rows(arrangement)[0], 3);
    }

    /// <summary>
    /// The rule the arrangement is built around.
    /// Letting each row fill the width instead would draw a row of one about twice the height of a row of
    /// two: one big tile beside small ones rather than a grid of equals.
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
    /// The row count is deliberately not asserted: picking the arrangement that draws the most picture is
    /// the solver's job, and the three come out equal either way.
    /// </summary>
    [Fact]
    public void ThreeEqualTilesComeOutEqual()
    {
        var arrangement = TileLayout.Solve([16.0 / 9, 16.0 / 9, 16.0 / 9], 1280, 620, Gap);

        Assert.All(arrangement.Tiles, tile => Assert.Equal(arrangement.Tiles[0].Width, tile.Width, 3));
        Assert.All(arrangement.Tiles, tile => Assert.Equal(arrangement.Tiles[0].Height, tile.Height, 3));
    }

    /// <summary>Nothing is stretched or cropped to make an arrangement come out even.</summary>
    [Fact]
    public void ATileKeepsItsOwnShape()
    {
        double[] aspects = [16.0 / 9, 4.0 / 3, 1.0, 21.0 / 9];
        var arrangement = TileLayout.Solve(aspects, 1200, 800, Gap);

        Assert.All(arrangement.Tiles, tile => Assert.Equal(aspects[tile.Index], tile.Width / tile.Height, 2));
    }

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

    [Fact]
    public void AFittingArrangementDoesNotScroll()
    {
        var arrangement = TileLayout.Solve([16.0 / 9, 16.0 / 9, 16.0 / 9, 16.0 / 9], 1600, 900, Gap);

        Assert.False(arrangement.Scrolls);
        Assert.True(arrangement.Height <= 900);
    }

    /// <summary>
    /// A solver that always chose one row passes every other property here and leaves most of the window
    /// empty.
    /// </summary>
    [Fact]
    public void AFittingArrangementFillsTheBox()
    {
        var arrangement = TileLayout.Solve([16.0 / 9, 16.0 / 9, 16.0 / 9, 16.0 / 9], 1600, 900, Gap);

        Assert.True(arrangement.Height > 900 * 0.8, $"{arrangement.Height} of 900 used");
    }

    /// <summary><c>MinRowHeight</c> is the floor, so a box too short gets a taller arrangement.</summary>
    [Fact]
    public void TooManyTilesScrollRatherThanShrink()
    {
        var aspects = Enumerable.Repeat(16.0 / 9, 20).ToArray();
        var arrangement = TileLayout.Solve(aspects, 900, 400, Gap);

        Assert.True(arrangement.Scrolls);
        Assert.All(arrangement.Tiles, tile => Assert.True(tile.Height >= TileLayout.MinRowHeight));
        Assert.True(arrangement.Height > 400);
    }

    [Fact]
    public void OneTileIsNotStretched()
    {
        var arrangement = TileLayout.Solve([16.0 / 9], 1600, 900, Gap);

        var tile = Assert.Single(arrangement.Tiles);
        Assert.Equal(16.0 / 9, tile.Width / tile.Height, 2);
        Assert.True(tile.Height <= 900);
    }

    /// <summary>
    /// An impossible aspect, a pipeline reporting a zero dimension among them, lands on the assumed one
    /// rather than dividing the arrangement by zero.
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
    /// The search sorts by aspect internally, so results carry the caller's order and no tile takes
    /// another's rectangle.
    /// </summary>
    [Fact]
    public void PlacementsComeBackInTheCallersOrder()
    {
        double[] aspects = [1.0, 21.0 / 9, 4.0 / 3, 16.0 / 9];
        var arrangement = TileLayout.Solve(aspects, 1200, 800, Gap);

        Assert.Equal([0, 1, 2, 3], arrangement.Tiles.Select(t => t.Index));
    }

    /// <summary>Ties keep input order, so no arrangement depends on which equal tile came first.</summary>
    [Fact]
    public void TheSameTilesArrangeTheSameWay()
    {
        double[] aspects = [16.0 / 9, 16.0 / 9, 16.0 / 9, 4.0 / 3, 4.0 / 3];

        var first = TileLayout.Solve(aspects, 1300, 850, Gap);
        var second = TileLayout.Solve(aspects, 1300, 850, Gap);

        Assert.Equal(first.Tiles, second.Tiles);
    }

    /// <summary>A window arrives unmeasured, so an empty box arranges nothing rather than crashing.</summary>
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

    [Fact]
    public void NoTilesIsNoArrangement()
    {
        var arrangement = TileLayout.Solve([], 1600, 900, Gap);

        Assert.Empty(arrangement.Tiles);
        Assert.False(arrangement.Scrolls);
    }

    /// <summary>
    /// Every other property here holds while two rectangles sit on top of each other, and overlap is the one
    /// failure a reader cannot miss.
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
