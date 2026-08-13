using Avalonia;
using Avalonia.Controls.Primitives;

namespace ScreenShare.App.Controls;

/// <summary>
/// A capsule naming one stream, checked while that stream is shown.
/// A performance control rather than a preference: unchecking tears the decode down, which is what
/// <see cref="Detail"/> prices.
/// </summary>
public sealed class Chip : ToggleButton
{
    /// <summary>The stream this chip names.</summary>
    public static readonly StyledProperty<string> LabelProperty =
        AvaloniaProperty.Register<Chip, string>(nameof(Label), "");

    /// <summary>
    /// The figure trailing the label: what this stream costs, or what hiding it frees.
    /// Hidden while unchecked, where there is nothing left to measure.
    /// </summary>
    public static readonly StyledProperty<string> DetailProperty =
        AvaloniaProperty.Register<Chip, string>(nameof(Detail), "");

    public string Label
    {
        get => GetValue(LabelProperty);
        set => SetValue(LabelProperty, value);
    }

    public string Detail
    {
        get => GetValue(DetailProperty);
        set => SetValue(DetailProperty, value);
    }

}
