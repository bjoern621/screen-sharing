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
/// The screens this machine has, each drawn from what is on it, so one is picked by looking
/// rather than by its number.
///
/// <b>The problem it removes.</b> A monitor used to be a dropdown of indices with a size beside
/// each, and the only picture in the app needed a live publish - so the flow was to guess, go
/// live, and find out. The picture here is taken before anything is sent: nothing is encoded,
/// no bandwidth is spent, and the relay is not a party to it.
///
/// <b>It offers the screens the form offers and no others.</b> Which outputs exist is the
/// catalog's, whether the setting can be edited at all and which entries are greyed is the
/// resolved form's, and both are read through on every pass (<c>docs/ipc-api.md</c>). What this
/// class adds is the pictures and the arrangement.
///
/// <b>The pictures are opened and closed here, and that is the only state it owns.</b> A
/// preview is a screen capture the backend runs until something stops it, so it goes up when
/// the reader is standing on this step with the window in front, and down when either stops
/// being true (<see cref="Converge"/>). It is a converge and not a sequence: a second pass over
/// unchanged input opens nothing and closes nothing.
///
/// <b>Where there are no pictures the wizard is unchanged.</b> A session that cannot read one
/// screen apart from another says so in the catalog, and a capture backend that takes no monitor
/// index disables the field; either way this draws nothing and the plain control is what the
/// reader gets.
/// </summary>
public sealed class ScreenPickerViewModel : Observable
{
    private readonly IBackend _backend;
    private readonly Session _session;
    private readonly Action<Action> _dispatch;

    /// <summary>Writes the chosen screen into the draft, which is the form's business and not this one's.</summary>
    private readonly Action<int> _choose;

    /// <summary>
    /// One tile per output, kept across passes and made on demand. Rebuilding one would restart
    /// a subscription that is already drawing the same screen.
    /// </summary>
    private readonly Dictionary<int, TileViewModel> _tiles = [];

    /// <summary>
    /// One select command per output, made once and reused, so two passes over an unchanged
    /// screen produce rows that compare equal and the collection is left alone.
    /// </summary>
    private readonly Dictionary<int, DelegateCommand> _select = [];

    /// <summary>
    /// The screens this shell has asked the backend to read, which is the desired state the
    /// converge is written against.
    ///
    /// It is asked-for and not running, and the difference is deliberate. A preview that ended
    /// on its own drops out of the reported set, and converging on that set would ask for it
    /// again immediately - which on a screen that cannot be read at all is a loop. So a screen
    /// is asked for once per visit to this step, and one that died stays dark with the tile
    /// saying so until the reader comes back to the step.
    /// </summary>
    private readonly HashSet<int> _asked = [];

    /// <summary>
    /// Why one screen could not be read, in the backend's own words, for the screens that were
    /// refused. It is what separates a picture that has not arrived yet from one that never will,
    /// which are the same empty tile and different things to tell a reader.
    /// </summary>
    private readonly Dictionary<int, string> _refused = [];

    /// <summary>Whether the control is on screen in a window that is in front. Written by the view.</summary>
    private bool _showing;

    /// <summary>Whether the wizard is standing on the step this belongs to. Written by the flow.</summary>
    private bool _onStep;

    /// <summary><c>publish.monitor</c> as the form last resolved it, null before a form has arrived.</summary>
    private Field? _field;

    /// <param name="choose">Writes the picked screen into the draft. It is handed in because the
    /// draft belongs to the form session the window holds, and a second writer would be a second
    /// copy of what the settings say.</param>
    /// <param name="dispatch">Hands work to the UI loop. A tile's own reports land on whichever
    /// thread the transport completed on, and everything this writes is read by a binding that
    /// only tolerates being written from one.</param>
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

    /// <summary>The screens on offer, in the enumeration's order. Empty while the grid is not drawn.</summary>
    public ObservableCollection<ScreenChoice> Screens { get; } = [];

    /// <summary>
    /// Whether the grid is drawn at all. False on a session with no way to read one screen, on a
    /// capture backend that takes no monitor index, and on a machine whose outputs could not be
    /// enumerated - three different facts with one consequence, each of which the plain control
    /// beneath already explains in its own words.
    /// </summary>
    public bool IsVisible { get => _isVisible; private set => Set(ref _isVisible, value); }

    /// <summary>
    /// What the pictures are, said once above the grid rather than on each tile. Empty while the
    /// grid is not drawn.
    /// </summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    // --- Inputs -------------------------------------------------------------------

    /// <summary>
    /// The screen setting as the form resolved it, and whether the reader is standing on the step
    /// that holds it. Both are the flow's to say; neither is read off a widget.
    /// </summary>
    public void Apply(Field? monitor, bool onStep)
    {
        _field = monitor;
        _onStep = onStep;
        Render();
    }

