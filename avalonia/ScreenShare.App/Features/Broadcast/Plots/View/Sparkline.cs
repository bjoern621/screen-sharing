using Avalonia;
using Avalonia.Controls;
using Avalonia.Media;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Broadcast.Plots.View;

/// <summary>
/// One or two polylines over a source coordinate space, with an optional area under the first, one optional
/// horizontal rule and one optional shaded band.
///
/// No axes, no ticks, no frame lines and not one glyph: the scale is carried by the annotation text the plot
/// card lays over this control.
///
/// Samples arrive in <see cref="Extent"/> coordinates and are stretched to the control's bounds, so the whole
/// window stays on screen at any card width.
/// Asks for no size of its own and draws nothing until the bounds it is given have area.
/// How much stream that window covers is a fact about the samples, so the card states it.
/// </summary>
public sealed class Sparkline : Control
{
    /// <summary>Primary series, in <see cref="Extent"/> coordinates. Drawn last, so it sits on top.</summary>
    public static readonly StyledProperty<IReadOnlyList<Point>?> PointsProperty =
        AvaloniaProperty.Register<Sparkline, IReadOnlyList<Point>?>(nameof(Points));

    /// <summary>Second series, drawn first and thinner so it reads as context.</summary>
    public static readonly StyledProperty<IReadOnlyList<Point>?> SecondaryProperty =
        AvaloniaProperty.Register<Sparkline, IReadOnlyList<Point>?>(nameof(Secondary));

    /// <summary>Source space the samples are expressed in.</summary>
    public static readonly StyledProperty<Size> ExtentProperty =
        AvaloniaProperty.Register<Sparkline, Size>(nameof(Extent), new Size(480, 104));

    /// <summary>Area under the primary series. Null leaves the line unfilled.</summary>
    public static readonly StyledProperty<IBrush?> FillProperty =
        AvaloniaProperty.Register<Sparkline, IBrush?>(nameof(Fill));

    public static readonly StyledProperty<IBrush?> StrokeProperty =
        AvaloniaProperty.Register<Sparkline, IBrush?>(nameof(Stroke));

    public static readonly StyledProperty<double> StrokeThicknessProperty =
        AvaloniaProperty.Register<Sparkline, double>(nameof(StrokeThickness), 1.5);

    public static readonly StyledProperty<IBrush?> SecondaryStrokeProperty =
        AvaloniaProperty.Register<Sparkline, IBrush?>(nameof(SecondaryStroke));

    public static readonly StyledProperty<double> SecondaryThicknessProperty =
        AvaloniaProperty.Register<Sparkline, double>(nameof(SecondaryThickness), 1.2);

    public static readonly StyledProperty<IBrush?> GridlineBrushProperty =
        AvaloniaProperty.Register<Sparkline, IBrush?>(nameof(GridlineBrush));

    /// <summary>Height of the one horizontal rule, 0 at the top to 1 at the bottom. NaN draws none.</summary>
    public static readonly StyledProperty<double> GridlineFractionProperty =
        AvaloniaProperty.Register<Sparkline, double>(nameof(GridlineFraction), double.NaN);

    public static readonly StyledProperty<IBrush?> BandBrushProperty =
        AvaloniaProperty.Register<Sparkline, IBrush?>(nameof(BandBrush));

    public static readonly StyledProperty<IBrush?> BandEdgeBrushProperty =
        AvaloniaProperty.Register<Sparkline, IBrush?>(nameof(BandEdgeBrush));

    /// <summary>Left edge of the shaded band, as a fraction of the width. NaN draws none.</summary>
    public static readonly StyledProperty<double> BandStartProperty =
        AvaloniaProperty.Register<Sparkline, double>(nameof(BandStart), double.NaN);

    /// <summary>Band width, as a fraction of the width. 0 draws none, and the band ends inside the plot.</summary>
    public static readonly StyledProperty<double> BandWidthProperty =
        AvaloniaProperty.Register<Sparkline, double>(nameof(BandWidth));

    static Sparkline() => AffectsRender<Sparkline>(
        PointsProperty, SecondaryProperty, ExtentProperty, FillProperty, StrokeProperty,
        StrokeThicknessProperty, SecondaryStrokeProperty, SecondaryThicknessProperty,
        GridlineBrushProperty, GridlineFractionProperty, BandBrushProperty,
        BandEdgeBrushProperty, BandStartProperty, BandWidthProperty);

    public IReadOnlyList<Point>? Points
    {
        get => GetValue(PointsProperty);
        set => SetValue(PointsProperty, value);
    }

    public IReadOnlyList<Point>? Secondary
    {
        get => GetValue(SecondaryProperty);
        set => SetValue(SecondaryProperty, value);
    }

