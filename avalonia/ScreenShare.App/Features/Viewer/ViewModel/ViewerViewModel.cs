using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.ViewModel;
using ScreenShare.App.Features.Viewer.Members.ViewModel;
using ScreenShare.App.Features.Viewer.Model;
using ScreenShare.App.Features.Viewer.Tile.Model;
using ScreenShare.App.Features.Viewer.Tile.ViewModel;
using ScreenShare.App.Features.Viewer.WatchSettings.ViewModel;
using ScreenShare.App.Mvvm;
using TablerIcons;

namespace ScreenShare.App.Features.Viewer.ViewModel;

/// <summary>
/// A rail over what the relay is carrying, and a grid of what this machine is watching of it.
///
/// <b>Holds no snapshot of its own.</b>
/// <see cref="Backend.Session"/> owns the running state and <see cref="Apply"/> reads it through on every pass,
/// so the rail and the status band cannot disagree about what is on the relay.
///
/// <b>The arrangement is this shell's alone.</b>
/// Which streams are decoded is the backend's list; which are drawn, in what order and in which window crosses
/// no message, the backend describing decodes and a decode not being a tile (<c>docs/ipc-api.md</c>).
/// The tile set, the focus, the pop-outs and the fullscreen states are that arrangement, owned here rather than read.
///
/// Legs come from the backend as the options of the form's watch-leg field,
/// so this module holds no list of protocols.
/// Whether a leg carries a given stream is settled as the viewer opens (<see cref="WatchLegViewModel"/>).
///
/// The settings behind those legs are edited in the panel beside the grid.
/// They govern how this machine receives and say nothing about what it sends, hence their placement here
/// (<c>Features/Fields/Model/GroupPlacement.cs</c>).
/// </summary>
public sealed class ViewerViewModel : Observable
{
    private readonly IBackend _backend;

    /// <summary>
    /// Draft and the form it resolves to, owned by the window.
    /// Read through on every pass and never copied,
    /// so a leg changed in the wizard reaches the rows and the next decode with nothing here being told
    /// (<c>docs/development-principles.md</c>, "A reader reads through").
    /// </summary>
    private readonly FormSession _form;

    private readonly Session _session;
    private readonly Action<Action> _dispatch;
    private readonly Dictionary<string, StreamRowViewModel> _rows = [];

    /// <summary>
    /// Tiles on screen, by stream name.
    /// Written rather than read through, the one departure here:
    /// the contract describes decodes and never a window, so there is no tile list to read back
    /// (<c>docs/ipc-api.md</c>).
    /// </summary>
    private readonly Dictionary<string, TileViewModel> _tiles = [];

    /// <summary>
    /// Streams drawn in windows of their own, by name.
    ///
    /// <b>Names and not windows.</b>
    /// Keeps a toolkit type out of a view model (<c>avalonia/README.md</c>).
    ///
    /// A popped stream keeps its tile in the grid, drawn as a plate at its own shape,
    /// so nothing reflows when a stream pops out or comes back.
    /// </summary>
    private readonly HashSet<string> _popped = [];

