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
/// What the relay is carrying, and what this machine is watching of it.
///
/// <b>This is a roster and not a tile grid, and the difference is the frame channel.</b> A grid shows decoded
/// pictures and per-tile decode figures; both need frames, and frames deliberately do not cross the control
/// API - they are a second channel of shared GPU handles that does not exist yet (<c>docs/ipc-api.md</c>, and
/// <c>avalonia/README.md</c>, "What is not settled yet").
/// Everything the control API does carry about a session is here and is real: which streams the relay has,
/// whether each is being served, what it says they carry, how many readers each has, what each is ingesting,
/// and which of them this machine has viewers open on.
/// The grid layout, the spotlight and the per-tile menus were removed rather than left drawing mockup figures
/// beside real ones.
///
/// There is no longer a button for the GTK4 grid window either, and its absence is the contract's rather than
/// this module's.
/// How a viewer arranges what it receives is the shell's whole job, so the backend describes no grid to open,
/// close or report the state of (<c>docs/ipc-api.md</c>).
/// When frames do cross, the window that shows them is this module's to build and not one it asks for by
/// name.
///
/// <b>It holds no snapshot of its own.</b> <see cref="Backend.Session"/> owns the running state and
/// <see cref="Apply"/> reads it through on every pass, so the list and the status band cannot disagree about
/// what is on the relay.
///
/// The legs a stream can be opened on come from the backend too: they are the options of the form's watch-leg
/// field, so this module holds no list of protocols.
/// Whether a particular leg can carry a particular stream is answered by the backend when the viewer is
/// opened, and its refusal is shown as it stands - the relay snapshot can be older than the stream, so
/// greying a leg here from a stale format would refuse a viewer that would have worked.
///
/// The settings behind those legs are edited here as well, in the panel beside the grid.
/// They govern how this machine receives and nothing about what it sends, so this is the screen they belong
/// to (<c>Features/Fields/Model/GroupPlacement.cs</c>).
/// </summary>
public sealed class ViewerViewModel : Observable
{
    private readonly IBackend _backend;

    /// <summary>
    /// The draft and the form it resolves to, owned by the window.
    /// Two things are read off it and neither is kept: the legs a stream can be opened on, and the leg a tile
    /// decodes over.
    ///
    /// It used to be a read of its own - this screen fetched the settings and resolved a form once, on mount
    /// - and that was a second copy of the one thing the setup wizard was already holding: a leg changed in
    /// the wizard did not reach the tiles until the window was reopened.
    /// Reading through is what removes that (<c>docs/development-principles.md</c>, "A reader reads
    /// through").
    /// </summary>
    private readonly FormSession _form;

    private readonly Session _session;
    private readonly Action<Action> _dispatch;
    private readonly Dictionary<string, StreamRowViewModel> _rows = [];

    /// <summary>
    /// The tiles this window has put on screen, by stream name.
    ///
    /// <b>The grid is this shell's alone.</b> Which streams are decoded is the backend's list and is read
    /// through on every pass; which of them are drawn, and in what arrangement, is not on the contract at all
    /// and could not be - a window is the one thing that contract may not describe (<c>docs/ipc-api.md</c>).
    /// </summary>
    private readonly Dictionary<string, TileViewModel> _tiles = [];

    /// <summary>
    /// The streams being drawn in windows of their own, by name.
    ///
    /// <b>A set of names and not a set of windows.</b> This is state; the windows are what a view reconciles
    /// against it on every pass, which is what makes opening one idempotent and what keeps a toolkit type out
    /// of a view model (<c>avalonia/README.md</c>).
    ///
    /// A popped stream keeps its tile in the grid.
    /// The tile becomes a plate saying where the picture went, still counted in the arrangement at its own
    /// shape, so nothing reflows when a stream pops out or comes back.
    /// </summary>
    private readonly HashSet<string> _popped = [];

    /// <summary>
    /// The popped-out streams whose own window should be fullscreen.
    ///
    /// A second set rather than a flag on the first, because it answers a different question: which windows
    /// exist, and which of those fill their screen.
    /// Several can be fullscreen at once, each on the monitor its window is already on, which is exactly the
    /// arrangement a single app-wide fullscreen could not express.
    /// </summary>
    private readonly HashSet<string> _poppedFullscreen = [];

    /// <param name="dispatch">
    /// Hands work to the UI loop.
    /// Every answer to an effect lands on whichever thread the transport completed on, and everything this
    /// writes is read by a binding that only tolerates being written from one.
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

