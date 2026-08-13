using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.ReviewStep.View;

/// <summary>
/// Markup only.
/// The commit is a command gated by what the form and the backend answered, so no handler here can start past
/// a check that has not cleared.
/// </summary>
public sealed partial class ReviewStepView : UserControl
{
    public ReviewStepView() => InitializeComponent();
}