    /// <summary>
    /// Popped-out streams whose own window should be fullscreen.
    /// A second set rather than a flag on the first: which windows exist, and which fill their screen,
    /// are different questions.
    /// Several fill theirs at once, each on the monitor its window sits on,
    /// which one app-wide fullscreen could not express.
    /// </summary>
    private readonly HashSet<string> _poppedFullscreen = [];

    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// An effect answers on whichever thread the transport completed on,
    /// and everything written here is read by a binding that tolerates one thread only.
    /// </param>
    public ViewerViewModel(IBackend backend, FormSession form, Session session, Action<Action> dispatch)
    {
        Assert.NotNull(backend, "a viewer asks the backend to open and close viewers");
        Assert.NotNull(form, "a viewer draws the settings that govern how it receives");
        Assert.NotNull(session, "a viewer renders the session's running state");
        Assert.NotNull(dispatch, "a viewer needs a UI loop to marshal an answer back to");

        _backend = backend;
        _form = form;
        _session = session;
        _dispatch = dispatch;

        Streams = [];
        Tiles = [];
        ToggleRail = new DelegateCommand(() =>
        {
            IsRailCollapsed = !IsRailCollapsed;
            Apply();
        });

        LeaveFullscreen = new DelegateCommand(() =>
        {
            Fullscreen = "";
            Apply();
        });

        // Above the list of what the relay carries,
        // so a stream's row and the member publishing it are read in one glance.
        Members = new MembersViewModel();

        // Whether the panel is open is this screen's state, so the panel is handed only a way to say it is done.
        // A panel holding the flag would be the arrangement written in two places.
        Watch = new WatchSettingsViewModel(form, session, dispatch, CloseWatchSettings);

        // Beside the grid where the window carries both, over it where it does not (Shell/Model/SideColumns.cs).
        WatchColumn = new SideColumnViewModel(
            SideColumns.ViewerWatch,
            "How this computer receives",
            "Close the watching settings");

        // News that the draft or the form behind it moved: a row's legs and a tile's leg are both read off it.
        // Raised on the UI loop by the form session, so nothing to marshal here.
        _form.Changed += Apply;

        Apply();
    }

    // --- Outputs --------------------------------------------------------------------

    private string _shownSummary = "";
    private string _notice = "";
    private bool _hasNotice;
    private string _gridEmptyLine = "";
    private bool _showsGridEmpty;
    private bool _noticeIsFailure;
    private bool _isDialling;
    private string _refusal = "";
    private bool _hasRefusal;
    private bool _hasStreams;

    /// <summary>One row per path the relay carries, in the order it listed them.</summary>
    public ObservableCollection<StreamRowViewModel> Streams { get; }

    /// <summary>
    /// Tiles on screen, in the order they were added.
    /// A stream leaves the grid when its row is toggled off and not when the relay stops carrying it,
    /// so a stream that dropped out and came back keeps the tile the reader put there.
    /// </summary>
    public ObservableCollection<TileViewModel> Tiles { get; }

    private LayoutMode _mode;
    private string _focused = "";
    private string _fullscreen = "";

    /// <summary>
    /// How the tiles are arranged.
    /// Follows the focus rather than being chosen separately: Focus with nothing focused has no drawing.
    /// </summary>
    public LayoutMode Mode { get => _mode; private set => Set(ref _mode, value); }

    /// <summary>
    /// Stream that has focus, empty when none has.
    /// <b>A name and not a tile.</b>
    /// A stream that drops out keeps its focus and its slot,
    /// which a reference to an object the drop threw away could not do.
    /// </summary>
    public string Focused { get => _focused; private set => Set(ref _focused, value); }

    /// <summary>
    /// Stream the main window is drawing fullscreen, empty when it is not.
    ///
    /// <b>Fullscreen is a property of a window, not of the app.</b>
    /// The main window's here, each popped-out window carrying its own, so several windows fill several monitors
    /// at once.
    /// Not a member of <see cref="LayoutMode"/>: a mode says how tiles sit relative to each other,
    /// this says which window one of them fills.
    ///
    /// The rail and the grid go, the shell takes its own bands off the window
    /// (<c>Features/Shell/ViewModel/ShellViewModel.cs</c>),
    /// and the picture is letterboxed on black at its stream's shape rather than stretched to the monitor's.
    /// </summary>
    public string Fullscreen { get => _fullscreen; private set => Set(ref _fullscreen, value); }

    /// <summary>
    /// Streams that should be drawn in windows of their own, as of this pass.
    /// A view opens and closes windows to match, an idempotent apply: an unchanged set opens and closes nothing.
    /// </summary>
    public IReadOnlyCollection<string> PoppedOut => _popped;

    /// <summary>Which of those windows should be filling their screen.</summary>
    public IReadOnlyCollection<string> PoppedFullscreen => _poppedFullscreen;

    private bool _hasFullscreen;
    private TileViewModel? _fullscreenTile;
    private bool _isRailCollapsed;

    /// <summary>
    /// Width the window last stated, infinite until it states one (<see cref="SetWindowWidth"/>).
    /// </summary>
    private double _window = double.PositiveInfinity;

