using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.QualityStep.View;

/// <summary>
/// Markup and nothing else.
/// The card list and the slider write the view model's two inputs through their bindings, and the view
/// model's render function decides everything else.
/// </summary>
public sealed partial class QualityStepView : UserControl
{
    public QualityStepView() => InitializeComponent();
}
