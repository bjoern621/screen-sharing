using Avalonia;
using Avalonia.Controls;

using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.Model;

namespace ScreenShare.App.Features.Viewer.View;

/// <summary>
/// Panel that puts tiles where the arrangement says, computing nothing itself.
/// Every rectangle comes from <see cref="TileLayout"/>, pure and tested without a window,
/// so what is here is the two Avalonia passes and the reading of each child's shape (<c>avalonia/README.md</c>).
///
/// A child declares its aspect ratio through <see cref="AspectProperty"/>
/// and whether it is the focused one through <see cref="IsFocusedTileProperty"/>, both bound in the item template.
/// Attached values rather than anybody's data context, so this knows no view model and a second kind of tile
/// needs no change here.
/// </summary>
public sealed class TileGrid : Panel
{
    /// <summary>
    /// Space between two tiles, and between two rows, in device-independent pixels.
    /// Fixed rather than styled: an argument to the arrangement, and spacing off a theme would be a layout whose
    /// test and whose screen disagreed.
    /// </summary>
    private const double Gap = 8;

    /// <summary>
    /// How much of the height the rail under a focused tile takes,
    /// held between <see cref="MinRail"/> and <see cref="MaxRail"/>.
    /// A fraction so the rail scales with the window, bounded so it neither disappears on a short one nor takes
    /// half of a tall one.
    /// The focused tile gets what the rail does not.
    /// </summary>
    private const double RailFraction = 0.18;

    private const double MinRail = 110;
    private const double MaxRail = 200;

    /// <summary>Width over height of one tile's stream, as the tile knows it.</summary>
    public static readonly AttachedProperty<double> AspectProperty =
        AvaloniaProperty.RegisterAttached<TileGrid, Control, double>("Aspect", TileLayout.UnknownAspect);

    /// <summary>At most one child carries it.</summary>
    public static readonly AttachedProperty<bool> IsFocusedTileProperty =
        AvaloniaProperty.RegisterAttached<TileGrid, Control, bool>("IsFocusedTile");

    public static readonly StyledProperty<LayoutMode> ModeProperty =
        AvaloniaProperty.Register<TileGrid, LayoutMode>(nameof(Mode));

    /// <summary>
    /// Height the arrangement is fitted into where the panel is measured with an unbounded one,
    /// which is what a scroll viewer hands it.
    /// Bound to the viewport rather than guessed: "fits the box" is the whole of what the arrangement decides,
    /// and a panel measured against infinity has no box.
    /// Zero before anything measured the viewport, the arrangement then taking whatever finite height the pass
    /// was given.
    /// </summary>
    public static readonly StyledProperty<double> ViewportProperty =
        AvaloniaProperty.Register<TileGrid, double>(nameof(Viewport));

    static TileGrid()
    {
        AffectsMeasure<TileGrid>(ModeProperty, ViewportProperty);
        AffectsParentMeasure<TileGrid>(AspectProperty, IsFocusedTileProperty);
    }

    public LayoutMode Mode
    {
        get => GetValue(ModeProperty);
        set => SetValue(ModeProperty, value);
    }

    public double Viewport
    {
        get => GetValue(ViewportProperty);
        set => SetValue(ViewportProperty, value);
    }

    public static double GetAspect(Control child) => child.GetValue(AspectProperty);

    public static void SetAspect(Control child, double value) => child.SetValue(AspectProperty, value);

    public static bool GetIsFocusedTile(Control child) => child.GetValue(IsFocusedTileProperty);

    public static void SetIsFocusedTile(Control child, bool value) => child.SetValue(IsFocusedTileProperty, value);

    /// <summary>
    /// Measures every child at the size it will be arranged at, and reports the height the arrangement needs.
    /// The arrangement's own height rather than the box's,
    /// so a grid that had to scroll comes out taller than its viewport,
    /// and the scroll viewer around it has something to scroll.
    /// A grid that fitted reports what it used.
    /// </summary>
    protected override Size MeasureOverride(Size availableSize)
    {
        var box = Box(availableSize);
        var places = Places(box);

        foreach (var (child, rect) in places)
        {
            child.Measure(new Size(rect.Width, rect.Height));
        }

        var height = places.Count == 0 ? 0 : places.Max(p => p.Rect.Bottom);
        return new Size(box.Width, height);
    }

    protected override Size ArrangeOverride(Size finalSize)
    {
        var places = Places(Box(finalSize));
        foreach (var (child, rect) in places)
        {
            child.Arrange(rect);
        }

        return finalSize;
    }

