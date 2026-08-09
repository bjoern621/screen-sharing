using Avalonia.Controls;

using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.PopOut.View;
using ScreenShare.App.Features.Viewer.ViewModel;

namespace ScreenShare.App.Features.Viewer.View;

/// <summary>
/// The viewer is markup and one thing markup cannot do: open a window.
///
/// Everything a binding can express is bound, for the reason it always was - a handler that set a
/// widget directly would mean <see cref="ViewerViewModel.Apply"/> alone could no longer restore a
/// correct view. Windows are the exception because nothing binds a window into existence, so what
/// is here is a reconciling pass: the view model names the streams that should be in windows of
/// their own, and this opens and closes windows until that is true.
///
/// <b>It is an apply and not a sequence of events.</b> The pass reads the wanted set and the open
/// set and acts on the difference, so running it twice with the same state opens nothing, closes
/// nothing and moves nothing - which is what lets it be raised on every render rather than only
/// on the passes somebody believed had changed something.
/// </summary>
public sealed partial class ViewerView : UserControl
{
    /// <summary>The windows this view has open, by the stream each draws.</summary>
    private readonly Dictionary<string, PopOutWindow> _windows = [];

    private ViewerViewModel? _viewer;

    public ViewerView()
    {
        InitializeComponent();

        DataContextChanged += (_, _) => Bind();
        DetachedFromVisualTree += (_, _) => CloseAll();
    }

    /// <summary>Follows the view model whose arrangement the windows are reconciled against.</summary>
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
    /// Opens, closes and re-states the windows until they match what the view model says.
    ///
    /// The main window's fullscreen is set here too, and for the same reason: a window state is
    /// not a bindable property of anything in this tree.
    /// </summary>
    private void Sync()
    {
        if (_viewer is null)
        {
            CloseAll();
            return;
        }

        // Closed first, so a stream that popped out and back in the same pass ends with the one
        // window it should have rather than with the old one still open beside a new one.
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

            // A window the reader closed is a stream returning to the grid, not a stream being
            // stopped. It is reported back rather than acted on here, so the arrangement stays
            // the view model's and this pass keeps only its own bookkeeping.
            window.Closed += (_, _) =>
            {
                _windows.Remove(stream);
                tile.TogglePopOut.Execute(null);
            };

            _windows[stream] = window;
            window.Show();
        }

        foreach (var (stream, window) in _windows)
        {
            window.SetFullscreen(_viewer.PoppedFullscreen.Contains(stream));
        }

        // The main window fills the screen when a tile in the grid was asked to. The tile itself
        // is drawn by a binding; only the window state is set here.
        if (TopLevel.GetTopLevel(this) is Window main)
        {
            var wanted = _viewer.HasFullscreen ? WindowState.FullScreen : WindowState.Normal;
            if (main.WindowState != wanted && (main.WindowState != WindowState.Maximized || _viewer.HasFullscreen))
            {
                main.WindowState = wanted;
            }
        }

        Assert.That(_windows.Count == _viewer.PoppedOut.Count(stream => _viewer.TileOf(stream) is not null),
            "a window per popped-out stream", _windows.Count, _viewer.PoppedOut.Count);
    }

    /// <summary>Closes one window without reporting it back: this side is the one that asked.</summary>
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
    ///
    /// A pop-out is a window this screen owns, so a screen that is gone owns none: leaving them
    /// behind would be windows holding frame subscriptions that nothing is left to close.
    /// </summary>
    private void CloseAll()
    {
        foreach (var stream in _windows.Keys.ToList())
        {
            Close(stream);
        }
    }
}