    /// <summary>Whether a tile fills this window, taking the rail and the grid off it.</summary>
    public bool HasFullscreen { get => _hasFullscreen; private set => Set(ref _hasFullscreen, value); }

    public TileViewModel? FullscreenTile { get => _fullscreenTile; private set => Set(ref _fullscreenTile, value); }

    /// <summary>
    /// Whether the reader collapsed the rail to its toggle.
    /// A reader watching a wall of streams wants the width, a reader looking for another wants the list.
    /// </summary>
    public bool IsRailCollapsed { get => _isRailCollapsed; private set => Set(ref _isRailCollapsed, value); }

    /// <summary>
    /// Whether the rail draws the names.
    /// Two facts read as one: what the reader asked for,
    /// and whether the window carries the names and a tile beside them
    /// (<c>Features/Shell/Model/SideColumns.cs</c>).
    /// </summary>
    public bool ShowsRailNames => !IsRailCollapsed && SideColumns.ViewerRail.FitsBeside(_window);

    /// <summary>
    /// Whether the collapse is offered.
    /// A window too narrow for the names has none to take away, and a control that changes nothing reads as broken.
    /// </summary>
    public bool ShowsRailToggle => SideColumns.ViewerRail.FitsBeside(_window);

    /// <summary>
    /// Collapsed fits an entry with its name taken out: the dot, the action button, the gaps between them,
    /// the padding around them and the list's scrollbar.
    /// The rail's header buttons are narrower than that.
    /// An entry keeps its shape and loses only its name.
    /// </summary>
    public double RailWidth => ShowsRailNames ? 240 : 88;

    public Icons RailGlyph => IsRailCollapsed ? Icons.IconChevronRight : Icons.IconChevronLeft;

    public string RailToggleTip => IsRailCollapsed ? "Show the stream names" : "Collapse the rail";

    public DelegateCommand ToggleRail { get; }

    /// <summary>
    /// Gives the main window back to its grid, whether or not a stream was filling it.
    /// The window's key rather than a tile's: a filled window draws no rail, no menu and no band,
    /// so the keyboard is what a reader can still reach (<c>Features/Shell/View/ShellWindow.axaml</c>).
    /// </summary>
    public DelegateCommand LeaveFullscreen { get; }

    /// <summary>
    /// Who this machine shares a group with, and the control that puts it in the group or takes it out.
    /// Never who is watching what: the group states presence and publication.
    /// </summary>
    public MembersViewModel Members { get; }

    /// <summary>
    /// How this machine receives: the legs, the jitter buffers and the render chain.
    /// One group of the same resolved form the setup wizard draws its steps from, on the screen its settings govern.
    /// </summary>
    public WatchSettingsViewModel Watch { get; }

    /// <summary>
    /// Where the panel stands, and whether it is open.
    /// Beside the grid on a window carrying the rail, the tiles and the panel at once, and over the grid on one
    /// that does not (<c>docs/design-language.md</c>, "Narrow windows").
    /// </summary>
    public SideColumnViewModel WatchColumn { get; }

    /// <summary>
    /// Shuts the settings panel, whether or not it was open.
    /// Names the state rather than a transition, so the panel's close button and its commit can both run it:
    /// a commit that closed by toggling would reopen a panel the reader had already dismissed.
    /// </summary>
    private void CloseWatchSettings()
    {
        WatchColumn.Close();
        Apply();
    }

    /// <summary>
    /// States the width the window has, which decides where the rail and the panel stand.
    /// Idempotent: the same width twice moves nothing.
    /// </summary>
    public void SetWindowWidth(double width)
    {
        Assert.That(width > 0, "a window states a width it has", width);

        _window = width;
        WatchColumn.SetWindowWidth(width);
        Apply();
    }

    /// <summary>For a view that has to hand a tile to a window it is opening.</summary>
    public TileViewModel? TileOf(string stream) => _tiles.GetValueOrDefault(stream);

    /// <summary>
    /// Raised after a pass in which the windows a view should be showing changed.
    /// The one thing a view is told imperatively, nothing binding a window into existence.
    /// </summary>
    public event Action? WindowsChanged;

