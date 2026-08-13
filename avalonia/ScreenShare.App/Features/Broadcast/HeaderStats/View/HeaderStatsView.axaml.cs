using Avalonia.Controls;

namespace ScreenShare.App.Features.Broadcast.HeaderStats.View;

/// <summary>
/// Markup and nothing else.
/// A handler here that set a figure directly would mean <c>HeaderStatsViewModel.Apply</c> alone could no
/// longer restore a correct bar.
/// </summary>
public sealed partial class HeaderStatsView : UserControl
{
    public HeaderStatsView() => InitializeComponent();
}
