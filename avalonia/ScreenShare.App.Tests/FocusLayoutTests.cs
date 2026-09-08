using ScreenShare.App.Features.Viewer.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Failures a screenshot shows once a group is big enough:
/// rail tiles past the right edge with no way to reach them, a stage that slides away as the rail is scrolled,
/// a scrollbar over the tiles it scrolls.
/// Solver is pure, so none of it needs a window (<c>Features/Viewer/Model/FocusLayout.cs</c>).
/// </summary>
public sealed class FocusLayoutTests
{
    private const double Gap = 8;
    private const double Wide = 16.0 / 9;

    private static FocusLayout.Arrangement Solve(int count, double width, double height, double offset = 0)
        => FocusLayout.Solve([.. Enumerable.Repeat(Wide, count)], 0, width, height, Gap, offset);

    private static TileLayout.Placement Stage(FocusLayout.Arrangement arrangement)
        => arrangement.Tiles.Single(tile => tile.Index == 0);

    private static List<TileLayout.Placement> Rail(FocusLayout.Arrangement arrangement)
        => arrangement.Tiles.Where(tile => tile.Index != 0).OrderBy(tile => tile.X).ToList();

    /// <summary>Slack split in two reads as a margin, and a rail that fits has nothing to scroll.</summary>
    [Fact]
    public void ARailThatFitsIsCentredAndScrollsNothing()
    {
        var arrangement = Solve(3, 1200, 800);
        var rail = Rail(arrangement);

        Assert.Equal(1200, arrangement.Extent, 3);
        Assert.Equal(rail[0].X, 1200 - (rail[^1].X + rail[^1].Width), 3);
    }

    /// <summary>
    /// The one thing the reader cannot do without: a rail past the width reports how wide it is,
    /// so the scroll viewer around the panel has something to scroll.
    /// </summary>
    [Fact]
    public void ARailWiderThanTheBoxReportsItsOwnWidth()
    {
        var arrangement = Solve(12, 600, 800);
        var rail = Rail(arrangement);

        Assert.True(arrangement.Extent > 600, $"the rail outgrew the box, not {arrangement.Extent}");
        Assert.Equal(0, rail[0].X, 3);
        Assert.Equal(arrangement.Extent, rail[^1].X + rail[^1].Width, 3);
    }

    /// <summary>
    /// The panel is drawn shifted by the offset, so a stage placed at the offset is a stage that stays put
    /// while the rail under it moves.
    /// </summary>
    [Fact]
    public void TheStageRidesTheOffsetAndTheRailDoesNot()
    {
        var arrangement = Solve(12, 600, 800, 100);

        Assert.Equal(100, Stage(arrangement).X, 3);
        Assert.Equal(0, Rail(arrangement)[0].X, 3);
    }

    /// <summary>An offset past the end leaves the last tile against the right edge.</summary>
    [Fact]
    public void AnOffsetPastTheEndStopsAtIt()
    {
        var arrangement = Solve(12, 600, 800, 10_000);

        Assert.Equal(arrangement.Extent - 600, Stage(arrangement).X, 3);
    }

    /// <summary>The stage keeps its stream shape, the leftover space going to its margins.</summary>
    [Fact]
    public void TheStageKeepsItsStreamsShape()
    {
        var arrangement = FocusLayout.Solve([21.0 / 9, Wide, Wide], 0, 1200, 800, Gap, 0);
        var stage = Stage(arrangement);

        Assert.Equal(21.0 / 9, stage.Width / stage.Height, 3);
        Assert.True(stage.Width <= 1200.001, $"the stage fits the width, not {stage.Width}");
    }

    /// <summary>
    /// The strip under the rail is reserved whether or not the rail scrolls.
    /// Taking it only where the rail overflows would shrink the tiles into fitting, which frees the strip,
    /// which overflows them again.
    /// </summary>
    [Fact]
    public void TheRailLeavesRoomForItsScrollbar()
    {
        foreach (var count in (int[])[3, 12])
        {
            var bottom = Rail(Solve(count, 600, 800)).Max(tile => tile.Y + tile.Height);

            Assert.True(800 - bottom >= FocusLayout.BarStrip, $"{count} tiles left {800 - bottom} under the rail");
        }
    }

    /// <summary>With nothing beside it the stage takes the box, no rail meaning no strip to keep clear.</summary>
    [Fact]
    public void AloneOnTheStageATileTakesTheWholeBox()
    {
        var stage = Stage(Solve(1, 1200, 800));

        Assert.Equal(1200, stage.Width, 3);
        Assert.Equal(1200 / Wide, stage.Height, 3);
    }

    /// <summary>A box with no room places nothing, which is the first measure pass inside a scroll viewer.</summary>
    [Fact]
    public void ABoxWithNoRoomPlacesNothing()
    {
        var arrangement = Solve(4, 0, 800);

        Assert.Empty(arrangement.Tiles);
        Assert.Equal(0, arrangement.Extent, 3);
    }
}
