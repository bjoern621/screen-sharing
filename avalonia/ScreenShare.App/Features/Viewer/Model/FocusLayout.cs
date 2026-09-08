using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Viewer.Model;

/// <summary>
/// Where the focused tile and the rail of the others go in a given box.
///
/// Pure as <see cref="TileLayout"/> is: sizes and aspect ratios in, rectangles out,
/// so the arrangement is asserted in tests without a window (<c>avalonia/README.md</c>).
///
/// The rail takes a share of the height off the top tile, one row at one height, each tile as wide as its own
/// shape makes it there.
/// A rail wider than the box keeps that height and is scrolled: shrinking to fit is what the grid arrangement
/// does, and a rail that shrank with every stream joining would end at thumbnails.
///
/// Scrolling is one offset in the box's own units, which the caller applies to the whole arrangement.
/// The focused tile is placed at the offset, so it stands still while the rail moves under it.
/// </summary>
public static class FocusLayout
{
    /// <summary>
    /// Share of the height the rail takes, held between <see cref="MinRail"/> and <see cref="MaxRail"/>.
    /// A fraction so the rail scales with the window, bounded so it neither disappears on a short one nor takes
    /// half of a tall one.
    /// </summary>
    public const double RailFraction = 0.18;

    public const double MinRail = 110;
    public const double MaxRail = 200;

    /// <summary>
    /// Height kept clear under the rail for its scrollbar.
    /// Reserved whether or not the rail scrolls: taken on overflow alone, it would shrink the tiles into fitting,
    /// which frees the strip, which overflows them again.
    /// </summary>
    public const double BarStrip = 12;

    /// <param name="Extent">
    /// Width the arrangement covers, the box's own where the rail fits inside it.
    /// A caller sizing itself by this gives the scroll viewer around it something to scroll.
    /// </param>
    public readonly record struct Arrangement(IReadOnlyList<TileLayout.Placement> Tiles, double Extent);

    /// <summary>
    /// Rectangles for one focused tile and the rail under it, in the caller's own order.
    /// </summary>
    /// <param name="aspects">Width over height per tile, in the caller's own order.</param>
    /// <param name="focused">Which of them takes the stage.</param>
    /// <param name="width">Usable width, the visible one where the rail runs past it.</param>
    /// <param name="height">Usable height, which the arrangement always fits inside.</param>
    /// <param name="gap">Space between two tiles, and between the stage and the rail.</param>
    /// <param name="offset">How far the rail has been scrolled, clamped here to what there is to scroll.</param>
    public static Arrangement Solve(
        IReadOnlyList<double> aspects, int focused, double width, double height, double gap, double offset)
    {
        Assert.NotNull(aspects, "an arrangement is of some tiles");
        Assert.That(
            focused >= 0 && focused < aspects.Count, "the focused tile is one of them", focused, aspects.Count);
        Assert.That(gap >= 0, "the space between tiles is not negative", gap);
        Assert.That(offset >= 0, "a scroll offset runs from the left edge", offset);

        if (width <= 0 || height <= 0)
        {
            return new Arrangement([], 0);
        }

        var rail = Enumerable.Range(0, aspects.Count).Where(index => index != focused).ToArray();
        var band = rail.Length == 0 ? 0 : Math.Clamp(height * RailFraction, MinRail, MaxRail);
        var row = Math.Max(0, band - BarStrip);
        var stage = Math.Max(0, height - band - (band > 0 ? gap : 0));

        var span = rail.Sum(index => row * TileLayout.Sane(aspects[index])) + (gap * Math.Max(0, rail.Length - 1));
        var extent = Math.Max(width, span);
        var scrolled = Math.Min(offset, extent - width);

        var placed = new TileLayout.Placement[aspects.Count];
        placed[focused] = Letterbox(TileLayout.Sane(aspects[focused]), focused, scrolled, 0, width, stage);

        // Centred while the rail fits, from the left edge once it does not:
        // slack split in two reads as a margin, and a scrolled rail has none to split.
        var x = span < width ? (width - span) / 2 : 0;
        var y = stage + gap;
        foreach (var index in rail)
        {
            var tile = row * TileLayout.Sane(aspects[index]);
            placed[index] = new TileLayout.Placement(index, x, y, tile, row);
            x += tile + gap;
        }

        Assert.That(placed.Length == aspects.Count, "a place for every tile", placed.Length, aspects.Count);
        Assert.That(extent >= width, "the arrangement covers the box it was given", extent, width);
        return new Arrangement(placed, extent);
    }

    /// <summary>
    /// Largest rectangle of the given shape that fits the box, centred in it.
    /// The focused tile keeps its stream's shape as every other tile does, being alone turning the leftover space
    /// into a margin.
    /// </summary>
    private static TileLayout.Placement Letterbox(
        double aspect, int index, double x, double y, double width, double height)
    {
        if (width <= 0 || height <= 0)
        {
            return new TileLayout.Placement(index, x, y, 0, 0);
        }

        var tileWidth = width;
        var tileHeight = tileWidth / aspect;
        if (tileHeight > height)
        {
            tileHeight = height;
            tileWidth = tileHeight * aspect;
        }

        return new TileLayout.Placement(
            index,
            x + ((width - tileWidth) / 2),
            y + ((height - tileHeight) / 2),
            tileWidth,
            tileHeight);
    }
}