    public Size Extent
    {
        get => GetValue(ExtentProperty);
        set => SetValue(ExtentProperty, value);
    }

    public IBrush? Fill
    {
        get => GetValue(FillProperty);
        set => SetValue(FillProperty, value);
    }

    public IBrush? Stroke
    {
        get => GetValue(StrokeProperty);
        set => SetValue(StrokeProperty, value);
    }

    public double StrokeThickness
    {
        get => GetValue(StrokeThicknessProperty);
        set => SetValue(StrokeThicknessProperty, value);
    }

    public IBrush? SecondaryStroke
    {
        get => GetValue(SecondaryStrokeProperty);
        set => SetValue(SecondaryStrokeProperty, value);
    }

    public double SecondaryThickness
    {
        get => GetValue(SecondaryThicknessProperty);
        set => SetValue(SecondaryThicknessProperty, value);
    }

    public IBrush? GridlineBrush
    {
        get => GetValue(GridlineBrushProperty);
        set => SetValue(GridlineBrushProperty, value);
    }

    public double GridlineFraction
    {
        get => GetValue(GridlineFractionProperty);
        set => SetValue(GridlineFractionProperty, value);
    }

    public IBrush? BandBrush
    {
        get => GetValue(BandBrushProperty);
        set => SetValue(BandBrushProperty, value);
    }

    public IBrush? BandEdgeBrush
    {
        get => GetValue(BandEdgeBrushProperty);
        set => SetValue(BandEdgeBrushProperty, value);
    }

    public double BandStart
    {
        get => GetValue(BandStartProperty);
        set => SetValue(BandStartProperty, value);
    }

    public double BandWidth
    {
        get => GetValue(BandWidthProperty);
        set => SetValue(BandWidthProperty, value);
    }

    /// <summary>The one draw pass. Back to front, so the data reads in front of what annotates it.</summary>
    public override void Render(DrawingContext context)
    {
        var width = Bounds.Width;
        var height = Bounds.Height;
        if (width <= 0 || height <= 0)
        {
            return;
        }

        DrawBand(context, width, height);
        DrawGridline(context, width, height);
        DrawSeries(context, width, height, Secondary, null, SecondaryStroke, SecondaryThickness);
        DrawSeries(context, width, height, Points, Fill, Stroke, StrokeThickness);
    }

    private void DrawSeries(
        DrawingContext context, double width, double height,
        IReadOnlyList<Point>? samples, IBrush? fill, IBrush? stroke, double thickness)
    {
        if (samples is not { Count: > 1 })
        {
            return;
        }

        var extent = Extent;
        Assert.That(extent.Width > 0 && extent.Height > 0,
            "a sparkline maps from a source space with area", extent.Width, extent.Height);

        var mapped = new Point[samples.Count];
        for (var i = 0; i < samples.Count; i++)
        {
            mapped[i] = new Point(samples[i].X / extent.Width * width, samples[i].Y / extent.Height * height);
        }

        if (fill is not null)
        {
            // Line, down to the floor, back along it to where the line started.
            var area = new Point[mapped.Length + 2];
            mapped.CopyTo(area, 0);
            area[^2] = new Point(mapped[^1].X, height);
            area[^1] = new Point(mapped[0].X, height);
            context.DrawGeometry(fill, null, new PolylineGeometry(area, true));
        }

        if (stroke is not null && thickness > 0)
        {
            context.DrawGeometry(null, new Pen(stroke, thickness), new PolylineGeometry(mapped, false));
        }
    }

    private void DrawGridline(DrawingContext context, double width, double height)
    {
        var brush = GridlineBrush;
        var fraction = GridlineFraction;
        if (brush is null || double.IsNaN(fraction))
        {
            return;
        }

        Assert.That(fraction is >= 0 and <= 1, "a gridline sits inside the plot", fraction);

        var y = fraction * height;
        context.DrawLine(new Pen(brush, 1), new Point(0, y), new Point(width, y));
    }

    private void DrawBand(DrawingContext context, double width, double height)
    {
        var start = BandStart;
        var extent = BandWidth;
        if (double.IsNaN(start) || extent <= 0)
        {
            return;
        }

        Assert.That(start >= 0 && start + extent <= 1, "a band sits inside the plot", start, extent);

        var left = start * width;
        var right = (start + extent) * width;

        if (BandBrush is not null)
        {
            context.DrawRectangle(BandBrush, null, new Rect(left, 0, right - left, height));
        }

        if (BandEdgeBrush is not null)
        {
            // Only dashed line in the design. Every data stroke stays solid.
            var pen = new Pen(BandEdgeBrush, 1, new DashStyle([3, 3], 0));
            context.DrawLine(pen, new Point(left, 0), new Point(left, height));
            context.DrawLine(pen, new Point(right, 0), new Point(right, height));
        }
    }
}
