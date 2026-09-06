using Avalonia.Controls;
using Avalonia.Interactivity;
using ScreenShare.App.Features.Shell.Update.ViewModel;

namespace ScreenShare.App.Features.Shell.Update.View;

/// <summary>
/// What a reader sees behind the band's update line: which release is waiting, and the restart.
///
/// Two handlers, both about this window rather than about updates.
/// Closing is the window's own act, and opening the release page is the desktop's,
/// reached through the toplevel's launcher because a view model holds no window to ask.
/// The restart is a command like every other effect here.
/// </summary>
public sealed partial class UpdateDialog : Window
{
    public UpdateDialog()
    {
        InitializeComponent();
    }

    /// <summary>Closes and leaves a staged release staged, which the next restart installs.</summary>
    private void Close(object? sender, RoutedEventArgs e) => Close();

    /// <summary>
    /// Opens the release page the backend named.
    /// Not awaited: what the browser does with the address is the browser's,
    /// and the dialog has nothing to report about it.
    /// </summary>
    private void OpenPage(object? sender, RoutedEventArgs e)
    {
        if (DataContext is UpdateViewModel updates && updates.PageUrl.Length > 0)
        {
            _ = Launcher.LaunchUriAsync(new Uri(updates.PageUrl));
        }
    }
}
