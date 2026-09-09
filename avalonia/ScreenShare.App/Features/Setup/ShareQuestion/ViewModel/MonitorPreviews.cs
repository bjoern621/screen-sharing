using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;

namespace ScreenShare.App.Features.Setup.ShareQuestion.ViewModel;

/// <summary>
/// Screens the share question has asked the backend to read, and the tiles drawing them.
///
/// <b>One owner for the whole question.</b>
/// The grid wants every screen it draws and the rectangle wants the screen it sits on,
/// so a converger per chooser would close the other's screens on every pass.
/// Each chooser names what it wants and this converges on the union.
///
/// Converged rather than sequenced, so a second pass over unchanged input opens nothing and closes nothing.
/// On the way out it closes what is running as well as what was asked for,
/// and the two differ after a shell that died with previews open: those outlive the window that asked for them,
/// as decodes do, and this is where the next shell tidies them up.
/// </summary>
public sealed class MonitorPreviews
{
    private readonly IBackend _backend;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <summary>Asks the question for another pass, a report being state no render pass produced.</summary>
    private readonly Action _changed;

    /// <summary>
    /// Screens asked of the backend, the desired state <see cref="Converge"/> is written against.
    /// Asked-for and not running: a preview that ended on its own leaves the reported set,
    /// and converging on that set would ask for it again at once, looping on a screen that cannot be read at all.
    /// </summary>
    private readonly HashSet<int> _asked = [];

    /// <summary>
    /// Why a screen was refused, in the backend's own words.
    /// Separates a picture that has not arrived from one that never will: the same empty tile, different news.
    /// </summary>
    private readonly Dictionary<int, string> _refused = [];

    /// <summary>
    /// One tile per screen, made on demand and kept across passes.
    /// Rebuilding one restarts a frame subscription already drawing that screen.
    /// </summary>
    private readonly Dictionary<int, TileViewModel> _tiles = [];

    /// <param name="dispatch">
    /// Marshals to the UI loop.
    /// A refusal lands on whichever thread the transport completed on,
    /// and every output derived from one is read by a binding written from one thread only.
    /// </param>
    public MonitorPreviews(IBackend backend, Session session, Action<Action> dispatch, Action changed)
    {
        Assert.NotNull(backend, "the previews are opened on the backend");
        Assert.NotNull(session, "the previews are read back off what the backend reports");
        Assert.NotNull(dispatch, "a refusal is marshalled back to the UI loop");
        Assert.NotNull(changed, "a report asks the question for the pass that draws it");

        _backend = backend;
        _session = session;
        _dispatch = dispatch;
        _changed = changed;
    }

    /// <summary>
    /// Opens the screens the question wants and closes the ones it does not.
    /// A screen already asked for is not asked for again,
    /// which keeps a render pass per keystroke from being a call per screen per keystroke.
    /// The effects are idempotent, so a duplicate would be wasteful rather than wrong.
    /// </summary>
    public void Converge(IReadOnlyCollection<int> wanted)
    {
        Assert.NotNull(wanted, "converging needs the screens the question wants");

        if (wanted.Count == 0)
        {
            var running = _session.PreviewedMonitors.Select(previewed => previewed.Monitor);
            foreach (var monitor in _asked.Union(running).ToList())
            {
                Close(monitor);
            }

            _asked.Clear();
            _refused.Clear();
            return;
        }

        foreach (var monitor in _asked.Where(monitor => !wanted.Contains(monitor)).ToList())
        {
            Close(monitor);
            _asked.Remove(monitor);
            _refused.Remove(monitor);
            Drop(monitor);
        }

        foreach (var monitor in wanted)
        {
            if (_asked.Add(monitor))
            {
                Open(monitor);
            }
        }
    }

    /// <summary>
    /// Tile drawing one screen, null while the backend is not reading it.
    /// Made no earlier: a subscription naming a screen nothing is reading is refused once and never retried,
    /// so a tile built ahead of the preview stays dark for as long as the question is open.
    /// </summary>
    public TileViewModel? TileOf(int monitor)
    {
        Assert.That(monitor >= 0, "a preview names an enumerated output", monitor);

        if (Previewed(monitor) is null)
        {
            return Drop(monitor);
        }

        if (_tiles.TryGetValue(monitor, out var held))
        {
            return held;
        }

        var tile = new TileViewModel(
            TileSource.MonitorPreview(monitor, _session.Words.Name("publish.monitor", monitor.ToString())),
            _backend,
            _dispatch,
            // The intents go nowhere: these tiles sit inside a question, with no focus, pop-out or fullscreen
            // to arrange.
            _ => { });

        // A tile reports what it drew, which no backend state carries:
        // a backend cannot see that a compositor was too slow to take a frame.
        tile.Changed += _changed;
        _tiles[monitor] = tile;
        return tile;
    }

    /// <summary>
    /// Why one screen has no picture, in the order the states happen in: refused with the backend's reason,
    /// or wanted and not up yet.
    /// Asked by the chooser drawing that screen, so a screen nothing wants reaches neither branch.
    /// </summary>
    public string PlaceholderFor(int monitor)
        => _refused.TryGetValue(monitor, out var refusal) ? refusal : Cards.ScreenOpening;

    /// <summary>What the backend reports about one screen's preview. Null while that screen is not being read.</summary>
    public PreviewedMonitor? Previewed(int monitor)
    {
        foreach (var previewed in _session.PreviewedMonitors)
        {
            if (previewed.Monitor == monitor)
            {
                return previewed;
            }
        }

        return null;
    }

    /// <summary>
    /// Asks the backend to read one screen.
    /// A refusal reaches no caller: it says this machine cannot show that screen,
    /// and the tile beside it already says nothing is reading it.
    /// </summary>
    private async void Open(int monitor)
    {
        try
        {
            await _backend.StartMonitorPreviewAsync(monitor).ConfigureAwait(false);
        }
        catch (BackendUnavailableException e)
        {
            // Held as the backend wrote it, what refuses a screen being a fact about this machine.
            // A tile left opening for a picture that will never arrive is the one thing a chooser must not say.
            _dispatch(() =>
            {
                _refused[monitor] = e.Message;
                _changed();
            });
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Asks the backend to stop reading one screen.
    /// A screen nothing is reading is not an error, so the one failure left is a backend that has gone,
    /// taking the previews with it.
    /// </summary>
    private async void Close(int monitor)
    {
        try
        {
            await _backend.StopMonitorPreviewAsync(monitor).ConfigureAwait(false);
        }
        catch (BackendUnavailableException)
        {
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Lets go of one screen's tile and answers null, so a caller drops the picture in the same expression.
    /// The frame subscription belongs to the control and ends when the tile leaves the tree.
    /// What goes here is the view model behind it,
    /// so a screen that comes back is drawn by a tile subscribing afresh rather than by one holding a dead channel.
    /// </summary>
    private TileViewModel? Drop(int monitor)
    {
        if (_tiles.Remove(monitor, out var tile))
        {
            tile.Changed -= _changed;
        }

        return null;
    }
}
