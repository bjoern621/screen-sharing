using Avalonia;
using Avalonia.Controls;
using Avalonia.Media;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Setup.CostRail.View;

/// <summary>
/// A figure against a limit, both as shares of one bar: a track, a fill, and a marker where the limit stands.
///
/// Drawn rather than laid out.
/// Two star columns sized by a converter would bind against <c>ColumnDefinition</c>, which is not an element
/// and has no data context to bind through.
/// Drawing also lets the fill run past the marker rather than stop at it, the state the bar exists to show:
/// a predicted bitrate the line cannot carry.
///
/// No size of its own.
/// Nothing is measured here, so the height comes from the host and the width from the parent's arrange.
/// A zero box draws nothing.
/// Lengths are device-independent pixels, so the marker is one DIP wide at any scale factor.
///
/// No colour of its own either.
/// Every brush comes in from the caller, leaving the design system the only place one is decided (avalonia/README.md,
/// "Nothing outside Design/").
/// </summary>
public sealed class MeterBar : Control
{
    /// <summary>Share of the bar the figure occupies, 0..1. Over 1 draws as full.</summary>
    public static readonly StyledProperty<double> FillProperty =
        AvaloniaProperty.Register<MeterBar, double>(nameof(Fill));

    /// <summary>
    /// Share of the bar the limit stands at, 0..1.
    /// Zero draws no marker, and over 1 draws it inside the right edge rather than off the end.
    /// </summary>
    public static readonly StyledProperty<double> LimitProperty =
        AvaloniaProperty.Register<MeterBar, double>(nameof(Limit));

    public static readonly StyledProperty<IBrush?> TrackBrushProperty =
        AvaloniaProperty.Register<MeterBar, IBrush?>(nameof(TrackBrush));

    public static readonly StyledProperty<IBrush?> FillBrushProperty =
        AvaloniaProperty.Register<MeterBar, IBrush?>(nameof(FillBrush));

    public static readonly StyledProperty<IBrush?> LimitBrushProperty =
        AvaloniaProperty.Register<MeterBar, IBrush?>(nameof(LimitBrush));

    /// <summary>Track thickness, below the control's height so the marker overhangs it.
    /// Capped at that height.</summary>
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
    /// The one draw pass, back to front: track, fill, marker.
    /// The marker runs the control's full height, over both, so it reads as a limit rather than part of the fill.
    /// </summary>
    public override void Render(DrawingContext context)
    {
        Assert.That(TrackHeight > 0, "a bar has a track to draw", TrackHeight);

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
            // Clipped to the track's shape: a short fill keeps the rounded left cap and gains no right one.
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
    }
}
