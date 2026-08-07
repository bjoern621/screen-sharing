using Avalonia.Controls;

namespace ScreenShare.App.Features.Broadcast.SessionLog.View;

/// <summary>
/// Markup and nothing else. The one control here asks for another surface through a
/// command, so this card never opens anything itself.
/// </summary>
public sealed partial class SessionLogView : UserControl
{
    public SessionLogView() => InitializeComponent();
}