        // Escape gives the main window back to its grid.
        // It names that state rather than toggling one, so a press with nothing filling the window is a pass
        // that changes nothing - and a press on another screen is the same pass.
        // Each popped-out window answers for its own fullscreen, which is why this clears the main window's
        // and no more than that.
        LeaveFullscreen = new DelegateCommand(() =>
        {
            Fullscreen = "";
            Apply();
        });

        // The settings that govern how this machine receives, beside the tiles they govern.
        // They used to be a step of the setup wizard, which is the screen for what this machine sends
        // (Features/Fields/Model/GroupPlacement.cs).
        //
        // Whether the panel is open is this screen's state and stays here, so the panel is handed the one
        // thing it needs of it: a way to say it is done.
        // A panel holding the flag itself would be the arrangement written in two places.
        Watch = new WatchSettingsViewModel(form, session, dispatch, CloseWatchSettings);

        // Names the state rather than the transition, so a close with the panel already shut is a pass that
        // changes nothing.
        ToggleWatchSettings = new DelegateCommand(() =>
        {
            IsWatchSettingsOpen = !IsWatchSettingsOpen;
            Apply();
        });

        // News that the draft or the form behind it moved: the legs a row offers and the leg a tile is opened
        // on are both read off it.
        // Raised on the UI loop by the form session itself, so there is nothing to marshal here.
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

    /// <summary>The streams the relay is carrying, in the order it listed them.</summary>
    public ObservableCollection<StreamRowViewModel> Streams { get; }

    /// <summary>
    /// The tiles on screen, in the order they were added.
    /// A stream leaves the grid when its row is toggled off and not when the relay stops carrying it: a
    /// stream that dropped out and came back keeps the tile the reader put there.
    /// </summary>
    public ObservableCollection<TileViewModel> Tiles { get; }

    private bool _hasTiles;

    /// <summary>Whether anything is in the grid, which is what separates it from its empty state.</summary>
    public bool HasTiles { get => _hasTiles; private set => Set(ref _hasTiles, value); }

    private LayoutMode _mode;
    private string _focused = "";
    private string _fullscreen = "";

    /// <summary>
    /// How the tiles are arranged.
    /// It follows the focus rather than being chosen separately: a mode saying Focus with nothing focused
    /// would be a state the screen has no drawing for.
    /// </summary>
    public LayoutMode Mode { get => _mode; private set => Set(ref _mode, value); }

    /// <summary>
    /// The stream that has focus, empty when none has.
    ///
    /// <b>A name and not a tile.</b> A stream that drops out keeps its focus and its slot, and comes back
    /// into the place the reader put it - which it could not do if focus were a reference to an object the
    /// drop had thrown away.
    /// </summary>
    public string Focused { get => _focused; private set => Set(ref _focused, value); }

    /// <summary>
    /// The stream the main window is drawing fullscreen, empty when it is not.
    ///
    /// <b>Fullscreen is a property of a window, not of the app.</b> This is the main window's; each
    /// popped-out window carries its own, so three windows can be fullscreen on three monitors at once.
    /// That is why it is not a member of <see cref="LayoutMode"/>: a mode says how tiles sit relative to each
    /// other, and this says which window one of them fills.
    ///
    /// What the window then draws is the stream and nothing else.
    /// The rail and the grid go, the shell takes its own bands off the window
    /// (<c>Features/Shell/ViewModel/ShellViewModel.cs</c>), and the picture is letterboxed on black at its
    /// stream's shape - a screen given to the stream rather than an app drawn larger.
    /// </summary>
    public string Fullscreen { get => _fullscreen; private set => Set(ref _fullscreen, value); }

    /// <summary>
    /// The streams that should be drawn in windows of their own, as of this pass.
    ///
    /// A view opens and closes windows to match it, which is an idempotent apply: a pass whose set is
    /// unchanged opens nothing and closes nothing.
    /// </summary>
    public IReadOnlyCollection<string> PoppedOut => _popped;

    /// <summary>Which of the popped-out windows should be filling their screen.</summary>
    public IReadOnlyCollection<string> PoppedFullscreen => _poppedFullscreen;

    private bool _hasFullscreen;
    private TileViewModel? _fullscreenTile;
    private bool _isRailCollapsed;

    /// <summary>Whether one tile is filling this window, which is what hides the rail and the grid.</summary>
    public bool HasFullscreen { get => _hasFullscreen; private set => Set(ref _hasFullscreen, value); }

