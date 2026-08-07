using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.ApplicationLifetimes;
using Avalonia.Input.Platform;
using Avalonia.Markup.Xaml;
using ScreenShare.App.Features.Shell.View;
using ScreenShare.App.Features.Shell.ViewModel;

namespace ScreenShare.App;

/// <summary>
/// The composition root. Everything is constructed here and handed down, so no component
/// reaches for a global and every one of them can be built with a stub in a test.
///
/// The shell is the whole surface: it owns the window chrome and the nav strip, and the
/// three destinations hang off it. Nothing else constructs a window.
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

            // Minimise, maximise and close are the platform's, not the app's, so the title
            // bar is handed them here rather than reaching for a window it should not know
            // about. Attaching is idempotent, like every other write in the shell.
            shell.TitleBar.Attach(
                minimise: () => window.WindowState = WindowState.Minimized,
                toggleMaximise: () => window.WindowState = window.WindowState == WindowState.Maximized
                    ? WindowState.Normal
                    : WindowState.Maximized,
                close: window.Close);

            // The middle caption button draws maximise or restore, and which one is the
            // window's state rather than the bar's. It is written here for the same reason
            // the three actions are: the state belongs to the window, and a title bar that
            // read it off a window it holds would be holding one.
            //
            // Both the subscription and the first write, because a window can be maximised
            // before it is ever shown - a restored session, or the shell started snapped.
            window.PropertyChanged += (_, change) =>
            {
                if (change.Property == Window.WindowStateProperty)
                {
                    shell.TitleBar.ShowMaximised(window.WindowState == WindowState.Maximized);
                }
            };
            shell.TitleBar.ShowMaximised(window.WindowState == WindowState.Maximized);

            // The viewer used to be handed two of the window's own capabilities here - a window
            // per popped-out tile, and the clipboard. Both belonged to the tile grid, which is
            // gone: how a viewer arranges what it receives is this module's job and not one the
            // backend describes, and nothing renders a frame yet to arrange (avalonia/README.md,
            // "What is not settled yet"). They come back with the surface that needs them.
            desktop.MainWindow = window;

            // The same call the render function makes, so starting an already-started
            // shell changes nothing.
            shell.Apply();
        }

        base.OnFrameworkInitializationCompleted();
    }
}
