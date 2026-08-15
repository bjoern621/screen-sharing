using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.Model;
using ScreenShare.App.Mvvm;
using TablerIcons;

namespace ScreenShare.App.Features.Viewer.ViewModel;

/// <summary>
/// One stream on the relay, as the rail draws it: what it is, what it is carrying, and the legs this machine
/// can open a viewer on.
///
/// <b>Outputs only.</b> The row holds no input of its own: a press runs one of its commands, which asks the
/// backend and waits for the event stream to say what happened.
///
/// It is kept across passes and updated in place rather than rebuilt, so the commands stay the same instances
/// and an expanded row does not collapse under the pointer on every relay poll.
/// </summary>
public sealed class StreamRowViewModel : Observable
{
    private readonly Func<string, string, bool, Task> _watch;
    private readonly Func<string, string, Task> _browse;
    private readonly Action<Action> _dispatch;
    private readonly Dictionary<string, PendingCommand> _toggles = [];
    private readonly Dictionary<string, PendingCommand> _pages = [];

    /// <summary>Read when the command runs rather than captured, so one instance stays right across passes.</summary>
    private bool _tiled;

    /// <summary>Which legs are open, read by the commands this row hands out.</summary>
    private IReadOnlyList<string> _watchedOn = [];

    public StreamRowViewModel(
        string name,
        Func<string, string, bool, Task> watch,
        Func<string, bool, Task> tile,
        Func<string, string, Task> browse,
        Action<Action> dispatch)
    {
        Assert.That(name.Length > 0, "a stream row names the stream it stands for");
        Assert.NotNull(watch, "a stream row needs somewhere to ask for a viewer");
        Assert.NotNull(tile, "a stream row needs somewhere to ask for a tile");
        Assert.NotNull(browse, "a stream row needs somewhere to ask for a page in the browser");
        Assert.NotNull(dispatch, "a stream row needs a UI loop to marshal an answer back to");

        Name = name;
        _watch = watch;
        _browse = browse;
        _dispatch = dispatch;
        Legs = [];
        BrowserLegs = [];

        // Made once, like the leg commands and for the same reason: a row outlives a pass, and a command
        // rebuilt per pass is a button that loses its press.
        //
        // A tile is two calls and the decode behind them, so the toggle waits on the answer.
        // Without the wait, every press that lands while the first is being opened starts a second decode.
        Show = new PendingCommand(() => tile(Name, _tiled), dispatch);
    }

    /// <summary>Carried and never parsed.</summary>
    public string Name { get; }

    // --- Outputs ------------------------------------------------------------------

    private string _label = "";
    private string _detail = "";
    private string _tracks = "";
    private string _readers = "";
    private bool _isReady;
    private bool _isWatched;
    private bool _isTiled;

    /// <summary>Legs a viewer can be opened on, in the order the backend offered them.</summary>
    public ObservableCollection<WatchLegViewModel> Legs { get; }

    /// <summary>
    /// Legs the relay serves a player page for, in the order the backend offered them.
    ///
    /// A second list rather than a flag on the first: what a native player reaches and what a browser reaches
    /// are different sets, and neither contains the other.
    /// </summary>
    public ObservableCollection<BrowserLegViewModel> BrowserLegs { get; }

    /// <summary>
    /// The word the entry carries: the stream's own name, with the prefix every row of one group shares taken
    /// off by the backend.
    /// <see cref="Name"/> is the whole path and stays what the commands open.
    /// </summary>
    public string Label { get => _label; private set => Set(ref _label, value); }

    /// <summary>Ingest rate while the stream runs, and what the path is doing otherwise.</summary>
    public string Detail { get => _detail; private set => Set(ref _detail, value); }

    /// <summary>What the path carries, in the relay's own words.</summary>
    public string Tracks { get => _tracks; private set => Set(ref _tracks, value); }

    /// <summary>Readers the relay counts, as the row prints them.</summary>
    public string Readers { get => _readers; private set => Set(ref _readers, value); }

    /// <summary>Whether a publisher is connected. A path that is not ready draws dimmed.</summary>
    public bool IsReady { get => _isReady; private set => Set(ref _isReady, value); }

    /// <summary>Whether this machine has any external viewer open on this stream.</summary>
    public bool IsWatched { get => _isWatched; private set => Set(ref _isWatched, value); }

    /// <summary>
    /// Whether this stream has a tile in the grid.
    ///
    /// Not the same fact as <see cref="IsWatched"/>: a viewer is a player window the backend launched, a tile
    /// is a decode this window draws.
    /// One stream can have both at once, over different legs.
    /// </summary>
    public bool IsTiled { get => _isTiled; private set => Set(ref _isTiled, value); }

    /// <summary>Adds this stream to the grid, or removes it.</summary>
    public PendingCommand Show { get; }

    private string _watchLabel = "";

    /// <summary>
    /// What the action says it will do.
    ///
    /// It names the effect and not the state: the dot already says whether anything is publishing and the
    /// pressed toggle says whether the stream is in the grid, so a label repeating either would be a third
    /// opinion about one fact.
    /// </summary>
    public string WatchLabel { get => _watchLabel; private set => Set(ref _watchLabel, value); }

