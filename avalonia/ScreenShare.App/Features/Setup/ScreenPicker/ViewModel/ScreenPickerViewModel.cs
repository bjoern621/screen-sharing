using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Setup.ScreenPicker.Model;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.ScreenPicker.ViewModel;

/// <summary>
/// One live picture per screen, so a screen is picked by looking rather than by index.
/// Costs one screen capture per screen and nothing else: nothing is encoded, no bandwidth is spent, and the relay is no party to it.
/// Which outputs exist is the catalog's answer and which entries are greyed is the resolved form's, both read through on every pass (<c>docs/ipc-api.md</c>).
/// The pictures and their arrangement are what this adds.
/// The previews are the only state owned here: up while the reader stands on this step with the window in front, down as soon as either stops holding.
/// Converged rather than sequenced, so a second pass over unchanged input opens nothing and closes nothing (<see cref="Converge"/>).
/// Where no screen can be read apart from another, or the capture backend takes no monitor index, nothing is drawn and the plain control is what is left.
/// </summary>
public sealed class ScreenPickerViewModel : Observable
{
    private readonly IBackend _backend;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <summary>Writes the picked screen into the form session's draft.</summary>
    private readonly Action<int> _choose;

    /// <summary>
    /// One tile per output, made on demand and kept across passes.
    /// Rebuilding one restarts a frame subscription already drawing that screen.
    /// </summary>
    private readonly Dictionary<int, TileViewModel> _tiles = [];

    /// <summary>One command per output, made once, so an unchanged pass produces rows that compare equal and the collection is left alone.</summary>
    private readonly Dictionary<int, DelegateCommand> _select = [];

    /// <summary>
    /// The screens asked of the backend, which is the desired state <see cref="Converge"/> is written against.
    /// Asked-for and not running: a preview that ended on its own leaves the reported set,
    /// and converging on that set would ask for it again at once, which loops on a screen that cannot be read at all.
    /// A screen is asked for once per visit to the step, and one that died stays dark with the tile saying so until the reader comes back.
    /// </summary>
    private readonly HashSet<int> _asked = [];

    /// <summary>
    /// Why a screen was refused, in the backend's own words.
    /// Separates a picture that has not arrived from one that never will: the same empty tile, different news.
    /// </summary>
    private readonly Dictionary<int, string> _refused = [];

    /// <summary>Control in the tree, in a window that is in front. Written by the view.</summary>
    private bool _showing;

    /// <summary>Reader standing on the step this belongs to. Written by the flow.</summary>
    private bool _onStep;

    /// <summary><c>publish.monitor</c> as the form last resolved it. null before a form arrives.</summary>
    private Field? _field;

    /// <param name="choose">Draft write, handed in because the draft is the form session's and a second writer would be a second copy of what the settings say.</param>
    /// <param name="dispatch">Marshals to the UI loop.
    /// A tile reports on whichever thread the transport completed on, and every output here is read by a binding written from one thread only.</param>
    public ScreenPickerViewModel(IBackend backend, Session session, Action<Action> dispatch, Action<int> choose)
    {
        Assert.NotNull(backend, "a screen picker opens the backend's previews");
        Assert.NotNull(session, "a screen picker draws this machine's outputs");
        Assert.NotNull(dispatch, "a screen picker needs a UI loop to marshal a report back to");
        Assert.NotNull(choose, "a screen picker writes the screen it was told to pick");

        _backend = backend;
        _session = session;
        _dispatch = dispatch;
        _choose = choose;
    }

    // --- Outputs ------------------------------------------------------------------

    private bool _isVisible;
    private string _notice = "";
    private bool _hasNotice;

    /// <summary>One row per output, in the order the catalog enumerated them. Empty while nothing is drawn.</summary>
    public ObservableCollection<ScreenChoice> Screens { get; } = [];

    /// <summary>
    /// Whether the grid is drawn at all.
    /// False on a session that cannot read one screen apart from another, on a capture backend that takes no monitor index,
    /// and on a machine whose outputs could not be enumerated.
    /// Each of those is explained in its own words by the plain control beneath.
    /// </summary>
    public bool IsVisible { get => _isVisible; private set => Set(ref _isVisible, value); }

    /// <summary>
    /// What the pictures are, said once above the grid rather than per tile.
    /// Empty while the grid is not drawn.
    /// </summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    // --- Inputs -------------------------------------------------------------------

