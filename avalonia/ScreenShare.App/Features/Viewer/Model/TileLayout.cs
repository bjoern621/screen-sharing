using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Viewer.Model;

/// <summary>
/// How many tiles of given shapes are arranged in a given box.
///
/// <b>Pure, and that is the point.</b> It takes sizes and aspect ratios and answers rectangles;
/// it holds no controls, no view models and no state, so the arrangement can be tested without
/// a window and the panel that draws it contains no arithmetic of its own
/// (<c>avalonia/README.md</c>).
///
/// The arrangement is rows of tiles. Every tile is drawn at the one height the whole arrangement
/// uses, and is as wide as its own aspect ratio makes it at that height, so nothing is ever
/// cropped or stretched: a 21:9 stream beside a 4:3 one is simply wider.
///
/// <b>One height for every row, and it is the constraint rather than a result.</b> Letting each
/// row fill the width exactly would give each its own height, and a row holding one tile would
/// then come out about twice the height of a row holding two - which reads as one big tile beside
/// some small ones rather than as a grid. So the height is chosen once: the largest that lets
/// every row fit the width and the whole stack fit the box. Rows are narrower than the box by
/// however much their contents leave over, and are centred in it.
///
/// <b>The stack is centred in the height for the same reason rows are centred in the width.</b>
/// A row count that fits the width leaves the rows shorter than the box, and that leftover is a
/// margin rather than an empty half-window under the tiles.
/// </summary>
public static class TileLayout
{
    /// <summary>
    /// The aspect ratio a tile is laid out at before its stream has said what shape it is.
    ///
    /// A stream's real shape is known when the frame channel announces its first pool, and not
    /// before. Sixteen by nine is the assumption until then rather than a square or a collapse
    /// to nothing, because it is what nearly every capture and every test pattern here is, and
    /// because a tile that starts at the shape it will settle at does not visibly jump when it
    /// settles there.
    /// </summary>
    public const double UnknownAspect = 16.0 / 9.0;

    /// <summary>
    /// The shortest a row is drawn before the grid gives up and scrolls instead.
    ///
    /// Below this a tile is not small, it is unreadable: the name, the figures and the meter
    /// stop fitting and the picture stops being a picture. Scrolling past legible tiles is a
    /// better answer than fitting illegible ones, which is the one place this layout refuses to
    /// honour the box it was given.
    /// </summary>
    public const double MinRowHeight = 160;

    /// <summary>
    /// One tile's place in the arrangement.
    /// </summary>
    /// <param name="Index">Which of the tiles handed in this rectangle belongs to. Results come
    /// back in the caller's own order however the search sorted them, so no caller has to undo
    /// the sort.</param>
    public readonly record struct Placement(int Index, double X, double Y, double Width, double Height);

    /// <summary>The whole arrangement: where every tile goes, how tall it is, and whether it outgrew the box.</summary>
    /// <param name="Height">How far down the box the arrangement reaches: the margin above the
    /// rows plus the rows themselves. A caller that sizes itself by this leaves the same margin
    /// under the tiles as the arrangement put over them.</param>
    /// <param name="Scrolls">Whether the tiles could not be fitted at a legible size, so the
    /// container has to scroll. A caller that cannot scroll gets a taller arrangement than it
    /// asked for rather than a silently squashed one.</param>
    public readonly record struct Arrangement(IReadOnlyList<Placement> Tiles, double Height, bool Scrolls);

