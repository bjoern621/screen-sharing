using Avalonia.Controls;

namespace ScreenShare.App.Features.Viewer.WatchSettings.View;

/// <summary>
/// Markup and nothing else.
/// Every control writes the one draft through its binding, and the next resolved form decides what the write
/// became.
/// </summary>
public sealed partial class WatchSettingsView : UserControl
{
    public WatchSettingsView() => InitializeComponent();
}
