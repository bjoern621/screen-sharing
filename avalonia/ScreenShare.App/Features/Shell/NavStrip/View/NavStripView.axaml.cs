using Avalonia.Controls;

namespace ScreenShare.App.Features.Shell.NavStrip.View;

/// <summary>
/// Markup and nothing else.
/// A destination is chosen by writing the strip's selection, which the view model forwards to the shell; a
/// handler here that swapped the body directly would leave the strip and the body with two definitions of
/// where the window is.
/// </summary>
public sealed partial class NavStripView : UserControl
{
    public NavStripView() => InitializeComponent();
}