    /// <summary>How much of what the relay carries this machine is watching, as the status band prints it.</summary>
    public string ShownSummary { get => _shownSummary; private set => Set(ref _shownSummary, value); }

    /// <summary>
    /// Relay's cost, as the status band prints it.
    /// A list rather than named slots, so what a destination reports stays that destination's business.
    /// </summary>
    public IReadOnlyList<string> Figures { get; private set; } = [];

    /// <summary>Stands in for the list while it is empty, and tells an unread relay from an idle one.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    /// <summary>What the empty grid says, empty while a tile is up or another notice speaks (<see cref="Model.GridEmpty"/>).</summary>
    public string GridEmptyLine { get => _gridEmptyLine; private set => Set(ref _gridEmptyLine, value); }

    public bool ShowsGridEmpty { get => _showsGridEmpty; private set => Set(ref _showsGridEmpty, value); }

    /// <summary>
    /// Whether the notice reports a failure rather than a state of the relay.
    /// True for the backend's own sentence, which is the one thing here that is broken:
    /// a relay carrying nothing answered (<c>docs/design-language.md</c>, "Palette").
    /// </summary>
    public bool NoticeIsFailure { get => _noticeIsFailure; private set => Set(ref _noticeIsFailure, value); }

    /// <summary>
    /// Whether the window is still dialling behind the notice.
    /// False where the notice is not about the backend.
    /// The window opens on this screen, so a shell launched before its backend draws it,
    /// and a sentence that never moves makes one still dialling look stuck.
    /// </summary>
    public bool IsDialling { get => _isDialling; private set => Set(ref _isDialling, value); }

    /// <summary>Backend's own sentence when it refused to open or close something, empty otherwise.</summary>
    public string Refusal { get => _refusal; private set => Set(ref _refusal, value); }

    public bool HasRefusal { get => _hasRefusal; private set => Set(ref _hasRefusal, value); }

    public bool HasStreams { get => _hasStreams; private set => Set(ref _hasStreams, value); }

    /// <summary>
    /// Printed by the status band rather than over the grid.
    /// The gestures it names are the tile's own (<c>Features/Viewer/Tile/View/TileKeys.cs</c>).
    /// </summary>
    public string Hint => "Right-click a tile for fullscreen, focus, pop-out, and volume. Escape leaves fullscreen";

