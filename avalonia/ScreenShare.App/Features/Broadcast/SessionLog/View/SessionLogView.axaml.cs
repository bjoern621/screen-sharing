using Avalonia.Controls;

namespace ScreenShare.App.Features.Broadcast.SessionLog.View;

/// <summary>
/// Markup only.
/// The one control here asks through a command, so this card opens no surface itself.
/// </summary>
public sealed partial class SessionLogView : UserControl
{
    public SessionLogView() => InitializeComponent();
}