    /// <summary>
    /// Arranges tiles of the given aspect ratios in a box.
    ///
    /// <b>The search.</b> An arrangement is decided by one number - how many rows it has - and
    /// the tiles are spread across those rows as evenly as their count allows. Every row count
    /// from one to the number of tiles is tried, each is scored by the picture area it actually
    /// draws, and the best is taken. That is a handful of arithmetic per candidate and it is
    /// exhaustive over the thing that matters, rather than a heuristic that stops early.
    ///
    /// Even spreading rather than a search over every possible break, because the tiles are
    /// sorted by shape first: neighbouring tiles in the sorted order have similar aspect ratios,
    /// so an uneven break buys almost nothing and costs a combinatorial search.
    ///
    /// <b>The order.</b> Tiles are sorted by aspect ratio, widest first, so that rows are made of
    /// similar shapes and their heights come out even. Ties keep their input order, so the result
    /// is deterministic; a tile whose aspect resolves from the assumed one does move, which is
    /// the price of packing tightly and was chosen over packing loosely.
    ///
    /// <b>When nothing fits.</b> If no row count can draw every row at a legible height, the box
    /// stops being honoured: rows are laid out at the legible minimum and the result reports that
    /// it scrolls.
    /// </summary>
    /// <param name="aspects">Width over height for each tile, in the caller's own order.</param>
    /// <param name="width">The usable width. A non-positive width arranges nothing.</param>
    /// <param name="height">The usable height, which the arrangement fits inside where it can.</param>
    /// <param name="gap">The space between two tiles, and between two rows.</param>
    public static Arrangement Solve(IReadOnlyList<double> aspects, double width, double height, double gap)
    {
        Assert.NotNull(aspects, "an arrangement is of some tiles");
        Assert.That(gap >= 0, "the space between tiles is not negative", gap);

        if (aspects.Count == 0 || width <= 0 || height <= 0)
        {
            return new Arrangement([], 0, false);
        }

        // Sorted here and mapped back at the end, so every caller sees its own order and none of
        // them has to know the arrangement sorted anything.
        var order = Enumerable.Range(0, aspects.Count)
            .OrderByDescending(i => Sane(aspects[i]))
            .ThenBy(i => i)
            .ToArray();
        var sorted = order.Select(i => Sane(aspects[i])).ToArray();

        var best = Best(sorted, width, height, gap);
        var tiles = Place(sorted, order, best, width, height, gap, out var total);

        Assert.That(tiles.Count == aspects.Count, "a place for every tile", tiles.Count, aspects.Count);
        return new Arrangement(tiles, total, best.Scrolls);
    }

    /// <summary>One candidate arrangement: how many rows, how tall every one of them is, and whether it fits.</summary>
    private readonly record struct Candidate(int Rows, double Height, bool Scrolls);

    /// <summary>
    /// The row count that draws the most picture.
    ///
    /// Each candidate is scored by the area its tiles actually cover. That is the figure a reader
    /// sees, and it is what makes the choice between "fewer, larger tiles" and "more, smaller
    /// ones" an answer rather than a preference: one row of four wide tiles covers less of a tall
    /// window than two rows of two.
    ///
    /// Since every tile is the one height, the area is that height squared times the sum of the
    /// aspect ratios - and the sum does not depend on the row count. So the best candidate is
    /// simply the one whose height comes out largest, and the comparison is over one number.
    ///
    /// A candidate whose height is below the legible minimum is not scored at all, unless none is
    /// legible - and then the tiles are drawn at the minimum and it scrolls.
    /// </summary>
    private static Candidate Best(double[] aspects, double width, double height, double gap)
    {
        var n = aspects.Length;
        var bestRows = 0;
        var bestHeight = 0.0;

        for (var rows = 1; rows <= n; rows++)
        {
            var row = Height(aspects, rows, width, height, gap);
            if (row < MinRowHeight || row <= bestHeight)
            {
                continue;
            }

            bestRows = rows;
            bestHeight = row;
        }

        if (bestRows > 0)
        {
            return new Candidate(bestRows, bestHeight, false);
        }

        // Nothing was legible in the box. The tiles are drawn at the legible minimum, in the most
        // rows that still fit the width there, and the reader scrolls.
        var floor = 1;
        for (var rows = 1; rows <= n; rows++)
        {
            if (Width(aspects, rows, MinRowHeight, gap) <= width)
            {
                floor = rows;
                break;
            }
        }

        return new Candidate(floor, MinRowHeight, true);
    }

    /// <summary>
    /// The one height every tile is drawn at, for a given row count, and zero where there is none.
    ///
    /// Two things bound it and the smaller wins. Every row has to fit the width, so the height is
    /// at most the one that makes the widest row exactly fill it; and the whole stack has to fit
    /// the box, so it is at most the box's height shared between the rows. Taking the minimum is
    /// the whole calculation.
    /// </summary>
    private static double Height(double[] aspects, int rows, double width, double height, double gap)
    {
        var byWidth = double.PositiveInfinity;
        for (var r = 0; r < rows; r++)
        {
            var (from, to) = Span(aspects.Length, rows, r);
            byWidth = Math.Min(byWidth, RowHeight(aspects, from, to, width, gap));
        }

        var byHeight = (height - (gap * (rows - 1))) / rows;
        var row = Math.Min(byWidth, byHeight);
        return row > 0 ? row : 0;
    }

