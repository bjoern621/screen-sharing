using Avalonia.Controls;

namespace ScreenShare.App.Features.Broadcast.ViewerTable.View;

/// <summary>
/// Markup and nothing else.
/// The table has no interaction at all: it reports, and the controls that act on what it reports live in the
/// header bar.
/// </summary>
public sealed partial class ViewerTableView : UserControl
{
    public ViewerTableView() => InitializeComponent();
}
