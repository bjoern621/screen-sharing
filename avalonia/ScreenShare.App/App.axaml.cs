using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.ApplicationLifetimes;
using Avalonia.Input.Platform;
using Avalonia.Markup.Xaml;
using ScreenShare.App.Features.Shell.View;
using ScreenShare.App.Features.Shell.ViewModel;

namespace ScreenShare.App;

/// <summary>
/// Composition root.
/// Everything is constructed here and handed down, so no component reaches for a global and each can be built
/// with a stub in a test.
/// Windows are constructed nowhere else.
/// </summary>
public sealed partial class App : Application
{
    public override void Initialize() => AvaloniaXamlLoader.Load(this);

    public override void OnFrameworkInitializationCompleted()
    {
        if (ApplicationLifetime is IClassicDesktopStyleApplicationLifetime desktop)
        {
            var shell = new ShellViewModel();
            var window = new ShellWindow { DataContext = shell };

            // Minimise, maximise and close are the window's, so the title bar is handed them rather than
            // holding a window it should not know about.
            // A second attach changes nothing, like every other write in the shell.
            shell.TitleBar.Attach(
                minimise: () => window.WindowState = WindowState.Minimized,
                toggleMaximise: () => window.WindowState = window.WindowState == WindowState.Maximized
                    ? WindowState.Normal
                    : WindowState.Maximized,
                close: window.Close);

            // Which of maximise and restore the middle caption button draws is the window's state rather than
            // the bar's, so it is written from here for the same reason the three actions are.
            // The first write as well as the subscription: a window can be maximised before it is ever shown,
            // by a restored session or a shell started snapped.
            window.PropertyChanged += (_, change) =>
            {
                if (change.Property == Window.WindowStateProperty)
                {
                    shell.TitleBar.ShowMaximised(window.WindowState == WindowState.Maximized);
                }
            };
            shell.TitleBar.ShowMaximised(window.WindowState == WindowState.Maximized);

            desktop.MainWindow = window;

            // One render pass before the window shows.
            // Apply is idempotent, so a second one changes nothing.
            shell.Apply();
        }

        base.OnFrameworkInitializationCompleted();
    }
}
