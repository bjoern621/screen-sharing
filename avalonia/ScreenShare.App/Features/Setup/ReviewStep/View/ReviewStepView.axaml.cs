using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.ReviewStep.View;

/// <summary>
/// Markup and nothing else.
/// Starting to share is a command whose enablement is the preflight list's answer, so no handler here can
/// commit past a check that has not cleared.
/// </summary>
public sealed partial class ReviewStepView : UserControl
{
    public ReviewStepView() => InitializeComponent();
}
