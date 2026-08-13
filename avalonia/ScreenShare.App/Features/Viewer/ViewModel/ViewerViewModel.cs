using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
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
/// <b>It holds no snapshot of its own.</b> <see cref="Backend.Session"/> owns the running state and
/// <see cref="Apply"/> reads it through on every pass, so the rail and the status band cannot disagree about
/// what is on the relay.
///
/// <b>The arrangement is this shell's alone.</b> Which streams are decoded is the backend's list; which of
/// them are drawn, in what order and in which window crosses no message, because the backend describes
/// decodes and a decode is not a tile (<c>docs/ipc-api.md</c>).
/// The tile set, the focus, the pop-outs and the fullscreen states are that arrangement, and they are what
/// this class owns rather than reads.
///
/// The legs a stream can be opened on come from the backend: they are the options of the form's watch-leg
/// field, so this module holds no list of protocols.
/// Whether a given leg can carry a given stream is answered when the viewer is opened, and the refusal is
/// shown as it stands - the relay's snapshot can be older than the stream, so greying a leg here from a stale
/// format would refuse a viewer that would have worked.
///
/// The settings behind those legs are edited here too, in the panel beside the grid.
/// They govern how this machine receives and say nothing about what it sends, which is why they are placed on
/// this screen (<c>Features/Fields/Model/GroupPlacement.cs</c>).
/// </summary>
public sealed class ViewerViewModel : Observable
{
    private readonly IBackend _backend;

    /// <summary>
    /// The draft and the form it resolves to, owned by the window.
    /// Read through on every pass and never copied, so a leg changed in the wizard reaches the rows and the
    /// next decode without this screen being told (<c>docs/development-principles.md</c>, "A reader reads
    /// through").
    /// </summary>
    private readonly FormSession _form;

    private readonly Session _session;
    private readonly Action<Action> _dispatch;
    private readonly Dictionary<string, StreamRowViewModel> _rows = [];

    /// <summary>
    /// The tiles on screen, by stream name.
    /// Owned here because there is nothing to read a tile list back from: the contract describes decodes and
    /// never a window (<c>docs/ipc-api.md</c>).
    /// </summary>
    private readonly Dictionary<string, TileViewModel> _tiles = [];

    /// <summary>
    /// The streams drawn in windows of their own, by name.
    ///
    /// <b>Names and not windows.</b> A view reconciles windows against this set on every pass, which is what
    /// makes opening one idempotent and what keeps a toolkit type out of a view model
    /// (<c>avalonia/README.md</c>).
    ///
    /// A popped stream keeps its tile in the grid, drawn as a plate at its own shape, so nothing reflows when
    /// a stream pops out or comes back.
    /// </summary>
    private readonly HashSet<string> _popped = [];

    /// <summary>
    /// The popped-out streams whose own window should be fullscreen.
    ///
    /// A second set rather than a flag on the first, because it answers a different question: which windows
    /// exist, and which of them fill their screen.
    /// Several fill theirs at once, each on the monitor its window is already on, which a single app-wide
    /// fullscreen could not express.
    /// </summary>
    private readonly HashSet<string> _poppedFullscreen = [];

    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// An effect answers on whichever thread the transport completed on, and everything written here is read
    /// by a binding that tolerates one thread only.
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

        // Names the state rather than the transition, so a press with nothing filling the window, or a press
        // from another screen, is a pass that changes nothing.
        // Each popped-out window answers for its own fullscreen, so this clears the main window's and no
        // more.
        LeaveFullscreen = new DelegateCommand(() =>
        {
            Fullscreen = "";
            Apply();
        });

        // Whether the panel is open is this screen's state and stays here, so the panel is handed the one
        // thing it needs of it: a way to say it is done.
        // A panel holding the flag itself would be the arrangement written in two places.
        Watch = new WatchSettingsViewModel(form, session, dispatch, CloseWatchSettings);

        // A toggle because the reader means either way.
        // The close path names the state instead (CloseWatchSettings).
        ToggleWatchSettings = new DelegateCommand(() =>
        {
            IsWatchSettingsOpen = !IsWatchSettingsOpen;
            Apply();
        });

