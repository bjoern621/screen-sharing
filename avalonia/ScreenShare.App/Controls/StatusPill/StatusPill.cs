using Avalonia;
using Avalonia.Controls.Primitives;

namespace ScreenShare.App.Controls;

/// <summary>
/// The sharing pill: a solid white dot, a label, and a running figure. It carries the one
/// red in the palette, so it is the single element on any screen that says the world is
/// being changed right now.
///
/// Not a general-purpose badge. A pill that meant anything other than "sharing" would
/// spend the red on state that is merely on.
/// </summary>
public sealed class StatusPill : TemplatedControl
{
    /// <summary>Prose, sentence case. In practice "Sharing".</summary>
    public static readonly StyledProperty<string> LabelProperty =
        AvaloniaProperty.Register<StatusPill, string>(nameof(Label), "");

    /// <summary>
    /// The figure beside the label, set in tabular figures because it ticks: the elapsed
    /// timer, zero-padded <c>HH:MM:SS</c>.
    /// </summary>
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
