using Avalonia;
using Avalonia.Controls;
using Avalonia.Platform;
using Avalonia.Threading;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Tray.Model;
using ScreenShare.App.Features.Tray.ViewModel;

namespace ScreenShare.App.Features.Tray.View;

/// <summary>
/// The tray icon as the platform draws it, rendered whole from <see cref="TrayViewModel.Menu"/>.
/// Code-behind for the reason the pop-out pass is: nothing binds a tray icon into existence.
///
/// <see cref="TryCreate"/> answers null where the platform serves no tray, an Umgebungsfehler the caller
/// reads as keep-the-quit-on-close-lifetime: a hidden window with no icon to come back through is gone.
/// </summary>
public sealed class TrayIconHost : IDisposable
{
    private readonly TrayViewModel _tray;
    private readonly TrayIcon _icon;
    private readonly WindowIcon _idle;
    private readonly WindowIcon _live;

    private TrayIconHost(TrayViewModel tray)
    {
        _tray = tray;
        _idle = new WindowIcon(AssetLoader.Open(new Uri("avares://mirrorme/Assets/tray.png")));
        _live = new WindowIcon(AssetLoader.Open(new Uri("avares://mirrorme/Assets/tray-live.png")));

        _icon = new TrayIcon { ToolTipText = TrayCopy.Tip, Icon = _idle };

        // Left-click brings the window back; the menu is the platform's right-click.
        _icon.Clicked += (_, _) => _tray.Open();

        // Registered against the application, which is what holds the platform icon alive.
        TrayIcon.SetIcons(Application.Current!, new TrayIcons { _icon });

        // Disposing the icon on quit takes down the process it is quitting (AvaloniaUI/Avalonia#21979):
        // the DBus watch loop's cancellation escapes an async void into the dispatcher.
        // Marked handled here, at the host owning the icon whose dispose arms it, until upstream catches it.
        Dispatcher.UIThread.UnhandledException += (_, thrown) =>
        {
            if (IsWatchDisposeCancellation(thrown.Exception))
            {
                thrown.Handled = true;
            }
        };

        // The menu compares by content, so each notification is a real change and each change one rebuild.
        tray.PropertyChanged += (_, changed) =>
        {
            if (changed.PropertyName == nameof(TrayViewModel.Menu))
            {
                Render();
            }
        };

        Render();
    }

    /// <summary>Puts the icon in the tray, or answers null on a platform serving none.</summary>
    public static TrayIconHost? TryCreate(TrayViewModel tray)
    {
        Assert.NotNull(tray, "a tray icon draws the tray's state");

        try
        {
            return new TrayIconHost(tray);
        }
        catch (Exception)
        {
            // Umgebungsfehler: a platform or a session with no tray to register against.
            return null;
        }
    }

    /// <summary>Takes the icon out of the tray. A disposed host stays disposed.</summary>
    public void Dispose() => _icon.Dispose();

    /// <summary>
    /// Whether a dispatcher exception is the tray watch dying of its own disposal,
    /// matched by type and by the frame that threw it.
    /// Every other exception passes through and crashes, as a broken contract should.
    /// </summary>
    internal static bool IsWatchDisposeCancellation(Exception thrown) =>
        thrown is OperationCanceledException
        && thrown.StackTrace?.Contains("DBusTrayIconImpl.WatchAsync") == true;

    /// <summary>The one render function: icon and menu, whole, from the menu state.</summary>
    private void Render()
    {
        var menu = _tray.Menu;

        _icon.Icon = menu.IsLive ? _live : _idle;
        _icon.Menu = MenuOf(menu);
    }

    private NativeMenu MenuOf(TrayMenu menu)
    {
        var built = new NativeMenu();

        var commit = new NativeMenuItem(menu.CommitLabel) { IsEnabled = menu.CanCommit };
        commit.Click += (_, _) => _tray.Commit();
        built.Items.Add(commit);

        if (menu.Presets.Count > 0)
        {
            built.Items.Add(new NativeMenuItem(TrayCopy.Presets) { Menu = PresetsOf(menu.Presets) });
        }

        built.Items.Add(new NativeMenuItemSeparator());

        var open = new NativeMenuItem(TrayCopy.Open);
        open.Click += (_, _) => _tray.Open();
        built.Items.Add(open);

        var quit = new NativeMenuItem(TrayCopy.Quit) { Command = _tray.QuitCommand };
        built.Items.Add(quit);

        return built;
    }

    /// <summary>A radio row per preset, the built-in half parted from the saved one as the card parts them.</summary>
    private NativeMenu PresetsOf(IReadOnlyList<TrayPresetEntry> entries)
    {
        var built = new NativeMenu();

        for (var at = 0; at < entries.Count; at++)
        {
            var entry = entries[at];

            if (at > 0 && entries[at - 1].Kind != entry.Kind)
            {
                built.Items.Add(new NativeMenuItemSeparator());
            }

            var row = new NativeMenuItem(entry.Name)
            {
                ToggleType = MenuItemToggleType.Radio,
                IsChecked = entry.IsCurrent,
                IsEnabled = entry.IsReachable,
            };
            row.Click += (_, _) => _tray.UsePreset(entry);
            built.Items.Add(row);
        }

        return built;
    }
}
