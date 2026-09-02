using Avalonia;
using Avalonia.Controls;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Controls;

/// <summary>
/// A body and its side column, in the two arrangements a window puts them in: the column beside the body,
/// or the column over it across the whole window (<c>docs/design-language.md</c>, "Narrow windows").
///
/// Children in one order: the body, the column that draws over it, and the foot the column carries where there
/// is one.
/// One element per region in both arrangements, so what a surface draws does not depend on how wide the window is.
///
/// The foot stands under the column while the column is beside the body,
/// and across the window while the column is over it.
/// It is the row a panel must not cover: a wizard whose Continue went behind an opened panel has no way on.
///
/// Not a <see cref="Grid"/>: moving a child between a fixed column and a span over both leaves the column's own
/// width to be taken back somewhere, and a spanning child grows an auto column it was supposed to cover.
/// </summary>
public sealed class SideColumnPanel : Panel
{
    public static readonly StyledProperty<bool> BesideProperty =
        AvaloniaProperty.Register<SideColumnPanel, bool>(nameof(Beside), defaultValue: true);

    public static readonly StyledProperty<bool> SideOnLeftProperty =
        AvaloniaProperty.Register<SideColumnPanel, bool>(nameof(SideOnLeft));

    public static readonly StyledProperty<double> SideWidthProperty =
        AvaloniaProperty.Register<SideColumnPanel, double>(nameof(SideWidth));

    public static readonly StyledProperty<double> GapProperty =
        AvaloniaProperty.Register<SideColumnPanel, double>(nameof(Gap));

    static SideColumnPanel()
        => AffectsMeasure<SideColumnPanel>(BesideProperty, SideOnLeftProperty, SideWidthProperty, GapProperty);

    /// <summary>Whether the column stands beside the body rather than over it.</summary>
    public bool Beside
    {
        get => GetValue(BesideProperty);
        set => SetValue(BesideProperty, value);
    }

    public bool SideOnLeft
    {
        get => GetValue(SideOnLeftProperty);
        set => SetValue(SideOnLeftProperty, value);
    }

    /// <summary>Column's width while it stands beside the body, in device-independent pixels.</summary>
    public double SideWidth
    {
        get => GetValue(SideWidthProperty);
        set => SetValue(SideWidthProperty, value);
    }

    /// <summary>Distance between the body and a column beside it.</summary>
    public double Gap
    {
        get => GetValue(GapProperty);
        set => SetValue(GapProperty, value);
    }

    /// <summary>Foot the column carries, absent where the surface states two children.</summary>
    private Control? Foot => Children.Count == 3 ? Children[2] : null;

    /// <summary>Whether the foot stands under the column rather than across the window.</summary>
    private bool FootUnderSide => Beside && Children[1].IsVisible;

    protected override Size MeasureOverride(Size availableSize)
    {
        Assert.That(Children.Count is 2 or 3, "a side column panel holds a body, a column and at most one foot", Children.Count);
        Assert.That(SideWidth >= 0 && Gap >= 0, "a column's width and its gap are distances", SideWidth, Gap);

        var (body, side) = Slots(availableSize.Width);

        Foot?.Measure(availableSize.WithWidth(FootUnderSide ? side : availableSize.Width));

        var foot = Foot?.DesiredSize.Height ?? 0;
        var above = Math.Max(0, availableSize.Height - foot);

        Children[0].Measure(new Size(body, FootUnderSide ? availableSize.Height : above));
        Children[1].Measure(new Size(side, above));

        var height = Math.Max(Children[0].DesiredSize.Height, Children[1].DesiredSize.Height + foot);

        return new Size(availableSize.Width, height);
    }

    protected override Size ArrangeOverride(Size finalSize)
    {
        var (body, side) = Slots(finalSize.Width);
        var sideOnLeft = Children[1].IsVisible && Beside && SideOnLeft;
        var sideX = sideOnLeft || !Beside ? 0 : finalSize.Width - side;

        var foot = Foot?.DesiredSize.Height ?? 0;
        var above = Math.Max(0, finalSize.Height - foot);

        Children[0].Arrange(new Rect(
            sideOnLeft ? finalSize.Width - body : 0,
            0,
            body,
            FootUnderSide ? finalSize.Height : above));

        Children[1].Arrange(new Rect(sideX, 0, side, above));

        Foot?.Arrange(FootUnderSide
            ? new Rect(sideX, above, side, foot)
            : new Rect(0, above, finalSize.Width, foot));

        return finalSize;
    }

    /// <summary>
    /// What the body and the column are given of a window this wide.
    /// A column that is not drawn leaves the whole window to the body, whichever arrangement holds,
    /// and a window narrower than the column leaves the body none rather than a negative width.
    /// </summary>
    private (double Body, double Side) Slots(double window)
    {
        if (!Children[1].IsVisible)
        {
            return (window, 0);
        }

        if (!Beside)
        {
            return (window, window);
        }

        return (Math.Max(0, window - SideWidth - Gap), Math.Min(window, SideWidth));
    }
}
