using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.AdvancedDrawer.View;

/// <summary>
/// Markup and nothing else. Opening the drawer and resetting the table are commands on the
/// view model, so the render function alone still decides what the drawer looks like.
/// </summary>
public sealed partial class AdvancedDrawerView : UserControl
{
    public AdvancedDrawerView() => InitializeComponent();
}
