using Avalonia;
using Avalonia.Controls.Primitives;

namespace ScreenShare.App.Controls;

/// <summary>
/// Sharing pill, in the red that means this machine is on air.
/// Not a general-purpose badge: a pill meaning anything but this machine sending would spend the red on state
/// that is merely on.
/// </summary>
public sealed class StatusPill : TemplatedControl
{
    /// <summary>Prose, sentence case: "Sharing".</summary>
    public static readonly StyledProperty<string> LabelProperty =
        AvaloniaProperty.Register<StatusPill, string>(nameof(Label), "");

    /// <summary>Figure beside the label: the elapsed timer, zero-padded <c>HH:MM:SS</c>.</summary>
    public static readonly StyledProperty<string> DetailProperty =
        AvaloniaProperty.Register<StatusPill, string>(nameof(Detail), "");

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