    /// <summary>Heading over the rail's list.</summary>
    public string ShowingLabel => "On the relay";

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// The one render function.
    /// Reads the session and the form through, keeping no copy of either.
    /// Safe to run twice: a row is reused by stream name and renders itself idempotently,
    /// so an unchanged snapshot fires no binding.
    /// </summary>
    public void Apply()
    {
        // Names the state wanted, a form resolved from the draft as it stands,
        // and the sync decides whether a round trip is owed (docs/development-principles.md, "Idempotency").
        _form.Sync();

        var legs = LegsOf(_session.PlayerLegs);
        var browserLegs = LegsOf(_session.BrowserLegs);
        var relay = _session.Relay;
        var rows = Rows(relay, _session.Watching);

        foreach (var row in rows)
        {
            Of(row.Name).Apply(row, legs, browserLegs, _tiles.ContainsKey(row.Name));
        }

        Reconcile.Onto(Streams, rows.Select(row => Of(row.Name)).ToList());

        // The one way out of Focus the reader did not ask for: a mode whose subject left the grid draws nothing.
        // A stream that merely stopped publishing keeps its tile.
        if (Focused.Length > 0 && !_tiles.ContainsKey(Focused))
        {
            Focused = "";
        }

        // A window whose stream left the grid has nothing behind it,
        // and a fullscreen state with no window has nothing to answer for.
        _popped.RemoveWhere(stream => !_tiles.ContainsKey(stream));
        _poppedFullscreen.RemoveWhere(stream => !_popped.Contains(stream));
        if (Fullscreen.Length > 0 && !_tiles.ContainsKey(Fullscreen))
        {
            Fullscreen = "";
        }

        // This window's fullscreen names a stream of this window's own grid,
        // so a stream that left for a window of its own gives this one back.
        // Fullscreen does not travel with the stream:
        // one arriving already filling a screen is a state the reader did not ask for.
        if (_popped.Contains(Fullscreen))
        {
            Fullscreen = "";
        }

        Mode = Focused.Length > 0 ? LayoutMode.Focus : LayoutMode.Grid;

        // Rendered from the backend's decode list, joined on the pair the contract keys a decode by.
        // A tile whose decode is not in it draws its own reason rather than disappearing:
        // the reader put it there.
        foreach (var tile in _tiles.Values)
        {
            tile.Apply(TilePipeline.Of(DecodeOf(tile)), _session.StatsOf(tile.Name, tile.Transport));
            tile.IsFocused = tile.Name == Focused;
            tile.IsPoppedOut = _popped.Contains(tile.Name);

            // Which window a stream is drawn in decides which fullscreen state answers for it.
            // Derived here rather than kept on the tile,
            // so the flag the menu ticks and the state the windows obey cannot disagree.
            tile.IsFullscreen = _popped.Contains(tile.Name)
                ? _poppedFullscreen.Contains(tile.Name)
                : Fullscreen == tile.Name;
        }

        // An absent backend is why there is no relay reading at all,
        // so it answers before the relay's own states and is the one notice the dialling belongs under.
        var absent = relay is null && _session.Unavailable.Length > 0;

        HasStreams = Streams.Count > 0;
        Notice = HasStreams ? "" : absent ? _session.Unavailable : NoticeFor(relay);
        HasNotice = Notice.Length > 0;
        NoticeIsFailure = absent;

        // Read off the sentence's own verdict: an attempt is coming for the notice that is a failure,
        // and for no other.
        IsDialling = NoticeIsFailure;

        var watched = rows.Count(row => row.IsWatched);
        ShownSummary = HasStreams ? $"{watched} of {rows.Count} streams watched" : "";
        Figures = FiguresFor(rows);

        HasRefusal = Refusal.Length > 0;

        FullscreenTile = Fullscreen.Length > 0 ? _tiles.GetValueOrDefault(Fullscreen) : null;
        HasFullscreen = FullscreenTile is not null;

        Members.Reported = _session.Members;
        Members.Apply();

        GridEmptyLine = GridEmpty.For(
            _session.Members, _session.Discord, _form.Stored?.Relay?.DiscordMode == true,
            Streams.Count, _tiles.Count, relay?.Reachable == true);
        ShowsGridEmpty = GridEmptyLine.Length > 0;

        // Draws from the same draft on every pass,
        // so a vocabulary arriving with the catalog reaches the panel's entries without a notification of its own.
        Watch.Apply();

        // Raised by hand: a property with no field of its own has nothing to compare against.
        OnPropertyChanged(nameof(ShowsRailNames));
        OnPropertyChanged(nameof(ShowsRailToggle));
        OnPropertyChanged(nameof(RailWidth));
        OnPropertyChanged(nameof(RailGlyph));
        OnPropertyChanged(nameof(RailToggleTip));

        WindowsChanged?.Invoke();

        Assert.That(Streams.Count == rows.Count, "a row per stream on the relay", Streams.Count, rows.Count);
        Assert.That(HasFullscreen == (FullscreenTile is not null), "a fullscreen tile and the state drawing it agree", HasFullscreen);
        Assert.That(Mode == LayoutMode.Focus == (Focused.Length > 0), "focus and the mode that draws it agree", Mode, Focused);
        Assert.That(Focused.Length == 0 || _tiles.ContainsKey(Focused), "a focused stream is one of the tiles", Focused);
        Assert.That(Fullscreen.Length == 0 || _tiles.ContainsKey(Fullscreen), "a fullscreen stream is one of the tiles", Fullscreen);
        Assert.That(!_popped.Contains(Fullscreen), "the stream filling this window is one of its own grid's", Fullscreen);
        Assert.That(_tiles.Count == Tiles.Count, "one tile per stream in the grid", _tiles.Count, Tiles.Count);
        Assert.That(HasStreams == (Notice.Length == 0), "a list and the sentence standing in for it are never both on screen", HasStreams);
        Assert.That(HasNotice == (Notice.Length > 0), "the notice and its text agree", HasNotice);
        Assert.That(!NoticeIsFailure || HasNotice, "a failure is marked on the sentence stating it", NoticeIsFailure, HasNotice);
        Assert.That(!IsDialling || HasNotice, "the wait appears under the notice it belongs to", IsDialling, HasNotice);
        Assert.That(HasRefusal == (Refusal.Length > 0), "a refusal and its sentence agree", HasRefusal);
    }

