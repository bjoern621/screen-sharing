using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.AdvancedDrawer.View;

/// <summary>
/// Markup only.
/// Opening the drawer is a command on the view model, so the render function alone decides what it looks like.
/// </summary>
public sealed partial class AdvancedDrawerView : UserControl
{
    public AdvancedDrawerView() => InitializeComponent();
}
