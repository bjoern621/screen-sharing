using Avalonia.Controls;

namespace ScreenShare.App.Features.Broadcast.View;

/// <summary>
/// Markup only.
/// Every control here binds to a command or a rendered value, so a handler would be a second definition of the
/// screen and <c>BroadcastViewModel.Apply</c> would no longer restore it.
/// No window, no title bar and no nav strip: the shell supplies those and this is the body inside them.
/// </summary>
public sealed partial class BroadcastView : UserControl
{
    public BroadcastView() => InitializeComponent();
}