    /// <summary>
    /// One row per relay path, carrying the legs this machine already has open on it.
    /// Joined on the name the backend uses on both sides,
    /// so nothing here has to know what a transport or a format is.
    /// </summary>
    private static IReadOnlyList<StreamRow> Rows(RelayStatus? relay, IReadOnlyList<StreamRef> watching)
    {
        if (relay is null || !relay.Reachable)
        {
            return [];
        }

        var rows = new List<StreamRow>(relay.Paths.Count);
        foreach (var path in relay.Paths)
        {
            var open = new List<string>();
            foreach (var key in watching)
            {
                if (key.StreamName == path.Name)
                {
                    open.Add(key.Transport);
                }
            }

            rows.Add(new StreamRow
            {
                Name = path.Name,
                IsReady = path.Ready,
                Tracks = path.Tracks,
                Format = path.Format,
                Readers = path.Readers,
                InMbps = path.InMbps,
                WatchedOn = open,
            });
        }

        return rows;
    }

    /// <summary>
    /// Why there is nothing to list, for every reason but an absent backend, which the render pass answers first.
    /// An unread relay, an unreachable one and an idle one are states a reader has to tell apart,
    /// and an unreachable one says why in the relay's own words.
    /// </summary>
    private static string NoticeFor(RelayStatus? relay)
    {
        if (relay is null)
        {
            return "Reading what the relay is carrying.";
        }

        if (!relay.Reachable)
        {
            return relay.Error.Length > 0 ? relay.Error : "The relay could not be reached.";
        }

        return "The relay is up and carrying nothing.";
    }

    /// <summary>
    /// What the band prints: the relay's total ingest, and its readers across every path.
    /// Both are the relay's own figures, this machine's decode being reported per tile in the stats panel.
    /// </summary>
    private static IReadOnlyList<string> FiguresFor(IReadOnlyList<StreamRow> rows)
    {
        if (rows.Count == 0)
        {
            return [];
        }

        var ingest = rows.Where(row => row.IsReady).Sum(row => row.InMbps);
        var readers = rows.Sum(row => row.Readers);

        return [$"relay in {ingest:0.0} Mb/s", $"{readers} readers"];
    }

    private StreamRowViewModel Of(string name)
    {
        if (_rows.TryGetValue(name, out var row))
        {
            return row;
        }

        row = new StreamRowViewModel(name, WatchAsync, TileAsync, BrowseAsync, _dispatch);
        _rows[name] = row;
        return row;
    }

    /// <summary>
    /// Backend's state for one tile's decode, null while nothing is decoding that pair.
    /// Joined on the stream name and the leg together,
    /// the relay re-serving each stream on all its listeners so the name alone is no identity.
    /// </summary>
    private ReceiveStream? DecodeOf(TileViewModel tile)
    {
        foreach (var decode in _session.Receiving)
        {
            if (decode.Stream.StreamName == tile.Name && decode.Stream.Transport == tile.Transport)
            {
                return decode;
            }
        }

        return null;
    }

    // --- The effects ----------------------------------------------------------------

    /// <summary>
    /// Legs of one roster, named for this screen.
    /// The list is the catalog's and the words this side's:
    /// which legs a receiver opens on is a fact, what a protocol is called on a row is a decision about the screen.
    ///
    /// One answer serves every row, and no entry carries a verdict.
    /// A roster names the legs its own receiver reaches and answers nothing about a stream,
    /// so whether a leg carries this one is settled as the viewer opens (<see cref="WatchLegViewModel"/>).
    ///
    /// Empty until the first catalog read lands, a menu with nothing under it rather than one guessing at protocols.
    /// </summary>
    private static IReadOnlyList<WatchLeg> LegsOf(IReadOnlyList<string> legs)
        => legs.Select(leg => new WatchLeg(leg, Copy.Words.Transport(leg))).ToList();

