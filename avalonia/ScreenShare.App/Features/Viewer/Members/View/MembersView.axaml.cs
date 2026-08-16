using Avalonia.Controls;

namespace ScreenShare.App.Features.Viewer.Members.View;

/// <summary>
/// Markup only.
/// What the two buttons do is the view model's, and no row affords anything: a group is left by the machine in
/// it and by nobody else.
/// </summary>
public sealed partial class MembersView : UserControl
{
    public MembersView() => InitializeComponent();
}