    /// <summary>How wide the widest row of a candidate is, at a given height.</summary>
    private static double Width(double[] aspects, int rows, double height, double gap)
    {
        var widest = 0.0;
        for (var r = 0; r < rows; r++)
        {
            var (from, to) = Span(aspects.Length, rows, r);
            var span = gap * (to - from);
            for (var i = from; i <= to; i++)
            {
                span += height * aspects[i];
            }

            widest = Math.Max(widest, span);
        }

        return widest;
    }

    /// <summary>
    /// Which tiles are in one row, when tiles are spread as evenly as their count allows.
    ///
    /// The first rows take the extra tile where the count does not divide, so an arrangement of
    /// five in two rows is three and then two. Earlier rows being the fuller ones keeps the
    /// ragged row at the bottom, where a reader expects it.
    /// </summary>
    private static (int From, int To) Span(int count, int rows, int row)
    {
        var size = count / rows;
        var extra = count % rows;

        var from = (row * size) + Math.Min(row, extra);
        var length = size + (row < extra ? 1 : 0);
        return (from, from + length - 1);
    }

    /// <summary>
    /// The height one row of tiles takes when it is scaled to fill the width exactly.
    ///
    /// Every tile in the row is drawn at the row's height, so the row's width is that height
    /// times the sum of the aspect ratios, plus the gaps between them. Solving that for the
    /// height is the whole of the justification.
    /// </summary>
    private static double RowHeight(double[] aspects, int from, int to, double width, double gap)
    {
        var sum = 0.0;
        for (var i = from; i <= to; i++)
        {
            sum += aspects[i];
        }

        Assert.That(sum > 0, "a row of tiles has a positive total aspect", sum);

        var usable = width - (gap * (to - from));
        return usable <= 0 ? 0 : usable / sum;
    }

    /// <summary>
    /// Turns a candidate into rectangles, in the caller's own order.
    ///
    /// Every row is the candidate's one height, so what differs between rows is only how wide
    /// their contents come out. A row narrower than the box is centred rather than left-aligned:
    /// a grid with all its slack on one side reads as broken alignment, where the same slack
    /// split in two reads as a margin.
    ///
    /// The stack of rows is centred in the box the same way. It is short of the box's height
    /// whenever the width is what bounded the row height, and an arrangement that scrolls has no
    /// slack to split, so the margin above is zero there.
    /// </summary>
    private static IReadOnlyList<Placement> Place(
        double[] aspects, int[] order, Candidate candidate, double width, double height, double gap,
        out double total)
    {
        var placed = new Placement[aspects.Length];
        var rowHeight = candidate.Height;

        var stack = (rowHeight * candidate.Rows) + (gap * (candidate.Rows - 1));
        var y = Math.Max(0, (height - stack) / 2);

        for (var r = 0; r < candidate.Rows; r++)
        {
            var (from, to) = Span(aspects.Length, candidate.Rows, r);

            var span = gap * (to - from);
            for (var i = from; i <= to; i++)
            {
                span += rowHeight * aspects[i];
            }

            var x = Math.Max(0, (width - span) / 2);
            for (var i = from; i <= to; i++)
            {
                var tileWidth = rowHeight * aspects[i];
                placed[order[i]] = new Placement(order[i], x, y, tileWidth, rowHeight);
                x += tileWidth + gap;
            }

            y += rowHeight + gap;
        }

        // The last row adds no gap under itself.
        total = Math.Max(0, y - gap);

        Assert.That(
            candidate.Scrolls || total <= height + 0.001,
            "an arrangement that fits reaches no further than the box",
            total,
            height);
        return placed;
    }

    /// <summary>
    /// A usable aspect ratio for one tile.
    ///
    /// A stream that has not announced a pool reports nothing, and a pipeline that reported a
    /// zero dimension would divide the arrangement by zero. Both land on the assumed shape,
    /// which is the same answer for the same reason: nothing has said otherwise yet.
    /// </summary>
    private static double Sane(double aspect)
        => double.IsFinite(aspect) && aspect > 0 ? aspect : UnknownAspect;
}
