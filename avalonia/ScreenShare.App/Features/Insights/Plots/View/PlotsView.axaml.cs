using Avalonia.Controls;

namespace ScreenShare.App.Features.Insights.Plots.View;

/// <summary>Markup only. <see cref="Sparkline"/> draws, and takes its samples from a binding.</summary>
public sealed partial class PlotsView : UserControl
{
    public PlotsView() => InitializeComponent();
}
