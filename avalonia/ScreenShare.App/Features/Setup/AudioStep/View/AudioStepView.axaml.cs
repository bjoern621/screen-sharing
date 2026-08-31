using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.AudioStep.View;

/// <summary>
/// Markup only: the rows and the dropdowns write the view model's inputs through bindings,
/// and its render function decides the rest.
/// </summary>
public sealed partial class AudioStepView : UserControl
{
    public AudioStepView() => InitializeComponent();
}
