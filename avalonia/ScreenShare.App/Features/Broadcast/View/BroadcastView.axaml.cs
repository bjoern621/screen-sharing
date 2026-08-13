using Avalonia.Controls;

namespace ScreenShare.App.Features.Broadcast.View;

/// <summary>
/// Markup and nothing else.
/// Every control on this screen is bound to a command or a rendered value, so a handler here would be a
/// second definition of what the screen shows and <c>BroadcastViewModel.Apply</c> would no longer be enough
/// to restore it.
///
/// No window, no title bar and no nav strip: the shell supplies those, and this view is only the body it puts
/// inside them.
/// </summary>
public sealed partial class BroadcastView : UserControl
{
    public BroadcastView() => InitializeComponent();
}