    /// <summary>The tile filling this window, null when none is.</summary>
    public TileViewModel? FullscreenTile { get => _fullscreenTile; private set => Set(ref _fullscreenTile, value); }

    /// <summary>
    /// Whether the rail is showing names or has been collapsed to its toggle.
    ///
    /// A reader watching six streams wants the width; a reader looking for a seventh wants the list.
    /// Collapsing is how one window is both, and it is this shell's own state like every other thing about
    /// the arrangement.
    /// </summary>
    public bool IsRailCollapsed { get => _isRailCollapsed; private set => Set(ref _isRailCollapsed, value); }

    /// <summary>
    /// How wide the rail is drawn, which is the collapsed width or the full one.
    ///
    /// Collapsed is wide enough for an entry with its name taken out: the dot, the action button, the gaps
    /// between them and the padding around them, with the list's scrollbar cleared.
    /// It also clears the two buttons in the rail's header, which are narrower than that.
    /// An entry therefore keeps its shape and only loses its name.
    /// </summary>
    public double RailWidth => IsRailCollapsed ? 88 : 240;

    /// <summary>Open or closed, as the one glyph that says which way the toggle goes.</summary>
    public Icons RailGlyph => IsRailCollapsed ? Icons.IconChevronRight : Icons.IconChevronLeft;

    /// <summary>What the rail's toggle says it will do, since the glyph alone is not a sentence.</summary>
    public string RailToggleTip => IsRailCollapsed ? "Show the stream names" : "Collapse the rail";

    /// <summary>Collapses the rail to its toggle, or opens it again.</summary>
    public DelegateCommand ToggleRail { get; }

    /// <summary>
    /// Gives the main window back to its grid, whether or not a stream was filling it.
    ///
    /// It is the window's key rather than a tile's, because a filled window draws no rail, no menu and no
    /// band: what a reader can still reach in that state is the keyboard
    /// (<c>Features/Shell/View/ShellWindow.axaml</c>).
    /// </summary>
    public DelegateCommand LeaveFullscreen { get; }

    private bool _isWatchSettingsOpen;

    /// <summary>
    /// How this machine receives: the legs, the jitter buffers and the render chain.
    /// It is one group of the same resolved form the setup wizard draws its steps from, placed here because
    /// this is the screen its settings govern.
    /// </summary>
    public WatchSettingsViewModel Watch { get; }

    /// <summary>
    /// Whether the settings panel is open.
    /// This shell's own state, like every other thing about the arrangement: the contract describes no panel
    /// and could not.
    /// </summary>
    public bool IsWatchSettingsOpen { get => _isWatchSettingsOpen; private set => Set(ref _isWatchSettingsOpen, value); }

    /// <summary>Opens the settings panel, or closes it again.</summary>
    public DelegateCommand ToggleWatchSettings { get; }

    /// <summary>
    /// Shuts the settings panel, whether or not it was open.
    ///
    /// It names the state rather than a transition, which is what lets the panel's own close button and its
    /// commit both run it: a commit that closed by toggling would reopen a panel the reader had already
    /// dismissed.
    /// </summary>
    private void CloseWatchSettings()
    {
        IsWatchSettingsOpen = false;
        Apply();
    }

    /// <summary>What the settings toggle says it will do, since the glyph alone is not a sentence.</summary>
    public string WatchSettingsTip => IsWatchSettingsOpen ? "Close the watching settings" : "How this machine receives";

    /// <summary>The tile for one stream, for a view that has to hand it to a window it is opening.</summary>
    public TileViewModel? TileOf(string stream) => _tiles.GetValueOrDefault(stream);

    /// <summary>
    /// Raised after a pass in which the windows a view should be showing changed.
    ///
    /// Separate from the ordinary render notification because opening a window is not a binding: nothing can
    /// bind a window into existence, so the one thing a view has to be told imperatively is told here and
    /// everything else is read off the properties above.
    /// </summary>
    public event Action? WindowsChanged;

    /// <summary>How much of what the relay carries this machine is watching. The status band repeats it.</summary>
    public string ShownSummary { get => _shownSummary; private set => Set(ref _shownSummary, value); }

    /// <summary>
    /// What the relay costs, as the status band prints it.
    /// A list rather than named slots, so what this destination reports stays this destination's business.
    /// </summary>
    public IReadOnlyList<string> Figures { get; private set; } = [];