    /// <summary>
    /// The screen setting as the form resolved it, and which step the reader stands on.
    /// Both are the flow's to say.
    /// Neither is read off a widget.
    /// </summary>
    public void Apply(Field? monitor, bool onStep)
    {
        _field = monitor;
        _onStep = onStep;
        Render();
    }

    /// <summary>
    /// Named write of whether the grid is being looked at, and idempotent: a value it already holds renders and converges to the same world.
    /// Called by the view, since tree membership and window activation are visible to the control and the platform alone.
    /// </summary>
    public void SetShowing(bool showing)
    {
        _showing = showing;
        Render();
    }

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// The one render function.
    /// Every output is written on every pass and the previews converge afterwards, so neither a row nor a running screen capture can stick.
    /// </summary>
    public void Render()
    {
        IsVisible = Offered();

        var monitors = IsVisible ? _session.Monitors : [];
        Converge(monitors);

        var rows = new List<ScreenChoice>(monitors.Count);
        foreach (var monitor in monitors)
        {
            // Made from what the backend reports reading, and dropped with it.
            // A frame subscription naming a screen nothing is reading is refused once and never asked again,
            // so a tile built while the start was in flight sits dark for the life of the step.
            var previewed = Previewed(monitor.Index);
            var tile = previewed is null ? Drop(monitor.Index) : Tile(monitor);
            // No sample: a screen is read rather than received, so no decode holds counters about it
            // (Features/Viewer/Tile/Model/TileStats.cs).
            tile?.Apply(TilePipeline.Of(previewed), sample: null);

            var option = OptionOf(monitor.Index);
            rows.Add(new ScreenChoice(
                monitor.Index,
                _session.Words.Name("publish.monitor", monitor.Index.ToString()),
                IsSelected: Selected() == monitor.Index,
                // An output the form does not offer is one the settings cannot reach, a monitor unplugged
                // since the value was stored being the case, so the row is drawn and cannot be picked.
                // Same treatment the dropdown beneath gives that entry.
                IsEnabled: option?.Enabled ?? false,
                Reason: Statements.Of(option?.Reason),
                tile,
                Placeholder: tile is null ? PlaceholderFor(monitor.Index) : "",
                SelectCommandOf(monitor.Index)));
        }

        Reconcile.Onto(Screens, rows);

        Notice = NoticeFor();
        HasNotice = Notice.Length > 0;

        Assert.That(HasNotice == (Notice.Length > 0), "a notice and its sentence agree", HasNotice);
        Assert.That(IsVisible || Screens.Count == 0, "a hidden picker offers no screen", Screens.Count);
        Assert.That(_asked.Count == 0 || IsVisible, "a screen is read only while the grid is drawn", _asked.Count);
    }

    /// <summary>
    /// What stands above the control, in the order the states happen in, and the two are different news.
    /// A drawing grid says what its pictures are, and before that says nothing is being shared yet.
    /// A machine that could have shown pictures and cannot says why in the backend's own statement,
    /// since an absence with no reason beside it reads as a fault rather than as how the session works.
    /// The second does not wait on <see cref="_showing"/>: a sentence costs nothing to draw, a picture costs a screen capture.
    /// </summary>
    private string NoticeFor()
    {
        if (IsVisible)
        {
            return Cards.ScreenPickerCost;
        }

        if (_onStep && _session.NoMonitorPreview is not null && Editable())
        {
            return Statements.Of(_session.NoMonitorPreview);
        }

        return "";
    }

    /// <summary>
    /// Whether a screen is the reader's to pick: the form left the setting editable and something was enumerated to pick between.
    /// Both hold before the absence of pictures is worth a word, since a capture backend that chooses its own source already explains itself on the disabled control.
    /// </summary>
    private bool Editable() => (_field?.Enabled ?? false) && _session.Monitors.Count > 0;

    /// <summary>
    /// Whether the grid has anything to draw: the reader on this step with the window in front, one screen readable apart from another, and <see cref="Editable"/>.
    /// Every one of those is read through rather than remembered,
    /// so a capture backend changed on the step above takes the grid away on the next pass with nothing here to clear.
    /// </summary>
    private bool Offered() => _onStep && _showing && _session.NoMonitorPreview is null && Editable();

