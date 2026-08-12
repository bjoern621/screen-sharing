using Avalonia.Controls;
using Avalonia.Markup.Xaml;

using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;

namespace ScreenShare.App.Features.Viewer.PopOut.View;

/// <summary>
/// The window one popped-out stream is drawn in.
///
/// <b>Its fullscreen is its own.</b> The main window's fullscreen names a stream; this window's
/// is a property of the window, so a reader can put one stream on one monitor and another on a
/// second and fill both. That is the whole reason fullscreen is not a member of the layout mode.
///
/// Closing it returns the stream to its slot in the grid and never stops the decode. Stopping is
/// the rail's toggle and means something a reader would have to undo differently.
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

        // The window opens at the stream's own shape rather than at a fixed rectangle, so a
        // 21:9 capture does not arrive letterboxed into a 16:9 window nobody asked for.
        Height = Math.Round(Width / tile.Aspect) + 1;
    }

    private readonly TileViewModel? _tile;

    /// <summary>
    /// Takes this window out of the captures this machine produces, on the same terms as the
    /// main one (<c>Features/Shell/Model/CaptureExclusion.cs</c>). A popped-out stream is a
    /// window drawing a capture of the screen it is on, which is where the picture would
    /// otherwise nest a copy of itself on every round trip.
    /// </summary>
    protected override void OnOpened(EventArgs e)
    {
        base.OnOpened(e);

        CaptureExclusions.ForThisSystem().Exclude(this);
    }

    /// <summary>
    /// The state this window is to be given back when it stops filling a screen, and none while
    /// nothing has filled it.
    ///
    /// It is remembered rather than assumed, because a window that was maximised before the stream
    /// filled it would otherwise come back as a normal one - a state the reader never asked for.
    /// It is also what leaves a fullscreen alone that this app did not ask for: a desktop can fill
    /// a window itself, and a pass that read the window state as its own would take that back.
    /// </summary>
    private WindowState? _restore;

    /// <summary>
    /// Puts this window fullscreen, or gives it back the state it was in.
    ///
    /// Written by the pass that reconciles windows against the arrangement, and idempotent like
    /// the rest of it: asking for the state the window is already in changes nothing. Which
    /// monitor it fills is not decided here - a fullscreen window fills the screen it is already
    /// on, so the reader places the window and fullscreen follows.
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

    /// <summary>The stream this window draws, so a reconciling pass can tell its windows apart.</summary>
    public string Stream => _tile?.Name ?? "";

    private void InitializeComponent() => AvaloniaXamlLoader.Load(this);
}
