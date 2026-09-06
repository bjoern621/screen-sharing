using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.ApplicationLifetimes;
using Avalonia.Input.Platform;
using Avalonia.Markup.Xaml;
using Avalonia.Media;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.Update.View;
using ScreenShare.App.Features.Shell.View;
using ScreenShare.App.Features.Shell.ViewModel;
using ScreenShare.App.Features.Tray.View;

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
        AssertMonoFaceLoads();

        if (ApplicationLifetime is IClassicDesktopStyleApplicationLifetime desktop)
        {
            var shell = new ShellViewModel();
            var window = new ShellWindow { DataContext = shell };

            // Minimise, maximise and close are the window's, so the title bar is handed them rather than holding
            // a window it should not know about.
            // A second attach changes nothing, like every other write in the shell.
            shell.TitleBar.Attach(
                minimise: () => window.WindowState = WindowState.Minimized,
                toggleMaximise: () => window.WindowState = window.WindowState == WindowState.Maximized
                    ? WindowState.Normal
                    : WindowState.Maximized,
                close: window.Close);

            // Which of maximise and restore the middle caption button draws is the window's state,
            // so it is written from here for the same reason the three actions are.
            // The first write as well as the subscription: a window can be maximised before it is ever shown,
            // by a restored session or a shell started snapped.
            window.PropertyChanged += (_, change) =>
            {
                if (change.Property == Window.WindowStateProperty)
                {
                    shell.TitleBar.ShowMaximised(window.WindowState == WindowState.Maximized);
                }

                // Whether the window is on screen is its state too, and every picture in it follows:
                // a window closed to the tray draws nothing, so the decodes behind its grid and its preview close
                // (ShellViewModel.SetWindowShown).
                if (change.Property == Visual.IsVisibleProperty)
                {
                    shell.SetWindowShown(window.IsVisible);
                }
            };
            shell.TitleBar.ShowMaximised(window.WindowState == WindowState.Maximized);
            shell.SetWindowShown(window.IsVisible);

            desktop.MainWindow = window;

            // The dialog behind the band's update line.
            // Constructed here rather than by the band, windows being this file's alone,
            // and over the same view model, so what it says and what the band says are one answer.
            // Modal: the reader's next act is the restart or dismissing it.
            shell.Update.OpenRequested += () =>
            {
                var dialog = new UpdateDialog { DataContext = shell.Update };
                _ = dialog.ShowDialog(window);
            };

            // Raised once the backend has the install under way.
            // The applier waits for this process to exit before it replaces a file,
            // so ending the app is what lets it start (backend/internal/update).
            shell.Update.RestartRequested += () => desktop.Shutdown();

            // The reader's close is the tray's hide where an icon is up, and the quit where none is:
            // a hidden window nothing can reopen is gone.
            // Both take the tray's quit, so the window's decodes close and a stream on a backend this shell
            // started ends before the process does, and the exit hooks take that backend with the shell
            // (Features/Tray/ViewModel/TrayViewModel.cs, Backend/BackendProcess.cs).
            // Shutdown is explicit either way: the close is cancelled, and the quit closes the window for real.
            var tray = TrayIconHost.TryCreate(shell.Tray);
            desktop.ShutdownMode = ShutdownMode.OnExplicitShutdown;

            window.Closing += (_, close) =>
            {
                // The desktop taking the app down closes for real.
                if (close.CloseReason is WindowCloseReason.ApplicationShutdown or WindowCloseReason.OSShutdown)
                {
                    return;
                }

                close.Cancel = true;
                if (tray is not null)
                {
                    window.Hide();
                }
                else
                {
                    shell.Tray.QuitCommand.Execute(null);
                }
            };

            // Raised once the quit's own stops are over.
            shell.Tray.QuitRequested += () => desktop.Shutdown();

            if (tray is not null)
            {
                shell.Tray.OpenRequested += () =>
                {
                    window.Show();
                    if (window.WindowState == WindowState.Minimized)
                    {
                        window.WindowState = WindowState.Normal;
                    }

                    window.Activate();
                };

                desktop.Exit += (_, _) => tray.Dispose();
            }

            // One render pass before the window shows.
            // Apply is idempotent, so a second one changes nothing.
            shell.Apply();

            // A link the desktop started this window with, which is how a stream of this app is opened from
            // outside it (<c>backend/internal/applink</c>).
            // After the pass, so the window it moves to is drawn either way, and unawaited because the answer
            // travels over the socket while the window is already up.
            var link = LaunchLink.In(desktop.Args);
            if (link.Length > 0)
            {
                _ = shell.FollowAsync(link);
            }
        }

        base.OnFrameworkInitializationCompleted();
    }

    /// <summary>
    /// The bundled mono face, before anything is drawn with it.
    /// A URI whose authority is not the assembly name, or a family name the files do not spell, loads nothing
    /// and leaves the transcript role drawing a session log in whatever face the platform hands back
    /// (Design/Typography.axaml).
    /// The substitution is invisible on screen to anyone who has not seen the intended face, so it is asserted
    /// rather than looked for.
    /// </summary>
    private void AssertMonoFaceLoads()
    {
        Resources.TryGetResource("MonoFont", ActualThemeVariant, out var declared);
        var family = Assert.NotNull(declared as FontFamily, "Design/Typography.axaml declares MonoFont as a font family");

        var loaded = FontManager.Current.TryGetGlyphTypeface(
            new Typeface(family, FontStyle.Normal, FontWeight.Medium),
            out var face);

        Assert.That(loaded, "the mono family loads out of the assets its URI names", family.Name, family.Key);
        Assert.That(
            face is { Metrics.IsFixedPitch: true },
            "the family the mono URI names is monospaced",
            face?.FamilyName);
    }
}
