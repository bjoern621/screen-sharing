using Avalonia.Controls;

namespace ScreenShare.App.Features.Broadcast.Nudge.View;

/// <summary>
/// Markup and nothing else. The slider writes <c>NudgeViewModel.Sharpness</c> through a
/// two-way binding, so the render function stays the only thing that decides what is shown.
/// </summary>
public sealed partial class NudgeView : UserControl
{
    public NudgeView() => InitializeComponent();
}