        // News that the draft or the form behind it moved: the legs a row offers and the leg a tile opens on
        // are both read off it.
        // Raised on the UI loop by the form session, so there is nothing to marshal here.
        _form.Changed += Apply;

        Apply();
    }

    // --- Outputs --------------------------------------------------------------------

    private string _shownSummary = "";
    private string _notice = "";
    private bool _hasNotice;
    private string _refusal = "";
    private bool _hasRefusal;
    private bool _hasStreams;

    /// <summary>One row per path the relay carries, in the order it listed them.</summary>
    public ObservableCollection<StreamRowViewModel> Streams { get; }

    /// <summary>
    /// The tiles on screen, in the order they were added.
    /// A stream leaves the grid when its row is toggled off and not when the relay stops carrying it, so a
    /// stream that dropped out and came back keeps the tile the reader put there.
    /// </summary>
    public ObservableCollection<TileViewModel> Tiles { get; }

    private LayoutMode _mode;
    private string _focused = "";
    private string _fullscreen = "";

    /// <summary>
    /// How the tiles are arranged.
    /// It follows the focus rather than being chosen separately: Focus with nothing focused is a state the
    /// screen has no drawing for.
    /// </summary>
    public LayoutMode Mode { get => _mode; private set => Set(ref _mode, value); }

    /// <summary>
    /// The stream that has focus, empty when none has.
    ///
    /// <b>A name and not a tile.</b> A stream that drops out keeps its focus and its slot and comes back into
    /// the place the reader put it, which a reference to an object the drop threw away could not do.
    /// </summary>
    public string Focused { get => _focused; private set => Set(ref _focused, value); }

    /// <summary>
    /// The stream the main window is drawing fullscreen, empty when it is not.
    ///
    /// <b>Fullscreen is a property of a window, not of the app.</b> This is the main window's and each
    /// popped-out window carries its own, so several windows fill several monitors at once.
    /// That is why it is not a member of <see cref="LayoutMode"/>: a mode says how tiles sit relative to each
    /// other, this says which window one of them fills.
    ///
    /// The rail and the grid go, the shell takes its own bands off the window
    /// (<c>Features/Shell/ViewModel/ShellViewModel.cs</c>), and the picture is letterboxed on black at its
    /// stream's shape rather than stretched to the monitor's.
    /// </summary>
    public string Fullscreen { get => _fullscreen; private set => Set(ref _fullscreen, value); }

    /// <summary>
    /// The streams that should be drawn in windows of their own, as of this pass.
    ///
    /// A view opens and closes windows to match, which is an idempotent apply: a pass whose set is unchanged
    /// opens nothing and closes nothing.
    /// </summary>
    public IReadOnlyCollection<string> PoppedOut => _popped;

    /// <summary>Which of those windows should be filling their screen.</summary>
    public IReadOnlyCollection<string> PoppedFullscreen => _poppedFullscreen;

    private bool _hasFullscreen;
    private TileViewModel? _fullscreenTile;
    private bool _isRailCollapsed;

    /// <summary>Whether a tile fills this window, which is what takes the rail and the grid off it.</summary>
    public bool HasFullscreen { get => _hasFullscreen; private set => Set(ref _hasFullscreen, value); }

    public TileViewModel? FullscreenTile { get => _fullscreenTile; private set => Set(ref _fullscreenTile, value); }

    /// <summary>
    /// Whether the rail shows names or has been collapsed to its toggle.
    ///
    /// A reader watching a wall of streams wants the width, a reader looking for another wants the list.
    /// Collapsing is how one window is both, and it is this shell's own state like everything else about the
    /// arrangement.
    /// </summary>
    public bool IsRailCollapsed { get => _isRailCollapsed; private set => Set(ref _isRailCollapsed, value); }

    /// <summary>
    /// Collapsed is wide enough for an entry with its name taken out: the dot, the action button, the gaps
    /// between them and the padding around them, with the list's scrollbar cleared.
    /// It clears the rail's header buttons too, which are narrower than that.
    /// An entry therefore keeps its shape and loses only its name.
    /// </summary>
    public double RailWidth => IsRailCollapsed ? 88 : 240;

    /// <summary>One glyph that says which way the toggle goes.</summary>
    public Icons RailGlyph => IsRailCollapsed ? Icons.IconChevronRight : Icons.IconChevronLeft;

    /// <summary>What the rail's toggle will do, since a glyph is not a sentence.</summary>
    public string RailToggleTip => IsRailCollapsed ? "Show the stream names" : "Collapse the rail";

    public DelegateCommand ToggleRail { get; }

    /// <summary>
    /// Gives the main window back to its grid, whether or not a stream was filling it.
    ///
    /// The window's key rather than a tile's: a filled window draws no rail, no menu and no band, so the
    /// keyboard is what a reader can still reach (<c>Features/Shell/View/ShellWindow.axaml</c>).
    /// </summary>
    public DelegateCommand LeaveFullscreen { get; }

    private bool _isWatchSettingsOpen;

    /// <summary>
    /// How this machine receives: the legs, the jitter buffers and the render chain.
    /// One group of the same resolved form the setup wizard draws its steps from, placed here because this is
    /// the screen its settings govern.
    /// </summary>
    public WatchSettingsViewModel Watch { get; }

    /// <summary>This shell's own state, like everything else about the arrangement: the contract describes no panel.</summary>
    public bool IsWatchSettingsOpen { get => _isWatchSettingsOpen; private set => Set(ref _isWatchSettingsOpen, value); }

    public DelegateCommand ToggleWatchSettings { get; }

    /// <summary>
    /// Shuts the settings panel, whether or not it was open.
    ///
    /// Names the state rather than a transition, which is what lets the panel's own close button and its
    /// commit both run it: a commit that closed by toggling would reopen a panel the reader had already
    /// dismissed.
    /// </summary>
    private void CloseWatchSettings()
    {
        IsWatchSettingsOpen = false;
        Apply();
    }

    /// <summary>What the settings toggle will do, since a glyph is not a sentence.</summary>
    public string WatchSettingsTip => IsWatchSettingsOpen ? "Close the watching settings" : "How this machine receives";

    /// <summary>For a view that has to hand a tile to a window it is opening.</summary>
    public TileViewModel? TileOf(string stream) => _tiles.GetValueOrDefault(stream);

    /// <summary>
    /// Raised after a pass in which the windows a view should be showing changed.
    ///
    /// Separate from the render notification because nothing binds a window into existence: this is the one
    /// thing a view is told imperatively, and everything else is read off the properties above.
    /// </summary>
    public event Action? WindowsChanged;

    /// <summary>
    /// How much of what the relay carries this machine is watching.
    /// The status band prints it.
    /// </summary>
    public string ShownSummary { get => _shownSummary; private set => Set(ref _shownSummary, value); }

    /// <summary>
    /// The relay's cost, as the status band prints it.
    /// A list rather than named slots, so what a destination reports stays that destination's business.
    /// </summary>
    public IReadOnlyList<string> Figures { get; private set; } = [];

    /// <summary>Stands in for the list while it is empty, and tells an unread relay from an idle one.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    /// <summary>The backend's own sentence when it refused to open or close something, empty otherwise.</summary>
    public string Refusal { get => _refusal; private set => Set(ref _refusal, value); }

    public bool HasRefusal { get => _hasRefusal; private set => Set(ref _hasRefusal, value); }

    public bool HasStreams { get => _hasStreams; private set => Set(ref _hasStreams, value); }

    /// <summary>
    /// Printed by the status band rather than over the grid.
    /// The gestures it names are the tile's own (<c>Features/Viewer/Tile/View/TileKeys.cs</c>).
    /// </summary>
    public string Hint => "Right-click a tile for focus, pop-out and volume; double-click one to fill the screen, Escape to leave";

    /// <summary>Heading over the rail's list.</summary>
    public string ShowingLabel => "On the relay";

    /// <summary>
    /// The field the player legs are read off.
    /// Named once rather than typed at the site that reads it, for the reason <see cref="TileLeg.Key"/> is: a
    /// rename in the contract is one line.
    /// </summary>
    private const string PlayerLegKey = "viewer.player_watch_transport";

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// The one render function.
    /// Reads the session and the form through, and keeps no copy of either.
    /// Safe to run twice: a row is reused by stream name and renders itself idempotently, so an unchanged
    /// snapshot fires no binding.
    /// </summary>
    public void Apply()
    {
        // The pass names the state it wants, a form resolved from the draft as it stands, and the sync
        // decides whether a round trip is owed (docs/development-principles.md, "Idempotency").
        _form.Sync();

        // Read through on every pass rather than held, so a leg changed in the settings panel beside this
        // grid reaches the rows and the next decode without anything here being told.
        var legs = LegsOf(_form.Form);
        var browserLegs = BrowserLegsOf(_session.BrowserLegs);
        var relay = _session.Relay;
        var rows = Rows(relay, _session.Watching);

        foreach (var row in rows)
        {
            Of(row.Name).Apply(row, legs, browserLegs, _tiles.ContainsKey(row.Name));
        }

        Reconcile.Onto(Streams, rows.Select(row => Of(row.Name)).ToList());

        // The one way out of Focus the reader did not ask for: a mode whose subject left the grid has nothing
        // to show.
        // A stream that merely stopped publishing keeps its tile, which is a slot with a reason written in
        // it.
        if (Focused.Length > 0 && !_tiles.ContainsKey(Focused))
        {
            Focused = "";
        }

        // A window whose stream left the grid has nothing behind it, and a fullscreen state with no window
        // has nothing to answer for.
        _popped.RemoveWhere(stream => !_tiles.ContainsKey(stream));
        _poppedFullscreen.RemoveWhere(stream => !_popped.Contains(stream));
        if (Fullscreen.Length > 0 && !_tiles.ContainsKey(Fullscreen))
        {
            Fullscreen = "";
        }

        // This window's fullscreen names a stream of this window's own grid, so a stream that left for a
        // window of its own gives this one back.
        // Fullscreen does not travel with the stream: the popped-out window carries its own, and one that
        // arrived already filling a screen is a state the reader did not ask for.
        if (_popped.Contains(Fullscreen))
        {
            Fullscreen = "";
        }

        Mode = Focused.Length > 0 ? LayoutMode.Focus : LayoutMode.Grid;

        // Rendered from the backend's decode list, joined on the pair the contract keys a decode by.
        // A tile whose decode is not in it draws its own reason rather than disappearing: the reader put it
        // there, and a stream that dropped out is a thing to say instead of a thing to hide.
        foreach (var tile in _tiles.Values)
        {
            tile.Apply(TilePipeline.Of(DecodeOf(tile)), _session.StatsOf(tile.Name, tile.Transport));
            tile.IsFocused = tile.Name == Focused;
            tile.IsPoppedOut = _popped.Contains(tile.Name);

            // Which window a stream is drawn in decides which fullscreen state answers for it, the split
            // Arrange writes through.
            // Derived here rather than kept on the tile, so the flag the menu ticks and the state the windows
            // obey cannot disagree.
            tile.IsFullscreen = _popped.Contains(tile.Name)
                ? _poppedFullscreen.Contains(tile.Name)
                : Fullscreen == tile.Name;
        }

        HasStreams = Streams.Count > 0;
        Notice = HasStreams ? "" : NoticeFor(relay);
        HasNotice = Notice.Length > 0;

        var watched = rows.Count(row => row.IsWatched);
        ShownSummary = HasStreams ? $"{watched} of {rows.Count} streams watched" : "";
        Figures = FiguresFor(rows);

        HasRefusal = Refusal.Length > 0;

        FullscreenTile = Fullscreen.Length > 0 ? _tiles.GetValueOrDefault(Fullscreen) : null;
        HasFullscreen = FullscreenTile is not null;

        // The panel draws from the same draft on every pass, so a vocabulary that arrived with the catalog
        // reaches its entries through this call rather than through a notification of its own.
        Watch.Apply();

        // Raised by hand: a property with no field of its own has nothing to compare against.
        OnPropertyChanged(nameof(RailWidth));
        OnPropertyChanged(nameof(RailGlyph));
        OnPropertyChanged(nameof(RailToggleTip));
        OnPropertyChanged(nameof(WatchSettingsTip));

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
        Assert.That(HasRefusal == (Refusal.Length > 0), "a refusal and its sentence agree", HasRefusal);
    }

    /// <summary>
    /// One row per relay path, carrying the legs this machine already has open on it.
    /// Joined on the name the backend uses on both sides, so nothing here has to know what a transport or a
    /// format is.
    /// </summary>
    private static IReadOnlyList<StreamRow> Rows(RelayStatus? relay, IReadOnlyList<WatchKey> watching)
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
    /// Why there is nothing to list.
    /// An unread relay, an unreachable one and an idle one are states a reader has to tell apart, and an
    /// unreachable one says why in the relay's own words.
    /// </summary>
    private string NoticeFor(RelayStatus? relay)
    {
        if (relay is null)
        {
            return _session.Unavailable.Length > 0
                ? _session.Unavailable
                : "Reading what the relay is carrying.";
        }

        if (!relay.Reachable)
        {
            return relay.Error.Length > 0 ? relay.Error : "The relay could not be reached.";
        }

        return "The relay is up and carrying nothing.";
    }

    /// <summary>
    /// What the band prints: the relay's total ingest, and its readers across every path.
    /// Both are the relay's own figures and neither is this machine's decode, which is reported per tile in
    /// the stats panel.
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
    /// The backend's state for one tile's decode, null while nothing is decoding that pair.
    /// Joined on the stream name and the leg together, because the relay re-serves each stream on all its
    /// listeners and the name alone is not an identity.
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
    /// The player legs a form offers, read off the field the contract names for them.
    /// A form that has not arrived, or one that does not carry the field, leaves the roster offering nothing
    /// rather than a protocol guessed here.
    ///
    /// A read of the vocabulary and not of a per-stream verdict, which is why one answer serves every row:
    /// what a leg is called is the same for all of them, and whether one carries a given stream is answered
    /// as that viewer is opened.
    ///
    /// <b>Whether the option is reachable travels with it.</b> An entry the availability pass ruled out
    /// arrives greyed carrying the sentence that says why, and dropping either would offer a leg this machine
    /// has no player for as though it were live (<c>docs/field-availability.md</c>).
    /// Neither is decided here: both are read off the option the backend sent, as the generic renderer reads
    /// them for a dropdown.
    /// </summary>
    private static IReadOnlyList<WatchLeg> LegsOf(Form? form)
    {
        if (form is null)
        {
            return [];
        }

        foreach (var group in form.Groups)
        {
            foreach (var field in group.Fields)
            {
                if (field.Key != PlayerLegKey)
                {
                    continue;
                }

                // The roster is the backend's and the words are this side's: which legs a viewer opens on is
                // a fact, what a protocol is called on a row is a decision about this screen.
                // The reason splits the same way, a code from the backend and a sentence from here.
                return field.Options
                    .Select(option => new WatchLeg(
                        option.Value,
                        Copy.Words.Transport(option.Value),
                        option.Enabled,
                        Copy.Statements.Of(option.Reason)))
                    .ToList();
            }
        }

        return [];
    }

    /// <summary>
    /// The browser legs, named for this screen.
    /// The list is the catalog's and the words are this side's, the split <see cref="LegsOf"/> makes for the
    /// player legs.
    ///
    /// They carry no verdict, which is the catalog's shape rather than a default chosen here: it names the
    /// legs the relay serves a page for and answers nothing about the ones it does not, so there is no
    /// greying to pass on.
    /// </summary>
    private static IReadOnlyList<WatchLeg> BrowserLegsOf(IReadOnlyList<string> legs)
        => legs
            .Select(leg => new WatchLeg(leg, Copy.Words.Transport(leg), IsEnabled: true, Reason: ""))
            .ToList();

    /// <summary>
    /// Puts one stream in the grid, or takes it out.
    ///
    /// <b>Two calls in each direction, and the order is the point.</b> The decode opens first and the tile is
    /// added once that answered, because a tile is a subscription to frames and there are none until
    /// something decodes.
    /// On the way out the tile goes first: a subscription outliving its decode is a window holding handles to
    /// memory the backend has freed.
    ///
    /// <b>The tile set is written here, which is this screen's one departure from reading everything
    /// through.</b> The contract describes no grid, so there is nothing to read a tile list back from
    /// (<c>docs/ipc-api.md</c>).
    ///
    /// <b>The two directions name their leg from two places.</b> A start opens the leg the stored settings
    /// say a tile receives on; a stop closes the leg the tile was opened on, which is the tile's own and can
    /// be an older setting.
    /// Stopping on the current setting would leave a decode running whenever the leg had moved since, because
    /// a decode is keyed by the stream and the leg together.
    ///
    /// <b>Stored and not the draft.</b> The leg is the only knob of the watch group this call names: the
    /// backend reads the render chain and the jitter buffers out of its own settings as it builds the decode.
    /// Opening on the draft therefore ran an unkept leg against kept buffers, a pipeline the reader never
    /// asked for (<c>Features/Viewer/Tile/Model/TileLeg.cs</c>).
    /// </summary>
    private async Task TileAsync(string stream, bool tiled)
    {
        if (tiled)
        {
            // Read before the tile goes, because the tile is where the answer is.
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
            Refused("The settings have not said which protocol a tile receives on yet.");
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
            // Shown as it arrived: a leg that cannot carry this stream's format names the format and the
            // protocols that would have carried it, which is what makes the refusal actionable.
            Refused(e.Message);
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Hands each tile its own decode's loudness.
    ///
    /// <b>Not part of <see cref="Apply"/>.</b> Levels arrive fifteen times a second, and running the render
    /// pass at that rate would re-read the relay, the roster and every decode to move a bar.
    /// This walks the tiles and writes only what the meter binds (<c>Backend/Session.cs</c>,
    /// <c>Metered</c>).
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
    ///
    /// <b>Every intent is decided here.</b> Focus is at most one tile, a pop-out moves a stream between
    /// windows, and fullscreen names the stream a window fills: each is a fact about the whole arrangement,
    /// so a tile raises the intent and writes none of it (<c>Features/Viewer/Model/TileIntent.cs</c>).
    /// </summary>
    private void Arrange(string stream, TileIntent intent)
    {
        Assert.That(stream.Length > 0, "an arrangement is asked for by a tile that names its stream");

        switch (intent)
        {
            case TileIntent.Focus:
                // A second stream asking for focus takes it.
                // No state has two focused, so none has one to be taken away first.
                Focused = Focused == stream ? "" : stream;
                break;

            case TileIntent.PopOut:
                // The decode is untouched either way.
                // Closing a pop-out returns the stream to its slot rather than stopping it: stopping is the
                // rail's toggle and is undone differently.
                if (!_popped.Remove(stream))
                {
                    _popped.Add(stream);
                }

                break;

            case TileIntent.Fullscreen:
                // Fullscreen is a property of a window, so which window this stream is drawn in decides which
                // state moves.
                // A popped-out stream fills its own window's screen and several do that at once; a stream in
                // the grid fills the main window, of which there is one.
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
                // The decode is untouched here too: a window that closed is a picture that moved, not a
                // stream that stopped.
                _popped.Remove(stream);
                break;

            case TileIntent.LeaveFullscreen:
                // The same split read the other way.
                // It names the state a window is to be in and never the transition, so a stream filling
                // nothing is left alone and the key is safe wherever it fires.
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
    /// The leg is passed in rather than read again, so the tile is keyed by the pair the backend keyed the
    /// decode by even where the setting moved in between.
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
        // The pass it asks for is this screen's, so the figures over a tile and the rail beside it stay one
        // render function.
        tile.Changed += Apply;

        _tiles[stream] = tile;
        Tiles.Add(tile);
        Apply();
    }

    /// <summary>
    /// Takes one tile off the grid and re-renders.
    ///
    /// Removing it from the collection is what ends the frame subscription: the control drawing it detaches
    /// with it, and the detach cancels the call and disposes every handle it imported
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
        // Apply drops the focus, the pop-out and the fullscreen of a stream that left the grid, so a stream
        // leaving is worked out in one place rather than one here and one there.
        Apply();
    }

    /// <summary>
    /// Opens or closes one external viewer.
    /// No local copy of the roster is edited: the roster is the backend's list and is read from there.
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

            // The backend announces a viewer that ended and not one that started, so the roster is re-read
            // here rather than patched with what was just sent.
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
    ///
    /// Nothing is re-read afterwards, which is the difference from <see cref="WatchAsync"/> rather than an
    /// omission: no list moved.
    /// The page opens in a program this app does not own, so what came of it is a state neither side can
    /// report.
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
    /// An empty reason is a success and clears whatever the last failure left, the render function's usual
    /// property applied to a string.
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
