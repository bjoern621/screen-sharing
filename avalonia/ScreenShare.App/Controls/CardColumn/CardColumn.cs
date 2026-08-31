using Avalonia;
using Avalonia.Controls;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Controls;

/// <summary>
/// Vertical column whose gap falls between the children that drew something.
/// A child measuring to nothing takes no gap, so a component deciding on its own terms whether it draws can sit
/// in the column unconditionally and the cards under it stay where they are.
///
/// Not <see cref="StackPanel"/>: that charges a gap for every child whose <see cref="Visual.IsVisible"/> is set,
/// and a component drawing nothing is still such a child.
/// One column then starts a gap lower than the column beside it, which is what every card layout in this app
/// avoids by stacking here.
/// </summary>
public sealed class CardColumn : Panel
{
    public static readonly StyledProperty<double> SpacingProperty =
        AvaloniaProperty.Register<CardColumn, double>(nameof(Spacing));

    static CardColumn() => AffectsMeasure<CardColumn>(SpacingProperty);

    /// <summary>Distance between two drawn children, in device-independent pixels.</summary>
    public double Spacing
    {
        get => GetValue(SpacingProperty);
        set => SetValue(SpacingProperty, value);
    }

    protected override Size MeasureOverride(Size availableSize)
    {
        Assert.That(Spacing >= 0, "a column's gap is a distance", Spacing);

        // Unbounded downwards, the column being what a ScrollViewer scrolls.
        var slot = availableSize.WithHeight(double.PositiveInfinity);
        var width = 0.0;
        var height = 0.0;
        var drawn = false;

        foreach (var child in Children)
        {
            child.Measure(slot);

            if (!Draws(child))
            {
                continue;
            }

            height += drawn ? Spacing + child.DesiredSize.Height : child.DesiredSize.Height;
            width = Math.Max(width, child.DesiredSize.Width);
            drawn = true;
        }

        Assert.That(height >= 0 && width >= 0, "a column measures to a size", width, height);

        return new Size(width, height);
    }

    protected override Size ArrangeOverride(Size finalSize)
    {
        var y = 0.0;
        var drawn = false;

        foreach (var child in Children)
        {
            if (!Draws(child))
            {
                child.Arrange(new Rect(0, y, finalSize.Width, 0));
                continue;
            }

            y += drawn ? Spacing : 0;
            child.Arrange(new Rect(0, y, finalSize.Width, child.DesiredSize.Height));
            y += child.DesiredSize.Height;
            drawn = true;
        }

        return finalSize;
    }

    /// <summary>
    /// Whether a child put anything on the screen this pass.
    /// A hidden child measures to nothing, and so does one whose own content is all hidden,
    /// which is the case a gap must not be charged for.
    /// </summary>
    private static bool Draws(Control child) => child.DesiredSize.Height > 0;
}
