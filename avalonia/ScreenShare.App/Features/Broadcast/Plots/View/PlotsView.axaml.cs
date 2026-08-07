using Avalonia.Controls;

namespace ScreenShare.App.Features.Broadcast.Plots.View;

/// <summary>
/// Markup and nothing else. The drawing lives in <see cref="Sparkline"/>, which reads its
/// samples from a binding rather than from anything a handler here would have to push.
/// </summary>
public sealed partial class PlotsView : UserControl
{
    public PlotsView() => InitializeComponent();
}