    /// <summary>
    /// Opens the previews the grid wants and closes the ones it does not.
    /// A screen already asked for is not asked for again, which keeps a render pass per keystroke from being a call per screen per keystroke.
    /// The effects are idempotent, so a duplicate would be wasteful rather than wrong.
    /// On the way out it closes what is running as well as what was asked for, and the two differ after a shell that died with previews open:
    /// those outlive the window that asked for them, as decodes do, and this is where the next shell tidies them up.
    /// </summary>
    private void Converge(IReadOnlyList<Api.V1.Monitor> monitors)
    {
        if (monitors.Count == 0)
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

        foreach (var monitor in monitors)
        {
            if (_asked.Add(monitor.Index))
            {
                Open(monitor.Index);
            }
        }
    }

    /// <summary>
    /// Asks the backend to read one screen.
    /// A refusal reaches no caller: it says this machine cannot show that screen, the tile already says nothing is reading it,
    /// and the plain control below still picks the screen.
    /// </summary>
    private async void Open(int monitor)
    {
        try
        {
            await _backend.StartMonitorPreviewAsync(monitor).ConfigureAwait(false);
        }
        catch (BackendUnavailableException e)
        {
            // Held as the backend wrote it, since what refuses a screen is a fact about this machine.
            // A tile left opening for a picture that will never arrive is the one thing the row must not say.
            _dispatch(() =>
            {
                _refused[monitor] = e.Message;
                Render();
            });
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Asks the backend to stop reading one screen.
    /// A screen nothing is reading is not an error, so the one failure left is a backend that has gone, taking the previews with it.
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

    /// <summary>What the backend reports about one screen's preview. null while that screen is not being read.</summary>
    private PreviewedMonitor? Previewed(int monitor)
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

    /// <summary>The screen the draft names. -1 before a form arrives.</summary>
    private int Selected()
        => _field?.Value?.KindCase == FieldValue.KindOneofCase.Number ? (int)_field.Value.Number : -1;

    /// <summary>The form's entry for one screen. null for an output it does not offer.</summary>
    private FieldOption? OptionOf(int monitor)
    {
        if (_field is null)
        {
            return null;
        }

        var value = monitor.ToString();
        foreach (var option in _field.Options)
        {
            if (option.Value == value)
            {
                return option;
            }
        }

        return null;
    }

    /// <summary>
    /// The tile for one screen, made on first use and held afterwards.
    /// The intents go nowhere: this grid draws its tiles side by side, with no focus, pop-out or fullscreen to arrange.
    /// </summary>
    private TileViewModel Tile(Api.V1.Monitor monitor)
    {
        if (_tiles.TryGetValue(monitor.Index, out var held))
        {
            return held;
        }

        var tile = new TileViewModel(
            TileSource.MonitorPreview(monitor.Index, _session.Words.Name("publish.monitor", monitor.Index.ToString())),
            _backend,
            _dispatch,
            _ => { });

        // A tile reports what it drew, which no backend state carries: a backend cannot see that a compositor
        // was too slow to take a frame.
        // The pass it asks for is this grid's own, so the pictures and the rows over them are written by one
        // function.
        tile.Changed += Render;
        _tiles[monitor.Index] = tile;
        return tile;
    }

    /// <summary>
    /// Why one screen has no picture, in the order the states happen in: refused with the backend's reason, or asked for and not up yet.
    /// A screen never asked for reaches neither branch, since rows are built only while the grid is drawn and the grid asks for every screen it draws.
    /// </summary>
    private string PlaceholderFor(int monitor)
        => _refused.TryGetValue(monitor, out var refusal) ? refusal : Cards.ScreenOpening;

    /// <summary>
    /// Lets go of one screen's tile and answers null, so a caller drops the row's picture in the same expression.
    /// The frame subscription belongs to the control and ends when the tile leaves the tree.
    /// What goes here is the view model behind it, so a screen that comes back is drawn by a tile subscribing afresh rather than by one holding a dead channel.
    /// </summary>
    private TileViewModel? Drop(int monitor)
    {
        if (_tiles.Remove(monitor, out var tile))
        {
            tile.Changed -= Render;
        }

        return null;
    }

    private DelegateCommand SelectCommandOf(int monitor)
    {
        if (_select.TryGetValue(monitor, out var held))
        {
            return held;
        }

        var command = new DelegateCommand(() => _choose(monitor));
        _select[monitor] = command;
        return command;
    }
}
