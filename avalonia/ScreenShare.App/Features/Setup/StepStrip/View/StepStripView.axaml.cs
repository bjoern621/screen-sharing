using Avalonia.Controls;

namespace ScreenShare.App.Features.Setup.StepStrip.View;

/// <summary>
/// Markup and nothing else.
/// A chip's press runs the row's command rather than a handler here,
/// so the render function alone still restores a correct strip.
/// </summary>
public sealed partial class StepStripView : UserControl
{
    public StepStripView() => InitializeComponent();
}
