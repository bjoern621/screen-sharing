using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.CostRail.View;

/// <summary>
/// Markup and nothing else.
/// The rail is read-only: it prices what the form chose and names what is still owed, and it edits none of
/// it.
/// </summary>
public sealed partial class CostRailView : UserControl
{
    public CostRailView() => InitializeComponent();
}
