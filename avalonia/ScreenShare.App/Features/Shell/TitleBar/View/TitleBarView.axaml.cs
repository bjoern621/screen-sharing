using Avalonia.Controls;

namespace ScreenShare.App.Features.Shell.TitleBar.View;

/// <summary>
/// Markup and nothing else.
/// The window controls reach the window through commands the shell attached.
/// A handler here calling <c>Close</c> would be a second definition of what the chrome does.
/// </summary>
public sealed partial class TitleBarView : UserControl
{
    public TitleBarView() => InitializeComponent();
}
