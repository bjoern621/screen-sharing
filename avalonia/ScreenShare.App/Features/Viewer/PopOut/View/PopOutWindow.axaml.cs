using Avalonia.Controls;
using Avalonia.Markup.Xaml;

using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;

namespace ScreenShare.App.Features.Viewer.PopOut.View;

/// <summary>
/// One stream, in a window of its own.
///
/// Fullscreen here is a property of this window, where the main window's fullscreen names a stream, so one
/// stream fills one monitor while another fills a second.
/// That is why fullscreen is no member of <c>LayoutMode</c>.
///
/// Closing returns the stream to its slot in the grid and stops no decode: stopping is the rail's toggle.
/// </summary>
public partial class PopOutWindow : Window
{
    public PopOutWindow()
    {
        InitializeComponent();
    }

    public PopOutWindow(TileViewModel tile)
        : this()
    {
        Assert.NotNull(tile, "a pop-out window draws one tile");

        DataContext = tile;
        _tile = tile;

        // Opened at the stream's own shape, so a 21:9 capture does not arrive letterboxed into a 16:9 window
        // nobody asked for.
        Height = Math.Round(Width / tile.Aspect) + 1;
    }

    private readonly TileViewModel? _tile;

    /// <summary>
    /// Keeps this window out of the captures this machine takes, on the main window's terms
    /// (<c>Features/Shell/Model/CaptureExclusion.cs</c>).
    /// A window drawing a decode of the screen it sits on otherwise nests one more copy of itself in the
    /// picture on every round trip.
    /// </summary>
    protected override void OnOpened(EventArgs e)
    {
        base.OnOpened(e);

        CaptureExclusions.ForThisSystem().Exclude(this);
    }

    /// <summary>
    /// State to give this window back when it stops filling a screen, and null while nothing has filled it.
    ///
    /// Remembered rather than assumed, so a window that was maximised does not come back as a normal one.
    /// Null is also what leaves a fullscreen this app did not ask for alone: a desktop can fill a window
    /// itself, and a pass reading the window state as its own would take that back.
    /// </summary>
    private WindowState? _restore;

    /// <summary>
    /// Fills a screen with this window, or gives it back the state it was in.
    ///
    /// Called by the pass reconciling windows against the arrangement, and idempotent like the rest of it:
    /// asking for the state the window is already in changes nothing.
    /// Which monitor is filled is not decided here, a fullscreen window filling the screen it is already on.
    /// </summary>
    public void SetFullscreen(bool fullscreen)
    {
        if (fullscreen)
        {
            if (_restore is not null)
            {
                return;
            }

            _restore = WindowState;
            WindowState = WindowState.FullScreen;
            return;
        }

        if (_restore is not { } restore)
        {
            return;
        }

        _restore = null;
        WindowState = restore;
    }

    /// <summary>Stream this window draws, and empty on the parameterless constructor the XAML loader
    /// uses.</summary>
    public string Stream => _tile?.Name ?? "";

    private void InitializeComponent() => AvaloniaXamlLoader.Load(this);
}
