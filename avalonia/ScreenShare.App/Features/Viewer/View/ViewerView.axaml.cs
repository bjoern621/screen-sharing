using Avalonia.Controls;

using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.PopOut.View;
using ScreenShare.App.Features.Viewer.ViewModel;

namespace ScreenShare.App.Features.Viewer.View;

/// <summary>
/// Markup, and the one thing markup cannot do: open a window.
///
/// Everything a binding can express is bound, a handler that set a widget directly leaving
/// <see cref="ViewerViewModel.Apply"/> unable to restore a correct view on its own.
/// Nothing binds a window into existence, so windows are reconciled here instead:
/// the view model names the streams belonging in windows of their own,
/// and this opens and closes windows until that holds.
///
/// An apply rather than a sequence of events.
/// The pass reads the wanted set against the open set and acts on the difference, so a second run over unchanged
/// state opens, closes and moves nothing, which lets every render raise it.
/// </summary>
public sealed partial class ViewerView : UserControl
{
    /// <summary>
    /// Keyed by the stream each window draws.
    /// The one piece of bookkeeping the pass owns.
    /// </summary>
    private readonly Dictionary<string, PopOutWindow> _windows = [];

    private ViewerViewModel? _viewer;

    /// <summary>
    /// Window a stream has taken fullscreen and the state to give it back, null while no stream has.
    ///
    /// The state is kept so a window maximised before a stream filled it does not come back normal,
    /// and the window because a detach hands it back long after this control can look one up.
    ///
    /// Presence also leaves a fullscreen alone that this app did not ask for: a desktop can fill a window itself,
    /// and a pass reading the window state as its own would take that back on the next render.
    /// </summary>
    private (Window Window, WindowState State)? _filled;

    public ViewerView()
    {
        InitializeComponent();

        DataContextChanged += (_, _) => Bind();
        DetachedFromVisualTree += (_, _) =>
        {
            CloseAll();
            Release();
        };
    }

    /// <summary>Follows the view model the windows are reconciled against, and reconciles once.</summary>
    private void Bind()
    {
        if (_viewer is not null)
        {
            _viewer.WindowsChanged -= Sync;
        }

        _viewer = DataContext as ViewerViewModel;
        if (_viewer is not null)
        {
            _viewer.WindowsChanged += Sync;
        }

        Sync();
    }

    /// <summary>
    /// Opens, closes and re-states windows until they match what the view model names.
    /// The main window's fullscreen is set here too, no window state in this tree being a bindable property.
    /// </summary>
    private void Sync()
    {
        if (_viewer is null)
        {
            CloseAll();
            Release();
            return;
        }

        // Closed before opened, so a stream that popped out and back within one pass ends with one window.
        foreach (var stream in _windows.Keys.Where(stream => !_viewer.PoppedOut.Contains(stream)).ToList())
        {
            Close(stream);
        }

        foreach (var stream in _viewer.PoppedOut)
        {
            if (_windows.ContainsKey(stream) || _viewer.TileOf(stream) is not { } tile)
            {
                continue;
            }

            var window = new PopOutWindow(tile);

            // A closed window is a stream returning to the grid, never a stream being stopped,
            // reported back rather than acted on so the arrangement stays the view model's.
            // A state and not a toggle: every close runs this, the closes this pass performs included,
            // and a toggle raised from there would pop the stream out into a window the next pass opens.
            window.Closed += (_, _) =>
            {
                _windows.Remove(stream);
                tile.LeavePopOut.Execute(null);
            };

            _windows[stream] = window;
            window.Show();
        }

        foreach (var (stream, window) in _windows)
        {
            window.SetFullscreen(_viewer.PoppedFullscreen.Contains(stream));
        }

        // The main window fills a screen where a tile in its grid was asked to.
        // Only the window state is set here.
        // The stream drawn over the grid, and the shell's bands coming off the window, are both bindings.
        if (_viewer.HasFullscreen && TopLevel.GetTopLevel(this) is Window main)
        {
            Fill(main);
        }
        else if (!_viewer.HasFullscreen)
        {
            Release();
        }

        Assert.That(_windows.Count == _viewer.PoppedOut.Count(stream => _viewer.TileOf(stream) is not null),
            "a window per popped-out stream", _windows.Count, _viewer.PoppedOut.Count);
    }

    /// <summary>
    /// Gives one window to the stream filling it.
    /// Idempotent: a window already filled is left as it is, so the render behind every level, every relay
    /// snapshot and every hover does not re-state a window state the platform would animate again.
    /// </summary>
    private void Fill(Window window)
    {
        if (_filled is not null)
        {
            return;
        }

        _filled = (window, window.WindowState);
        window.WindowState = WindowState.FullScreen;
    }

    /// <summary>
    /// Gives a filled window back to the state it was in, and does nothing where this view filled none.
    /// A fullscreen this app did not ask for is somebody else's to end.
    /// </summary>
    private void Release()
    {
        if (_filled is not { } filled)
        {
            return;
        }

        _filled = null;
        filled.Window.WindowState = filled.State;
    }

    /// <summary>Closes one window without reporting it back, this side being the one that asked.</summary>
    private void Close(string stream)
    {
        if (!_windows.Remove(stream, out var window))
        {
            return;
        }

        window.Close();
    }

    /// <summary>
    /// Closes every window this view opened.
    /// A pop-out belongs to this screen, so a screen that is gone owns none: a window left behind holds a frame
    /// subscription, and its slots of the lent pool, with nothing left to close it.
    /// </summary>
    private void CloseAll()
    {
        foreach (var stream in _windows.Keys.ToList())
        {
            Close(stream);
        }
    }
}
