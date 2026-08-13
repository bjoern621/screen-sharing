using Avalonia.Controls;

namespace ScreenShare.App.Features.Broadcast.HeaderStats.View;

/// <summary>
/// Markup only.
/// A handler writing a figure here leaves <c>HeaderStatsViewModel.Apply</c> no longer the one writer of the bar.
/// </summary>
public sealed partial class HeaderStatsView : UserControl
{
    public HeaderStatsView() => InitializeComponent();
}
