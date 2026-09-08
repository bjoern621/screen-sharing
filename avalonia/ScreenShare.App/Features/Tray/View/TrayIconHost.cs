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
/// <b>The icon comes and goes with the setting.</b> <see cref="Render"/> registers one where
/// <see cref="IsWanted"/> asks for it and takes it out where it does not, so a toggle in the settings
/// dialog lands without a restart. Both directions are idempotent, a second pass over an unchanged menu
/// registering and removing nothing.
///
/// No icon stands until the settings have been read, an icon that flashes up and goes being worse than
/// one arriving late.
///
/// A platform serving no tray is an Umgebungsfehler, and leaves <see cref="IsUp"/> false the way the
/// setting does: quit-on-close stands, a hidden window with no icon to come back through being gone.
/// </summary>
public sealed class TrayIconHost : IDisposable
{
    /// <summary>
    /// <c>0</c> keeps the icon out of the tray for a whole run, whatever the setting holds.
    ///
    /// For a desktop whose panel draws no tray, and for a checkout run beside an installed app.
    /// Everything the menu offers is the window's own, so a run without the icon loses no control.
    /// </summary>
    internal const string EnvTray = "MIRRORME_TRAY";

    private readonly TrayViewModel _tray;
    private readonly string? _env;
    private readonly WindowIcon _idle;
    private readonly WindowIcon _live;

    /// <summary>The registered icon while one stands, null while none does.</summary>
    private TrayIcon? _icon;

    private TrayIconHost(TrayViewModel tray)
    {
        _tray = tray;
        _env = Environment.GetEnvironmentVariable(EnvTray);
        _idle = new WindowIcon(AssetLoader.Open(new Uri("avares://mirrorme/Assets/tray.png")));
        _live = new WindowIcon(AssetLoader.Open(new Uri("avares://mirrorme/Assets/tray-live.png")));

        // Disposing an icon takes down the process it is registered against (AvaloniaUI/Avalonia#21979):
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

    /// <summary>Builds the host. The icon itself follows the menu.</summary>
    public static TrayIconHost Create(TrayViewModel tray)
    {
        Assert.NotNull(tray, "a tray icon draws the tray's state");

        return new TrayIconHost(tray);
    }

    /// <summary>
    /// Whether an icon stands in the tray right now, which is what makes a window close a hide.
    /// </summary>
    public bool IsUp => _icon is not null;

    /// <summary>Takes the icon out of the tray. A disposed host stays disposed.</summary>
    public void Dispose() => Remove();

    /// <summary>
    /// Whether a run wants an icon.
    /// <see cref="EnvTray"/> refuses one at <c>0</c>, and the app setting decides everywhere else.
    /// </summary>
    internal static bool IsWanted(string? environment, bool wanted) => environment != "0" && wanted;

    /// <summary>
    /// Whether a dispatcher exception is the tray watch dying of its own disposal,
    /// matched by type and by the frame that threw it.
    /// Every other exception passes through and crashes, as a broken contract should.
    /// </summary>
    internal static bool IsWatchDisposeCancellation(Exception thrown) =>
        thrown is OperationCanceledException
        && thrown.StackTrace?.Contains("DBusTrayIconImpl.WatchAsync") == true;

    /// <summary>The one render function: whether an icon stands, and what it draws, from the menu state.</summary>
    private void Render()
    {
        var menu = _tray.Menu;

        if (!IsWanted(_env, menu.IsIconWanted))
        {
            Remove();
            return;
        }

        Register();
        if (_icon is null)
        {
            return;
        }

        _icon.Icon = menu.IsLive ? _live : _idle;
        _icon.Menu = MenuOf(menu);
    }

    /// <summary>Puts an icon in the tray, where none stands. A platform serving no tray leaves none.</summary>
    private void Register()
    {
        if (_icon is not null)
        {
            return;
        }

        try
        {
            var icon = new TrayIcon { ToolTipText = TrayCopy.Tip, Icon = _idle };

            // Left-click brings the window back; the menu is the platform's right-click.
            icon.Clicked += (_, _) => _tray.Open();

            // Registered against the application, which is what holds the platform icon alive.
            TrayIcon.SetIcons(Application.Current!, new TrayIcons { icon });
            _icon = icon;
        }
        catch (Exception)
        {
            // Umgebungsfehler: a platform or a session with no tray to register against.
        }
    }

    /// <summary>
    /// Takes the icon out of the tray, where one stands.
    /// The empty set and the dispose both: which of the two drops the platform icon is Avalonia's business,
    /// and a second dispose is a no-op.
    /// </summary>
    private void Remove()
    {
        if (_icon is null)
        {
            return;
        }

        var icon = _icon;
        _icon = null;

        TrayIcon.SetIcons(Application.Current!, new TrayIcons());
        icon.Dispose();
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
