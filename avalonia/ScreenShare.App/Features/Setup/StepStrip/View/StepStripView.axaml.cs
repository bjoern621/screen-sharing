using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.StepStrip.View;

/// <summary>
/// Markup and nothing else.
/// A chip's click is a command on the row, not a handler here: a handler that moved the step itself would
/// mean the render function alone could no longer restore a correct strip.
/// </summary>
public sealed partial class StepStripView : UserControl
{
    public StepStripView() => InitializeComponent();
}