    /// <summary>Why the list is empty, empty while it is not. It separates an unread relay from an idle one.</summary>
    public string Notice { get => _notice; private set => Set(ref _notice, value); }

    public bool HasNotice { get => _hasNotice; private set => Set(ref _hasNotice, value); }

    /// <summary>The backend's own sentence when it refused to open or close a viewer, empty otherwise.</summary>
    public string Refusal { get => _refusal; private set => Set(ref _refusal, value); }

    public bool HasRefusal { get => _hasRefusal; private set => Set(ref _hasRefusal, value); }

    public bool HasStreams { get => _hasStreams; private set => Set(ref _hasStreams, value); }

    /// <summary>
    /// The status band's affordance.
    /// It names what this screen affords rather than what a tile grid would, because this screen has no
    /// tiles.
    /// </summary>
    public string Hint => "Right-click a tile for focus, pop-out and volume; double-click one to fill the screen, Escape to leave";

    /// <summary>The rail's leading label.</summary>
    public string ShowingLabel => "On the relay";

    /// <summary>
    /// The field the legs a player can be opened on are read off.
    /// Named once here rather than typed at the site that reads it, for the reason <see cref="TileLeg.Key"/>
    /// is: a rename in the contract is one line.
    /// </summary>
    private const string PlayerLegKey = "viewer.player_watch_transport";

    // --- Lifecycle ------------------------------------------------------------------