    private Icons _watchGlyph = Icons.IconPlayerPlay;

    /// <summary>
    /// The action's glyph: on screen, or off it.
    ///
    /// One glyph that changes rather than two controls one of which is hidden, so the action never moves
    /// under the pointer.
    /// </summary>
    public Icons WatchGlyph { get => _watchGlyph; private set => Set(ref _watchGlyph, value); }

    /// <summary>
    /// The one render function.
    /// Safe to run twice: every output is written on every pass, the legs of two passes over one row compare
    /// equal, and the commands are reused by value.
    /// </summary>
    public void Apply(StreamRow row, IReadOnlyList<WatchLeg> legs, IReadOnlyList<WatchLeg> browserLegs, bool tiled)
    {
        Assert.NotNull(row, "a row renders the stream the relay reported");
        Assert.That(row.Name == Name, "a row renders the stream it was made for", Name, row.Name);
        Assert.NotNull(legs, "a row offers the legs the backend named");
        Assert.NotNull(browserLegs, "a row offers the browser legs the backend named");

        _watchedOn = row.WatchedOn;
        _tiled = tiled;
        IsTiled = tiled;
        WatchLabel = tiled ? "Take out of the grid" : "Watch in the grid";
        WatchGlyph = tiled ? Icons.IconX : Icons.IconPlayerPlay;

        Label = row.OwnName;
        Detail = row.Detail;
        Tracks = row.Tracks;
        Readers = row.Readers == 1 ? "1 reader" : $"{row.Readers} readers";
        IsReady = row.IsReady;
        IsWatched = row.IsWatched;

        Reconcile.Onto(Legs, legs.Select(leg => new WatchLegViewModel
        {
            Value = leg.Value,
            Label = leg.Label,
            IsOpen = row.WatchedOn.Contains(leg.Value),

            // An open leg stays pressable whatever the availability pass says, because the press is what
            // closes it.
            // Greying it would leave a player running with its one stop control inert, which is worse than
            // the state the greying is about (docs/field-availability.md).
            IsEnabled = leg.IsEnabled || row.WatchedOn.Contains(leg.Value),
            Reason = leg.Reason,
            Toggle = ToggleOf(leg.Value),
        }).ToList());

        Reconcile.Onto(BrowserLegs, browserLegs.Select(leg => new BrowserLegViewModel
        {
            Value = leg.Value,
            Label = leg.Label,
            Open = PageOf(leg.Value),
        }).ToList());

        Assert.That(Legs.Count == legs.Count, "a control per offered leg", Legs.Count, legs.Count);
        Assert.That(BrowserLegs.Count == browserLegs.Count,
            "a control per offered browser leg", BrowserLegs.Count, browserLegs.Count);
        // Gated on legs being offered at all, and not as an economy: a pass that runs before the form
        // resolves offers none (ViewerViewModel.LegsOf answers empty for a form that is not there), while a
        // stream the backend is already supervising arrives watched on that same pass.
        // The invariant is what a drawn leg list means, so a row with nothing drawn yet states nothing.
        Assert.That(Legs.Count == 0 || !IsWatched || Legs.Any(leg => leg.IsOpen),
            "a watched stream offering legs has one to close", Name, row.WatchedOn.Count);
    }

    /// <summary>
    /// The command for one leg, made once and reused.
    /// Which direction it goes is read when it runs rather than captured now, so a command made on one pass
    /// still does the right thing on the next.
    /// </summary>
    private PendingCommand ToggleOf(string transport)
    {
        if (_toggles.TryGetValue(transport, out var command))
        {
            return command;
        }

        // Opening a viewer launches a player on the backend's machine and closing one brings that player
        // down, and neither is quick.
        // The command holds its own call, so a leg being opened says so while the ones beside it stay
        // pressable.
        command = new PendingCommand(
            () => _watch(Name, transport, _watchedOn.Contains(transport)), _dispatch);

        _toggles[transport] = command;
        return command;
    }

    /// <summary>
    /// The command for one browser leg, made once and reused for the reason the leg toggles are.
    ///
    /// It reads nothing as it runs, which is what separates it from a toggle: one direction, because a page
    /// this app opened is one it cannot find again.
    /// </summary>
    private PendingCommand PageOf(string transport)
    {
        if (_pages.TryGetValue(transport, out var command))
        {
            return command;
        }

        command = new PendingCommand(() => _browse(Name, transport), _dispatch);

        _pages[transport] = command;
        return command;
    }
}

/// <summary>
/// One transport the backend offered as a leg: the value the effect takes, the word a control shows, and
/// whether this machine can be opened on it at all.
/// All of it comes off an option of the form's watch-leg field.
///
/// The verdict travels with the entry rather than beside it, because a list of legs and a separate list of
/// the reachable ones are two facts free to disagree, and the one that would be wrong is the one a menu
/// draws.
/// </summary>
/// <param name="IsEnabled">Reachable, as the availability pass answered.</param>
/// <param name="Reason">Why not, empty where it is (<c>docs/field-availability.md</c>).</param>
public sealed record WatchLeg(string Value, string Label, bool IsEnabled, string Reason);