    /// <summary>
    /// Says whether the grid is being looked at. The named write of that state, and idempotent:
    /// telling it what it already holds re-renders and converges to the same world.
    ///
    /// The view calls it, because whether a control is in a visual tree and whether the window
    /// around it is in front are facts only the control and the platform can see.
    /// </summary>
    public void SetShowing(bool showing)
    {
        _showing = showing;
        Render();
    }

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// The one render function. Writes every output on every pass and converges the previews
    /// afterwards, so neither a row nor a running screen capture can stick.
    /// </summary>
    public void Render()
    {
        IsVisible = Offered();

        var monitors = IsVisible ? _session.Monitors : [];
        Converge(monitors);

        var rows = new List<ScreenChoice>(monitors.Count);
        foreach (var monitor in monitors)
        {
            // The tile is made from what the backend reports and dropped with it. A tile
            // subscribes to frames when it is drawn, and a subscription that names a screen
            // nothing is reading is refused once and never asked again, so a tile built while
            // the start was still in flight would sit dark for the life of the step.
            var previewed = Previewed(monitor.Index);
            var tile = previewed is null ? Drop(monitor.Index) : Tile(monitor);
            tile?.Apply(TilePipeline.Of(previewed));

            var option = OptionOf(monitor.Index);
            rows.Add(new ScreenChoice(
                monitor.Index,
                _session.Words.Name("publish.monitor", monitor.Index.ToString()),
                IsSelected: Selected() == monitor.Index,
                // An entry the backend did not offer is one this machine has an output for and
                // the settings cannot reach - a monitor unplugged since the value was stored is
                // the case - so it is drawn and cannot be picked, which is what the dropdown
                // beneath does with the same entry.
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
    /// What is said above the control, in the order the states happen in.
    ///
    /// Two sentences and they are different news. A grid that is drawing says what its pictures
    /// are, and above all that none of them is being shared yet. A machine that could have shown
    /// pictures and cannot says why, in the backend's own statement: the reader is standing on
    /// the step, the setting is theirs to change, and an absence with no reason beside it reads
    /// as a fault rather than as how their session works.
    ///
    /// The second does not wait on <see cref="_showing"/>. A sentence costs nothing to draw,
    /// where a picture costs a screen capture.
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
    /// Whether the reader could pick a screen at all: the form left the setting editable and
    /// this machine enumerated something to pick between. Both have to hold before the absence
    /// of pictures is worth a word - a capture backend that chooses its own source already
    /// explains itself on the disabled control.
    /// </summary>
    private bool Editable() => (_field?.Enabled ?? false) && _session.Monitors.Count > 0;

    /// <summary>
    /// Whether the grid has anything to draw: the reader is on this step with the window in
    /// front, this machine can read one screen apart from another, the setting is editable, and
    /// there are outputs to offer.
    ///
    /// All four are read through rather than remembered, so a capture backend changed on the
    /// step above takes the grid away on the next pass with nothing here to clear.
    /// </summary>
    private bool Offered() => _onStep && _showing && _session.NoMonitorPreview is null && Editable();

    /// <summary>
    /// Opens the previews the grid wants and closes the ones it does not.
    ///
    /// <b>It is a converge and it calls nothing it does not have to.</b> A screen already asked
    /// for is not asked for again, which is what keeps a render pass on every keystroke from
    /// being a call per screen per keystroke; the effects themselves are idempotent, so a
    /// duplicate would be harmless and merely wasteful.
    ///
    /// On the way out it closes what is running as well as what was asked for. The two differ
    /// only after a shell that died with previews open: those outlive the window that asked for
    /// them, exactly as decodes do, and this is where the next shell tidies them up.
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
    /// Asks the backend to read one screen. A refusal is swallowed: what it means is that this
    /// machine cannot show that screen, and the tile beside it already says nothing is reading
    /// it. There is nothing else for a reader to do about it here, and the plain control below
    /// still picks the screen.
    /// </summary>
    private async void Open(int monitor)
    {
        try
        {
            await _backend.StartMonitorPreviewAsync(monitor).ConfigureAwait(false);
        }
        catch (BackendUnavailableException e)
        {
            // Kept rather than swallowed, and kept as the backend wrote it. What refuses a
            // screen is a fact about this machine, and a tile that stayed on "opening" for a
            // picture that will never arrive is the one thing the row must not say.
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
    /// Asks the backend to stop reading one screen. A screen nothing is reading is not an error,
    /// so the only failure this can meet is a backend that has gone, which takes the previews
    /// with it.
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

    /// <summary>What the backend says about one screen's preview, and null while it is reading none.</summary>
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

    /// <summary>The screen the draft currently names, and -1 before a form has arrived.</summary>
    private int Selected()
        => _field?.Value?.KindCase == FieldValue.KindOneofCase.Number ? (int)_field.Value.Number : -1;

    /// <summary>The form's entry for one screen, and null for an output the form does not offer.</summary>
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
    /// The tile for one screen, made on first use and kept afterwards. Nothing is rearranged from
    /// here: this grid draws its tiles side by side with no focus, pop-out or fullscreen, so the
    /// intents go nowhere.
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

        // A tile reports what it drew, which no state the backend owns can carry: a backend
        // cannot see that a compositor was too slow to take a frame. The pass it asks for is
        // this grid's own, so the pictures and the rows over them are written by one function.
        tile.Changed += Render;
        _tiles[monitor.Index] = tile;
        return tile;
    }

    /// <summary>
    /// Why one screen has no picture, in the order the states happen in: the backend refused to
    /// read it and said why, or it has been asked for and has not come up yet.
    ///
    /// A screen that was never asked for reaches neither branch, because the rows are only built
    /// while the grid is drawn and the grid asks for every screen it draws.
    /// </summary>
    private string PlaceholderFor(int monitor)
        => _refused.TryGetValue(monitor, out var refusal) ? refusal : Cards.ScreenOpening;

    /// <summary>
    /// Lets go of one screen's tile, and returns null so a caller can drop the row's picture in
    /// the same expression. The subscription belongs to the control, which ends it when the tile
    /// leaves the tree; what is dropped here is the view model behind it, so a screen that comes
    /// back is drawn by a tile that subscribes afresh rather than by one holding a dead channel.
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
