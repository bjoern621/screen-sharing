using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Viewer.Model;

/// <summary>
/// Where tiles of given shapes go in a given box.
///
/// Pure: sizes and aspect ratios in, rectangles out, holding no control, no view model and no state, so the
/// arrangement is asserted in tests without a window and the panel drawing it carries no arithmetic
/// (<c>avalonia/README.md</c>).
///
/// Every length is an Avalonia layout unit, and a rectangle is in the box's own space with the origin at its top
/// left.
///
/// Every tile is drawn at the one height the whole arrangement uses and is as wide as its own aspect ratio makes
/// it there, so nothing is cropped or stretched: a 21:9 stream beside a 4:3 one is simply wider.
/// Letting each row fill the width exactly would instead give each row its own height, and a row of one tile
/// would come out about twice the height of a row of two.
/// The height is chosen once: the largest that lets every row fit the width and the whole stack fit the box.
///
/// Rows are centred in whatever width their contents leave over and the stack in whatever height it leaves over,
/// so the slack reads as a margin rather than as an empty half-window.
/// </summary>
public static class TileLayout
{
    /// <summary>
    /// Aspect a tile is laid out at before its stream has said what shape it is.
    /// The real shape arrives with the frame channel's first pool and not before.
    /// 16:9 until then, what nearly every capture and test pattern here is, so a tile does not visibly jump when
    /// it settles.
    /// </summary>
    public const double UnknownAspect = 16.0 / 9.0;

    /// <summary>
    /// Shortest row drawn before the arrangement scrolls instead.
    /// Below it the name, the figures and the meter stop fitting.
    /// The one place the box handed in is not honoured.
    /// </summary>
    public const double MinRowHeight = 160;

    /// <summary>Where one tile lands.</summary>
    /// <param name="Index">
    /// Which of the tiles handed in this rectangle belongs to.
    /// Results come back in the caller's own order whatever the search sorted, so no caller undoes a sort.
    /// </param>
    public readonly record struct Placement(int Index, double X, double Y, double Width, double Height);

    /// <summary>Where every tile goes, how far the arrangement reaches, and whether it outgrew the box.</summary>
    /// <param name="Height">
    /// Margin above the rows plus the rows themselves.
    /// A caller sizing itself by this leaves the same margin under the tiles as the arrangement put over them.
    /// </param>
    /// <param name="Scrolls">
    /// The tiles could not be fitted at a legible size, so the container has to scroll.
    /// A caller that cannot scroll gets a taller arrangement than it asked for rather than a squashed one.
    /// </param>
    public readonly record struct Arrangement(IReadOnlyList<Placement> Tiles, double Height, bool Scrolls);

    /// <summary>
    /// Solves the arrangement for tiles of the given aspect ratios.
    ///
    /// An arrangement is decided by one number, its row count, with the tiles spread across those rows as evenly
    /// as their count allows.
    /// Every count from one to the number of tiles is tried and scored by the picture area it draws, so the search
    /// is exhaustive over the thing that matters rather than a heuristic that stops early.
    /// Even spreading rather than a search over every possible break, the tiles being sorted by shape first and an
    /// uneven break then buying almost nothing for a combinatorial cost.
    ///
    /// Tiles are sorted by aspect ratio, widest first, so a row is made of similar shapes.
    /// Ties keep their input order, so the result is deterministic.
    /// A tile whose aspect resolves from the assumed one moves, the price of packing tightly.
    ///
    /// Where no row count draws every row at a legible height, the box stops being honoured: rows go at
    /// <see cref="MinRowHeight"/> and the result reports that it scrolls.
    /// </summary>
    /// <param name="aspects">Width over height per tile, in the caller's own order.</param>
    /// <param name="width">Usable width. A non-positive width or height arranges nothing.</param>
    /// <param name="height">Usable height, which the arrangement fits inside where it can.</param>
    /// <param name="gap">Space between two tiles and between two rows.</param>
    public static Arrangement Solve(IReadOnlyList<double> aspects, double width, double height, double gap)
    {
        Assert.NotNull(aspects, "an arrangement is of some tiles");
        Assert.That(gap >= 0, "the space between tiles is not negative", gap);

        if (aspects.Count == 0 || width <= 0 || height <= 0)
        {
            return new Arrangement([], 0, false);
        }

        // Sorted here and mapped back at the end, so no caller learns that the arrangement sorted anything.
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

    /// <summary>One candidate: row count, the height every row is drawn at, and whether it fits.</summary>
    private readonly record struct Candidate(int Rows, double Height, bool Scrolls);

    /// <summary>
    /// Row count covering the most picture area.
    ///
    /// Candidates are scored by the area their tiles cover, which makes fewer larger tiles against more smaller
    /// ones an answer rather than a preference: one row of four wide tiles covers less of a tall window than two
    /// rows of two.
    /// Every tile is the one height, so that area is the height squared times the sum of the aspect ratios, and
    /// the sum does not depend on the row count.
    /// The best candidate is the one whose height comes out largest, over a single number.
    ///
    /// A candidate below <see cref="MinRowHeight"/> is not scored, unless none is legible, and then the tiles go
    /// at that minimum and it scrolls.
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

        // No row count was legible in this box.
        // Tiles go at the legible minimum, in the fewest rows whose widest row still fits the width there, and the
        // reader scrolls.
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
    /// The one height every tile is drawn at for a given row count, zero where there is none.
    /// Bounded from two sides and the smaller wins: the height that makes the widest row exactly fill the width,
    /// and the box's height shared between the rows and the gaps.
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

    /// <summary>Width of a candidate's widest row, at a given height.</summary>
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
    /// Which tiles fall in one row when spread as evenly as their count allows.
    /// Earlier rows take the extra tile where the count does not divide, so five in two rows is three then two,
    /// which keeps the ragged row at the bottom.
    /// </summary>
    /// <returns>First and last index of the row, both inclusive.</returns>
    private static (int From, int To) Span(int count, int rows, int row)
    {
        var size = count / rows;
        var extra = count % rows;

        var from = (row * size) + Math.Min(row, extra);
        var length = size + (row < extra ? 1 : 0);
        return (from, from + length - 1);
    }

    /// <summary>
    /// Height one row takes when scaled to fill the width exactly, zero where the gaps alone exceed the width.
    /// A row's width is its height times the sum of its aspect ratios plus the gaps, solved for that height.
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
    /// Rectangles for one candidate, back in the caller's order.
    ///
    /// Every row is the candidate's one height, so rows differ only in how wide their contents come out.
    /// A row narrower than the box is centred: slack split in two reads as a margin, all of it on one side reads
    /// as broken alignment.
    ///
    /// The stack of rows is centred the same way.
    /// It falls short of the box's height whenever the width bounded the row height, and an arrangement that
    /// scrolls has no slack to split, so the margin above is zero there.
    /// </summary>
    /// <param name="total">How far down the box the rectangles reach, counting no gap under the last row.</param>
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

        // No gap under the last row.
        total = Math.Max(0, y - gap);

        Assert.That(
            candidate.Scrolls || total <= height + 0.001,
            "an arrangement that fits reaches no further than the box",
            total,
            height);
        return placed;
    }

    /// <summary>
    /// An aspect ratio safe to lay one tile out at.
    /// Zero, negative and non-finite all land on <see cref="UnknownAspect"/>: a stream with no pool announced has
    /// reported nothing, and a zero or NaN dimension would divide the arrangement by zero.
    /// </summary>
    private static double Sane(double aspect)
        => double.IsFinite(aspect) && aspect > 0 ? aspect : UnknownAspect;
}
