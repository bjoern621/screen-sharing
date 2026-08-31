using Avalonia.Controls;

namespace ScreenShare.App.Features.Broadcast.Nudge.View;

/// <summary>
/// Markup only.
/// The slider writes <c>NudgeViewModel.Sharpness</c> through its two-way binding, so the render function stays
/// the one decider of what is shown.
/// </summary>
public sealed partial class NudgeView : UserControl
{
    public NudgeView() => InitializeComponent();
}
