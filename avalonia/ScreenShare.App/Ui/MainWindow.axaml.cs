using Avalonia.Controls;

namespace ScreenShare.App.Ui;

/// <summary>
/// The window is markup and nothing else. There is no code-behind handler here on
/// purpose: an event handler that set a widget directly would mean
/// <see cref="MainViewModel.Apply"/> alone could no longer restore a correct view.
/// </summary>
public sealed partial class MainWindow : Window
{
    public MainWindow() => InitializeComponent();
}
