using Avalonia;
using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.RegionPicker.View;

/// <summary>
/// Draws one rectangle of a picture that covers a whole screen.
///
/// The child is arranged larger than this control and clipped to it,
/// scaled until the rectangle fills the box and offset so the rectangle's corner lands on the box's.
/// The picture is stretched across whatever bounds the child is given
/// (<c>Features/Viewer/Tile/View/StreamTile.cs</c>), so the arithmetic is the rectangle against the screen
/// and nothing about how many pixels either carries.
///
/// The box takes the rectangle's own shape: a width from the layout, a height from the rectangle's aspect.
/// A box of a fixed shape would show a wide rectangle as a square one.
/// </summary>
public sealed class RegionCrop : Decorator
{
    /// <summary>Previewed screen's size, which the child's picture covers exactly.</summary>
    public static readonly StyledProperty<int> SourceWidthProperty =
        AvaloniaProperty.Register<RegionCrop, int>(nameof(SourceWidth));

    public static readonly StyledProperty<int> SourceHeightProperty =
        AvaloniaProperty.Register<RegionCrop, int>(nameof(SourceHeight));

    /// <summary>Rectangle in that screen's own pixels.</summary>
    public static readonly StyledProperty<int> CropXProperty =
        AvaloniaProperty.Register<RegionCrop, int>(nameof(CropX));

    public static readonly StyledProperty<int> CropYProperty =
        AvaloniaProperty.Register<RegionCrop, int>(nameof(CropY));

    public static readonly StyledProperty<int> CropWidthProperty =
        AvaloniaProperty.Register<RegionCrop, int>(nameof(CropWidth));

    public static readonly StyledProperty<int> CropHeightProperty =
        AvaloniaProperty.Register<RegionCrop, int>(nameof(CropHeight));

    static RegionCrop()
        => AffectsMeasure<RegionCrop>(
            SourceWidthProperty,
            SourceHeightProperty,
            CropXProperty,
            CropYProperty,
            CropWidthProperty,
            CropHeightProperty);

    public RegionCrop() => ClipToBounds = true;

    public int SourceWidth
    {
        get => GetValue(SourceWidthProperty);
        set => SetValue(SourceWidthProperty, value);
    }

    public int SourceHeight
    {
        get => GetValue(SourceHeightProperty);
        set => SetValue(SourceHeightProperty, value);
    }

    public int CropX
    {
        get => GetValue(CropXProperty);
        set => SetValue(CropXProperty, value);
    }

    public int CropY
    {
        get => GetValue(CropYProperty);
        set => SetValue(CropYProperty, value);
    }

    public int CropWidth
    {
        get => GetValue(CropWidthProperty);
        set => SetValue(CropWidthProperty, value);
    }

    public int CropHeight
    {
        get => GetValue(CropHeightProperty);
        set => SetValue(CropHeightProperty, value);
    }

    /// <summary>
    /// The box: the width offered, and the height the rectangle's aspect asks for.
    /// Zero on a rectangle or a screen with no size, which is a chooser with nothing to draw yet.
    /// </summary>
    protected override Size MeasureOverride(Size availableSize)
    {
        if (!Measurable(availableSize.Width, out var box))
        {
            return default;
        }

        Child?.Measure(box);
        return box;
    }

    protected override Size ArrangeOverride(Size finalSize)
    {
        if (Child is null || !Measurable(finalSize.Width, out _))
        {
            return finalSize;
        }

        var scaleX = finalSize.Width / CropWidth;
        var scaleY = finalSize.Height / CropHeight;

        Child.Arrange(new Rect(
            -CropX * scaleX,
            -CropY * scaleY,
            SourceWidth * scaleX,
            SourceHeight * scaleY));

        return finalSize;
    }

    /// <summary>Whether there is a box to draw, and how big it is.</summary>
    private bool Measurable(double width, out Size box)
    {
        box = default;

        if (SourceWidth <= 0 || SourceHeight <= 0 || CropWidth <= 0 || CropHeight <= 0)
        {
            return false;
        }

        if (!double.IsFinite(width) || width <= 0)
        {
            return false;
        }

        box = new Size(width, width * CropHeight / CropWidth);
        return true;
    }
}
