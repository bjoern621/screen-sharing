using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.QualityStep.View;

/// <summary>
/// Markup only: the cards and the slider write the view model's inputs through bindings,
/// and its render function decides the rest.
/// </summary>
public sealed partial class QualityStepView : UserControl
{
    public QualityStepView() => InitializeComponent();
}