    /// <summary>
    /// Puts one stream in the grid, or takes it out.
    ///
    /// <b>Two calls in each direction, and the order is the point.</b>
    /// In: the decode opens first, a tile being a subscription to frames and there being none until something
    /// decodes.
    /// Out: the tile goes first,
    /// a subscription outliving its decode being a window holding handles to memory the backend has freed.
    ///
    /// <b>The two directions name their leg from two places.</b>
    /// A start opens the leg the stored settings say a tile receives on;
    /// a stop closes the leg the tile was opened on, possibly an older setting.
    /// A decode is keyed by the stream and the leg together,
    /// so stopping on the current setting would leave one running whenever the leg had moved.
    ///
    /// <b>Stored and not the draft.</b>
    /// The leg is the only knob of the watch group this call names,
    /// the backend reading the render chain and the jitter buffers out of its own settings as it builds the decode.
    /// Opening on the draft would run an unkept leg against kept buffers
    /// (<c>Features/Viewer/Tile/Model/TileLeg.cs</c>).
    /// </summary>
    private async Task TileAsync(string stream, bool tiled)
    {
        if (tiled)
        {
            // Read before the tile goes, the tile being where the answer is.
            var opened = _tiles.TryGetValue(stream, out var tile) ? tile.Transport : "";
            Drop(stream);

            if (opened.Length == 0)
            {
                return;
            }

            try
            {
                await _backend.StopReceiveAsync(stream, opened).ConfigureAwait(false);
                Refused("");
            }
            catch (BackendUnavailableException e)
            {
                Refused(e.Message);
            }
            catch (OperationCanceledException)
            {
            }

            return;
        }

        var leg = TileLeg.Of(_form.Stored);
        if (leg.Length == 0)
        {
            Refused("The settings name no protocol for a tile to receive on. Pick one under Watching.");
            return;
        }

        try
        {
            await _backend.StartReceiveAsync(stream, leg).ConfigureAwait(false);
            Refused("");
            _dispatch(() => Add(stream, leg));
        }
        catch (BackendUnavailableException e)
        {
            // Shown as it arrived: a refusal names the format and the protocols that would have carried it.
            Refused(e.Message);
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Hands each tile its own decode's loudness.
    /// <b>Not part of <see cref="Apply"/>.</b>
    /// Levels arrive fifteen times a second, and the render pass at that rate would re-read the relay,
    /// the roster and every decode to move a bar.
    /// Writes only what the meter binds (<c>Backend/Session.cs</c>, <c>Metered</c>).
    /// </summary>
    public void Meter()
    {
        foreach (var tile in _tiles.Values)
        {
            tile.Meter(_session.LevelOf(tile.Name, tile.Transport));
        }
    }

    /// <summary>
    /// Does what a tile's menu or key asked for.
    /// <b>Every intent is decided here.</b>
    /// Each is a fact about the whole arrangement, so a tile raises the intent and writes none of it
    /// (<c>Features/Viewer/Model/TileIntent.cs</c>).
    /// </summary>
    private void Arrange(string stream, TileIntent intent)
    {
        Assert.That(stream.Length > 0, "an arrangement is asked for by a tile that names its stream");

        switch (intent)
        {
            case TileIntent.Focus:
                // A second stream asking for focus takes it.
                // No state has two focused, so none has one to take away first.
                Focused = Focused == stream ? "" : stream;
                break;

            case TileIntent.PopOut:
                // The decode is untouched either way: closing a pop-out returns the stream to its slot,
                // stopping being the rail's toggle.
                if (!_popped.Remove(stream))
                {
                    _popped.Add(stream);
                }

                break;

            case TileIntent.Fullscreen:
                // Which window the stream is drawn in decides which state moves:
                // a popped-out stream fills its own window's screen and several do that at once,
                // a stream in the grid fills the main window, of which there is one.
                if (_popped.Contains(stream))
                {
                    if (!_poppedFullscreen.Remove(stream))
                    {
                        _poppedFullscreen.Add(stream);
                    }
                }
                else
                {
                    Fullscreen = Fullscreen == stream ? "" : stream;
                }

                break;

            case TileIntent.LeavePopOut:
                // The state a closed window reports, so a stream already in the grid is left where it is.
                // The decode is untouched: a window that closed is a picture that moved, not a stream that stopped.
                _popped.Remove(stream);
                break;

            case TileIntent.Close:
                // The rail toggle's own path, fired without waiting:
                // the tile leaves the grid before the stop's round trip answers.
                _ = TileAsync(stream, true);
                break;

            case TileIntent.LeaveFullscreen:
                // Names the state a window is to be in and never the transition,
                // so a stream filling nothing is left alone and the key is safe wherever it fires.
                if (_popped.Contains(stream))
                {
                    _poppedFullscreen.Remove(stream);
                }
                else if (Fullscreen == stream)
                {
                    Fullscreen = "";
                }

                break;
        }

        Apply();
    }

    /// <summary>
    /// Adds one tile, on the UI loop, and re-renders.
    /// The leg is passed in rather than read again,
    /// so the tile is keyed by the pair the backend keyed the decode by even where the setting moved in between.
    /// </summary>
    private void Add(string stream, string leg)
    {
        Assert.That(leg.Length > 0, "a tile names the leg its decode was opened on", stream);

        if (_tiles.ContainsKey(stream))
        {
            return;
        }

        var tile = new TileViewModel(
            TileSource.Relay(stream, leg), _backend, _dispatch, intent => Arrange(stream, intent));
        // A tile reports what it drew, which no state the backend owns can carry.
        // The pass it asks for is this screen's,
        // so the figures over a tile and the rail beside it stay one render function.
        tile.Changed += Apply;

        _tiles[stream] = tile;
        Tiles.Add(tile);
        Apply();
    }

    /// <summary>
    /// Takes one tile off the grid and re-renders.
    /// Removing it from the collection ends the frame subscription:
    /// the control drawing it detaches, cancelling the call and disposing every handle it imported
    /// (<c>Features/Viewer/Tile/View/StreamTile.cs</c>).
    /// </summary>
    private void Drop(string stream)
    {
        if (!_tiles.Remove(stream, out var tile))
        {
            return;
        }

        tile.Changed -= Apply;
        Tiles.Remove(tile);

        // The arrangement is not edited here.
        // Apply drops the focus, the pop-out and the fullscreen of a stream that left the grid.
        Apply();
    }

    /// <summary>
    /// Opens or closes one external viewer.
    /// No local copy of the roster is edited, the roster being the backend's list.
    /// </summary>
    private async Task WatchAsync(string stream, string transport, bool open)
    {
        try
        {
            if (open)
            {
                await _backend.StopWatchAsync(stream, transport).ConfigureAwait(false);
            }
            else
            {
                await _backend.StartWatchAsync(stream, transport).ConfigureAwait(false);
            }

            Refused("");

            // The backend announces a viewer that ended and not one that started,
            // so the roster is re-read rather than patched with what was sent.
            var watching = await _backend.WatchingAsync().ConfigureAwait(false);
            _session.Adopt(watching);
        }
        catch (BackendUnavailableException e)
        {
            Refused(e.Message);
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Opens the relay's player page for one stream in the machine's default browser.
    /// Nothing is re-read afterwards, no list having moved.
    /// The page opens in a program this app does not own, so what came of it is a state neither side can report.
    /// The refusal is, and it lands where every other one does.
    /// </summary>
    private async Task BrowseAsync(string stream, string transport)
    {
        try
        {
            await _backend.OpenInBrowserAsync(stream, transport).ConfigureAwait(false);
            Refused("");
        }
        catch (BackendUnavailableException e)
        {
            Refused(e.Message);
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Records what the backend said about the last effect and re-renders, on the UI loop.
    /// An empty reason is a success and clears whatever the last failure left.
    /// </summary>
    private void Refused(string reason)
    {
        _dispatch(() =>
        {
            Refusal = reason;
            Apply();
        });
    }
}