    /// <summary>
    /// The one render function.
    /// Reads the session through on every pass and keeps no copy of what it found.
    /// Safe to run twice: rows are reused by stream name and each runs its own idempotent pass, so an
    /// unchanged relay snapshot fires no binding.
    /// </summary>
    public void Apply()
    {
        // Reconciled from the render pass rather than performed by it: the pass states that it wants a form
        // to read the legs off, and the converge decides whether anything has to be asked
        // (docs/development-principles.md, "Idempotency").
        _form.Sync();

        // Both read through on every pass rather than held, so a leg changed in the settings panel beside
        // this grid reaches the rows and the next decode without anything here having been told.
        var legs = LegsOf(_form.Form);
        var browserLegs = BrowserLegsOf(_session.BrowserLegs);
        var relay = _session.Relay;
        var rows = Rows(relay, _session.Watching);

        foreach (var row in rows)
        {
            Of(row.Name).Apply(row, legs, browserLegs, _tiles.ContainsKey(row.Name));
        }

        Reconcile.Onto(Streams, rows.Select(row => Of(row.Name)).ToList());

        // Focus is dropped where the stream it named is no longer being decoded.
        // That is the one way out of Focus that the reader did not ask for, and it is the right one: a mode
        // whose subject has gone has nothing to show, where a stream that merely stopped publishing is still
        // a slot with a reason written in it.
        if (Focused.Length > 0 && !_tiles.ContainsKey(Focused))
        {
            Focused = "";
        }

        // A stream popped out of a grid it is no longer in is a window with nothing behind it.
        _popped.RemoveWhere(stream => !_tiles.ContainsKey(stream));
        _poppedFullscreen.RemoveWhere(stream => !_popped.Contains(stream));
        if (Fullscreen.Length > 0 && !_tiles.ContainsKey(Fullscreen))
        {
            Fullscreen = "";
        }

        // This window's fullscreen names a stream drawn in this window, so a stream that left for a window of
        // its own gives this one back to its grid.
        // Fullscreen does not travel with the stream: the popped-out window carries its own, and one that
        // arrived already filling a screen is a state the reader did not ask for.
        if (_popped.Contains(Fullscreen))
        {
            Fullscreen = "";
        }

        Mode = Focused.Length > 0 ? LayoutMode.Focus : LayoutMode.Grid;

        // The tiles are rendered from the backend's decode list, joined on the pair the whole contract keys a
        // decode by.
        // A tile whose decode is not in it draws its own reason for that rather than disappearing: the reader
        // put it there, and a stream that dropped out is a thing to say instead of a thing to hide.
        foreach (var tile in _tiles.Values)
        {
            tile.Apply(TilePipeline.Of(DecodeOf(tile)), _session.StatsOf(tile.Name, tile.Transport));
            tile.IsFocused = tile.Name == Focused;
            tile.IsPoppedOut = _popped.Contains(tile.Name);

            // Which window a stream is drawn in decides which fullscreen state answers for it, the same split
            // Arrange writes through.
            // Read here rather than kept on the tile, so the flag the menu ticks and the state the windows
            // obey cannot disagree.
            tile.IsFullscreen = _popped.Contains(tile.Name)
                ? _poppedFullscreen.Contains(tile.Name)
                : Fullscreen == tile.Name;
        }

        HasTiles = Tiles.Count > 0;

        HasStreams = Streams.Count > 0;
        Notice = HasStreams ? "" : NoticeFor(relay);
        HasNotice = Notice.Length > 0;

        var watched = rows.Count(row => row.IsWatched);
        ShownSummary = HasStreams ? $"{watched} of {rows.Count} streams watched" : "";
        Figures = FiguresFor(rows);

        HasRefusal = Refusal.Length > 0;

        FullscreenTile = Fullscreen.Length > 0 ? _tiles.GetValueOrDefault(Fullscreen) : null;
        HasFullscreen = FullscreenTile is not null;

        // The settings panel draws from the same draft on every pass, so a vocabulary that arrived with the
        // catalog reaches its entries through this call rather than through a notification of its own.
        Watch.Apply();

        // The computed faces are raised by hand: a binding on a property with no field of its own has nothing
        // to compare.
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
    /// The join is on the name the backend uses on both sides, so nothing here has to know what a transport
    /// or a format is.
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
    /// Why there is nothing to list. Three states the reader has to be able to tell apart: the
    /// relay has not been read, it could not be reached - and then it says why, in its own words
    /// - or it is up and carrying nothing.
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
    /// What the band prints: what the relay is ingesting in total, and how many readers it has across every
    /// path.
    /// Both are the relay's own figures rather than this machine's - this shell receives nothing, so a
    /// receive rate here would be a number about somebody else.
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
    /// The backend's state for one tile's decode, and null while nothing is decoding that pair.
    /// The join is on the stream name and the leg together, because the relay re-serves each stream on all
    /// its listeners and the name alone is not an identity.
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
    /// The watch legs a form offers, read off the field the contract names for them.
    /// A form that has not arrived, or one that does not carry the field, leaves the roster offering nothing
    /// rather than a protocol guessed here.
    ///
    /// It is a read of the vocabulary rather than of a per-stream verdict, which is why one answer serves
    /// every row: what a leg is called is the same for all of them, and whether one carries a given stream is
    /// answered when that viewer is opened.
    ///
    /// <b>Whether the option is reachable travels with it.</b> An entry the availability pass ruled out
    /// arrives greyed carrying the sentence that says why, and dropping either would leave the menu offering
    /// a leg this machine has no player for as though it were live (<c>docs/field-availability.md</c>).
    /// Neither is decided here: both are read off the option the backend sent, exactly as the generic
    /// renderer reads them for a dropdown.
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

                // The roster is the backend's and the names are this side's: which legs a viewer can be
                // opened on is a fact, and what each protocol is called on a toggle is a decision about this
                // screen.
                // The reason is the same split again - the code is the backend's and the sentence is written
                // here.
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
    /// The list is the catalog's and the words are this side's, which is the split <see cref="LegsOf"/> makes
    /// for the player legs and for the same reason: which legs the relay serves a page for is a fact, and
    /// what a protocol is called on a menu row is a decision about this screen.
    ///
    /// They carry no verdict, and that is the catalog's shape rather than a default chosen here: it names the
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
    /// <b>Two calls in each direction, and the order is the point.</b> A decode is opened first and the tile
    /// is added once that has answered, because a tile is a subscription to frames and there are none until
    /// something is decoding; on the way out the tile goes first, because a subscription outliving its decode
    /// would be a window holding handles to memory the backend has freed.
    ///
    /// Adding a tile is the shell's own state and is written here, which is the one place this screen departs
    /// from reading everything through.
    /// That is the contract's doing rather than a shortcut: the backend describes no grid, so there is
    /// nothing to read a tile list back from.
    ///
    /// <b>The two directions name their leg from two places, and that is not a slip.</b> A start opens the
    /// leg the stored settings say a tile receives on; a stop closes the leg the tile was actually opened on,
    /// which is the tile's own and can be an older setting.
    /// Stopping on the current setting would leave a decode running whenever the leg had been changed since -
    /// a decode is keyed by the stream and the leg together, and the pair is what identifies it.
    ///
    /// <b>Stored and not the draft.</b> The leg is the only one of the watch group's six knobs this call
    /// names; the backend reads the render chain and both jitter buffers out of its own settings when it
    /// builds the decode.
    /// Opening on the draft therefore ran an unkept leg against kept buffers, which is a pipeline the reader
    /// never asked for (<c>Features/Viewer/Tile/Model/TileLeg.cs</c>).
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
            // The backend's own sentence: a leg that cannot carry this stream's format names the format and
            // the protocols that would have carried it, which is the whole of what makes the refusal
            // actionable.
            Refused(e.Message);
        }
        catch (OperationCanceledException)
        {
        }
    }

    /// <summary>
    /// Takes one measurement of every decode's loudness and gives each tile its own.
    ///
    /// <b>Not part of <see cref="Apply"/>.</b> Levels arrive fifteen times a second, and running the render
    /// pass at that rate would re-read the relay, the roster and every decode to move a bar.
    /// This walks the tiles and writes two properties on each (<c>Backend/Session.cs</c>, <c>Metered</c>).
    /// </summary>
    public void Meter()
    {
        foreach (var tile in _tiles.Values)
        {
            tile.Meter(_session.LevelOf(tile.Name, tile.Transport));
        }
    }

    /// <summary>
    /// Does what a tile's menu asked for.
    ///
    /// <b>Every one of them is decided here.</b> Focus is at most one tile, a pop-out moves a stream between
    /// windows, and fullscreen names the stream a window fills - three facts about the whole arrangement,
    /// which is why a tile raises the intent and writes none of it
    /// (<c>Features/Viewer/Model/TileIntent.cs</c>).
    /// </summary>
    private void Arrange(string stream, TileIntent intent)
    {
        Assert.That(stream.Length > 0, "an arrangement is asked for by a tile that names its stream");

        switch (intent)
        {
            case TileIntent.Focus:
                // A second stream asking for focus takes it.
                // There is no state in which two are focused, so there is no state in which one has to be
                // taken away first.
                Focused = Focused == stream ? "" : stream;
                break;

            case TileIntent.PopOut:
                // The decode is untouched either way.
                // Closing a pop-out returns the stream to its slot rather than stopping it, because stopping
                // is the rail's toggle and means something a reader would have to undo differently.
                if (!_popped.Remove(stream))
                {
                    _popped.Add(stream);
                }

                break;

            case TileIntent.Fullscreen:
                // Fullscreen is a property of a window, so which window this stream is drawn in decides which
                // state moves.
                // A popped-out stream fills the screen its own window is on, and several of them can do that
                // at once; a stream in the grid fills the main window, of which there is one.
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
                // The state a closed window reports, and a stream already in the grid is left where it is.
                // The decode is untouched here as well: a window that closed is a picture that moved, not a
                // stream that stopped.
                _popped.Remove(stream);
                break;

            case TileIntent.LeaveFullscreen:
                // The same split read the other way.
                // A stream that is not filling anything is left where it is: this names the state a window is
                // to be in and never the transition, so it is safe on a key that fires on every screen.
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
    /// The leg is the one the decode was opened on and is passed in rather than read again, so the tile is
    /// keyed by the pair the backend keyed the decode by even if the setting moved in between.
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
        // The pass it asks for is this screen's own, so the figures over a tile and the roster under it are
        // still written by one render function.
        tile.Changed += Apply;

        _tiles[stream] = tile;
        Tiles.Add(tile);
        Apply();
    }

    /// <summary>
    /// Takes one tile off the grid and re-renders.
    ///
    /// Removing it from the collection is what ends the subscription: the control that draws it is detached
    /// with it, and its detach cancels the call and disposes every handle it imported
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
        // Apply drops a focus, a pop-out and a fullscreen whose stream has left the grid, so this method
        // removes a tile and the render pass decides what that meant - one place where a stream leaving is
        // worked out, rather than one here and one there.
        Apply();
    }

    /// <summary>
    /// Opens or closes one external viewer.
    /// Nothing is written here on the way out: the roster is the backend's list and this reads it again when
    /// the event stream says it moved.
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
            // here.
            // It is a read of the backend's own list rather than an edit of a copy, which is what keeps one
            // opinion about what is open.
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
    /// Nothing is re-read afterwards, and that is the difference from <see cref="WatchAsync"/> rather than an
    /// omission: no list moved.
    /// A page is opened in a program this app does not own, so what came of it is not a state either side can
    /// report - only the refusal is, and it lands where every other one does.
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
    /// An empty reason is a success, which clears whatever the last failure left - the render function's
    /// usual property applied to a string.
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
