using Avalonia.Controls;

namespace ScreenShare.App.Features.Viewer.View;

/// <summary>
/// The viewer is markup and nothing else. There is no code-behind handler here on purpose:
/// a handler that set a widget directly would mean
/// <see cref="ViewModel.ViewerViewModel.Apply"/> alone could no longer restore a correct
/// view.
/// </summary>
public sealed partial class ViewerView : UserControl
{
    public ViewerView() => InitializeComponent();
}