    /// <summary>
    /// Box the arrangement is fitted into, one box for both passes.
    ///
    /// <see cref="Viewport"/> wins over the size a pass was handed.
    /// Inside a scroll viewer the two passes get different heights,
    /// measure an unbounded one and arrange the height measure returned,
    /// so solving against what each was given solves two boxes,
    /// placing the tiles by one having measured them by the other.
    /// The viewport is the space a reader sees, what "fits the box" is about.
    ///
    /// A non-finite width, and an unmeasured height, both count as no room: the arrangement places nothing until
    /// a pass carries a real box.
    /// </summary>
    private Size Box(Size available)
    {
        var height = Viewport > 0
            ? Viewport
            : (double.IsFinite(available.Height) && available.Height > 0 ? available.Height : 0);

        return new Size(
            double.IsFinite(available.Width) ? available.Width : 0,
            height);
    }

    private List<(Control Child, Rect Rect)> Places(Size box)
        => Mode == LayoutMode.Focus ? Focused(box) : Grid(box);

    /// <summary>
    /// Every tile at the one height the arrangement chose, each as wide as its own shape makes it there.
    /// A box with no room arranges nothing (<see cref="TileLayout.Solve"/>), which is the first measure pass
    /// inside a scroll viewer: the viewport is unmeasured, so there is no height to solve against until the pass
    /// after.
    /// Every child is placed at an empty rectangle there rather than left out,
    /// a child this list skips being a child the measure pass never measures.
    /// </summary>
    private List<(Control Child, Rect Rect)> Grid(Size box)
    {
        var children = Children.OfType<Control>().ToList();
        var arrangement = TileLayout.Solve(children.Select(GetAspect).ToList(), box.Width, box.Height, Gap);

        Assert.That(
            arrangement.Tiles.Count == children.Count || arrangement.Tiles.Count == 0,
            "the arrangement places every tile or none of them",
            arrangement.Tiles.Count,
            children.Count);

        var places = children.Select(child => (Child: child, Rect: default(Rect))).ToList();
        foreach (var tile in arrangement.Tiles)
        {
            places[tile.Index] = (children[tile.Index], new Rect(tile.X, tile.Y, tile.Width, tile.Height));
        }

        return places;
    }

    /// <summary>
    /// One tile above, the rest in a rail below.
    /// With nothing focused it is the grid,
    /// so the mode is safe to be in while a focused stream is being chosen and after it has gone:
    /// the arrangement degrades to the other mode rather than to an empty screen.
    /// </summary>
    private List<(Control Child, Rect Rect)> Focused(Size box)
    {
        var children = Children.OfType<Control>().ToList();
        var focused = children.FirstOrDefault(GetIsFocusedTile);
        if (focused is null || children.Count == 0)
        {
            return Grid(box);
        }

        var rest = children.Where(child => child != focused).ToList();
        var rail = rest.Count == 0 ? 0 : Math.Clamp(box.Height * RailFraction, MinRail, MaxRail);
        var stage = new Size(box.Width, Math.Max(0, box.Height - rail - (rail > 0 ? Gap : 0)));

        var places = new List<(Control, Rect)>(children.Count)
        {
            (focused, Letterbox(GetAspect(focused), new Rect(0, 0, stage.Width, stage.Height))),
        };

        // One row at a fixed height, tiles at their own widths.
        // Centred while they fit, left-aligned once they do not.
        // Not the grid arrangement: the rail's height comes off the stage above it rather than off the tiles in it.
        var y = stage.Height + Gap;
        var span = rest.Sum(child => rail * GetAspect(child)) + (Gap * Math.Max(0, rest.Count - 1));
        var x = span < box.Width ? (box.Width - span) / 2 : 0;

        foreach (var child in rest)
        {
            var width = rail * GetAspect(child);
            places.Add((child, new Rect(x, y, width, rail)));
            x += width + Gap;
        }

        Assert.That(places.Count == children.Count, "a place for every tile", places.Count, children.Count);
        return places;
    }

    /// <summary>
    /// Largest rectangle of the given shape that fits the box, centred in it.
    /// The focused tile keeps its stream's shape as every other tile does, being alone turning the leftover space
    /// into a margin rather than into another tile.
    /// </summary>
    private static Rect Letterbox(double aspect, Rect box)
    {
        if (box.Width <= 0 || box.Height <= 0)
        {
            return new Rect(box.X, box.Y, 0, 0);
        }

        var width = box.Width;
        var height = width / aspect;
        if (height > box.Height)
        {
            height = box.Height;
            width = height * aspect;
        }

        return new Rect(
            box.X + ((box.Width - width) / 2),
            box.Y + ((box.Height - height) / 2),
            width,
            height);
    }
}
