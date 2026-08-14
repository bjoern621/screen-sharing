using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.ReviewStep.View;

/// <summary>
/// Markup only.
/// Every tile's Edit is a command the flow handed down, so nothing here decides where a press lands.
/// </summary>
public sealed partial class ReviewStepView : UserControl
{
    public ReviewStepView() => InitializeComponent();
}
