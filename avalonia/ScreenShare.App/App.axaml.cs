using Avalonia;
using Avalonia.Controls;
using Avalonia.Controls.ApplicationLifetimes;
using Avalonia.Input.Platform;
using Avalonia.Markup.Xaml;
using Avalonia.Media;
using ScreenShare.App.Contracts;
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

            // Which of maximise and restore the middle caption button draws is the window's state, not the bar's,
            // so it is written from here for the same reason the three actions are.
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

            // With a tray, closing the window keeps the app alive in it,
            // and only the tray's quit ends the process:
            // the backend keeps publishing behind a hidden window,
            // and the exit hooks take a spawned one down with the shell (Backend/BackendProcess.cs).
            // Without one, a session serving no tray, quit-on-close stands:
            // a hidden window nothing can reopen is gone.
            var tray = TrayIconHost.TryCreate(shell.Tray);
            if (tray is not null)
            {
                desktop.ShutdownMode = ShutdownMode.OnExplicitShutdown;

                window.Closing += (_, close) =>
                {
                    // The desktop taking the app down closes for real; the reader's close hides to the tray.
                    if (close.CloseReason is WindowCloseReason.ApplicationShutdown or WindowCloseReason.OSShutdown)
                    {
                        return;
                    }

                    close.Cancel = true;
                    window.Hide();
                };

                shell.Tray.OpenRequested += () =>
                {
                    window.Show();
                    if (window.WindowState == WindowState.Minimized)
                    {
                        window.WindowState = WindowState.Normal;
                    }

                    window.Activate();
                };

                // Raised once the quit's own stop attempt is over (Features/Tray/ViewModel/TrayViewModel.cs).
                shell.Tray.QuitRequested += () => desktop.Shutdown();

                desktop.Exit += (_, _) => tray.Dispose();
            }

            // One render pass before the window shows.
            // Apply is idempotent, so a second one changes nothing.
            shell.Apply();
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
