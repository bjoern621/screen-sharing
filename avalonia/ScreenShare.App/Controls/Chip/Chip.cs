using Avalonia;
using Avalonia.Controls.Primitives;

namespace ScreenShare.App.Controls;

/// <summary>
/// A capsule that names one stream and toggles it. The only toggle in the viewer, and a
/// performance control rather than a preference: turning a chip off tears the decoder
/// down, so the chip reports the bandwidth that frees.
///
/// It is a <see cref="ToggleButton"/> because that is exactly what it is - the checked
/// state is the stream being shown, and the struck-through unchecked skin comes from the
/// <c>:unchecked</c> pseudo-class rather than from anything a view has to remember to set.
/// </summary>
public sealed class Chip : ToggleButton
{
    /// <summary>The stream's name, as a person reads it.</summary>
    public static readonly StyledProperty<string> LabelProperty =
        AvaloniaProperty.Register<Chip, string>(nameof(Label), "");

    /// <summary>
    /// The figure that trails the label: what this stream costs, or what hiding it frees.
    /// A disabled chip drops it, because there is nothing left to measure.
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
