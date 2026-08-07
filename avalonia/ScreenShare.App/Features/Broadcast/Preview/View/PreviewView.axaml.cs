using Avalonia.Controls;

namespace ScreenShare.App.Features.Broadcast.Preview.View;

/// <summary>
/// Markup and nothing else. The tile is a placeholder until a video pipeline hands it a
/// frame, and a handler here would only be a second definition of what it shows.
/// </summary>
public sealed partial class PreviewView : UserControl
{
    public PreviewView() => InitializeComponent();
}
