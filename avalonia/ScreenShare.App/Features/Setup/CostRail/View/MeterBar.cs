using Avalonia;
using Avalonia.Controls;
using Avalonia.Media;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Setup.CostRail.View;

/// <summary>
/// A figure against a limit, both as shares of one bar: a track, a fill, and a one-pixel
/// marker where the limit stands.
///
/// Drawn rather than laid out. The obvious markup - two star columns whose widths a converter
/// derives from the share - binds against <c>ColumnDefinition</c>, which is not an element and
/// therefore has no data context to bind through. Drawing it is also what lets the fill run
/// past the marker instead of being clamped at it, which is the one state this control exists
/// to show: a predicted bitrate the line cannot carry.
///
/// It states no colour and no size. Everything comes in from the caller, so the design system
/// stays the only thing that decides them (avalonia/README.md, "Nothing outside Design/").
/// </summary>
public sealed class MeterBar : Control
{
    /// <summary>How much of the bar the figure occupies, 0 to 1.</summary>
    public static readonly StyledProperty<double> FillProperty =
        AvaloniaProperty.Register<MeterBar, double>(nameof(Fill));

    /// <summary>Where along the bar the limit stands, 0 to 1. Zero draws no marker.</summary>
    public static readonly StyledProperty<double> LimitProperty =
        AvaloniaProperty.Register<MeterBar, double>(nameof(Limit));

    /// <summary>The unfilled track behind everything.</summary>
    public static readonly StyledProperty<IBrush?> TrackBrushProperty =
        AvaloniaProperty.Register<MeterBar, IBrush?>(nameof(TrackBrush));

    public static readonly StyledProperty<IBrush?> FillBrushProperty =
        AvaloniaProperty.Register<MeterBar, IBrush?>(nameof(FillBrush));

    public static readonly StyledProperty<IBrush?> LimitBrushProperty =
        AvaloniaProperty.Register<MeterBar, IBrush?>(nameof(LimitBrush));

    /// <summary>The track's own height, which is less than the control's: the marker overhangs it.</summary>
    public static readonly StyledProperty<double> TrackHeightProperty =
        AvaloniaProperty.Register<MeterBar, double>(nameof(TrackHeight), 5);

    static MeterBar() => AffectsRender<MeterBar>(
        FillProperty, LimitProperty, TrackBrushProperty, FillBrushProperty,
        LimitBrushProperty, TrackHeightProperty);

    public double Fill
    {
        get => GetValue(FillProperty);
        set => SetValue(FillProperty, value);
    }

    public double Limit
    {
        get => GetValue(LimitProperty);
        set => SetValue(LimitProperty, value);
    }

    public IBrush? TrackBrush
    {
        get => GetValue(TrackBrushProperty);
        set => SetValue(TrackBrushProperty, value);
    }

    public IBrush? FillBrush
    {
        get => GetValue(FillBrushProperty);
        set => SetValue(FillBrushProperty, value);
    }

    public IBrush? LimitBrush
    {
        get => GetValue(LimitBrushProperty);
        set => SetValue(LimitBrushProperty, value);
    }

    public double TrackHeight
    {
        get => GetValue(TrackHeightProperty);
        set => SetValue(TrackHeightProperty, value);
    }

    /// <summary>
    /// The one draw pass. Back to front: the track, the fill clipped to its rounded ends, and
    /// the marker over both at the control's full height so it reads as a limit rather than as
    /// part of the fill.
    /// </summary>
    public override void Render(DrawingContext context)
    {
        var width = Bounds.Width;
        var height = Bounds.Height;
        if (width <= 0 || height <= 0)
        {
            return;
        }

        var thickness = Math.Min(TrackHeight, height);
        var radius = thickness / 2;
        var track = new Rect(0, (height - thickness) / 2, width, thickness);

        if (TrackBrush is { } behind)
        {
            context.DrawRectangle(behind, null, new RoundedRect(track, radius));
        }

        var fill = Math.Clamp(Fill, 0, 1);
        if (FillBrush is { } front && fill > 0)
        {
            // Clipped to the track's own shape, so a short fill keeps the rounded left cap and
            // gains no rounded right one.
            using (context.PushGeometryClip(new RectangleGeometry(track, radius, radius)))
            {
                context.FillRectangle(front, new Rect(track.X, track.Y, width * fill, thickness));
            }
        }

        var limit = Math.Clamp(Limit, 0, 1);
        if (LimitBrush is { } marker && limit > 0)
        {
            context.FillRectangle(marker, new Rect(Math.Min(width * limit, width - 1), 0, 1, height));
        }

        Assert.That(thickness > 0, "a bar has a track to draw", thickness, height);
    }
}
