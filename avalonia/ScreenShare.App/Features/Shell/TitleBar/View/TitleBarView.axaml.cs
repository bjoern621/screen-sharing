using Avalonia.Controls;

namespace ScreenShare.App.Features.Shell.TitleBar.View;

/// <summary>
/// Markup and nothing else.
/// The window controls reach the window through commands the shell attached, not through a handler here: a
/// title bar that called <c>Close</c> itself would be a second definition of what the chrome does.
/// </summary>
public sealed partial class TitleBarView : UserControl
{
    public TitleBarView() => InitializeComponent();
}
