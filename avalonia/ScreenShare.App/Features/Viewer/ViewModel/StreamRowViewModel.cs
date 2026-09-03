using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.Model;
using ScreenShare.App.Mvvm;
using TablerIcons;

namespace ScreenShare.App.Features.Viewer.ViewModel;

/// <summary>
/// One stream on the relay, as the rail draws it: what it is, what it is carrying,
/// and the legs this machine can open a viewer on.
///
/// <b>Outputs only.</b>
/// A press runs one of the row's commands, which asks the backend and waits for the event stream to answer.
///
/// Kept across passes and updated in place rather than rebuilt,
/// so the commands stay the same instances and an expanded row does not collapse on every relay poll.
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

    /// <summary>Which legs are open, read by the commands the row hands out.</summary>
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

        // Made once, like the leg commands: a row outlives a pass, and a command rebuilt per pass loses its press.
        //
        // A tile is two calls and the decode behind them, so the toggle waits on the answer.
        // Without the wait, a press landing while the first is opening starts a second decode.
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
    /// A second list rather than a flag on the first:
    /// what a native player reaches and what a browser reaches are different sets, neither containing the other.
    /// </summary>
    public ObservableCollection<BrowserLegViewModel> BrowserLegs { get; }

    /// <summary>
    /// Word the entry carries: the stream's own name,
    /// with the prefix every row of one group shares taken off by the backend.
    /// <see cref="Name"/> is the whole path and stays what the commands open.
    /// </summary>
    public string Label { get => _label; private set => Set(ref _label, value); }

    /// <summary>Ingest rate while the stream runs, and what the path is doing otherwise.</summary>
    public string Detail { get => _detail; private set => Set(ref _detail, value); }

    /// <summary>What the path carries, in the relay's own words.</summary>
    public string Tracks { get => _tracks; private set => Set(ref _tracks, value); }

    /// <summary>Readers the relay counts, as the row prints them.</summary>
    public string Readers { get => _readers; private set => Set(ref _readers, value); }

    /// <summary>Whether a publisher is connected.</summary>
    public bool IsReady { get => _isReady; private set => Set(ref _isReady, value); }

    /// <summary>Whether this machine has any external viewer open on this stream.</summary>
    public bool IsWatched { get => _isWatched; private set => Set(ref _isWatched, value); }

    /// <summary>
    /// Not the same fact as <see cref="IsWatched"/>:
    /// a viewer is a player window the backend launched, a tile is a decode this window draws.
    /// One stream can have both at once, over different legs.
    /// </summary>
    public bool IsTiled { get => _isTiled; private set => Set(ref _isTiled, value); }

    /// <summary>Adds the stream to the grid, or takes it out.</summary>
    public PendingCommand Show { get; }

    private string _watchLabel = "";

    /// <summary>
    /// What the action says it will do, the effect and not the state.
    /// The dot says whether anything is publishing and the pressed toggle says whether the stream is in the grid,
    /// so a label repeating either is a third opinion about one fact.
    /// </summary>
    public string WatchLabel { get => _watchLabel; private set => Set(ref _watchLabel, value); }

    private Icons _watchGlyph = Icons.IconPlayerPlay;

    /// <summary>
    /// Action's glyph: on screen, or off it.
    /// One glyph that changes rather than two controls one of which is hidden,
    /// so the action never moves under the pointer.
    /// </summary>
    public Icons WatchGlyph { get => _watchGlyph; private set => Set(ref _watchGlyph, value); }

    /// <summary>
    /// The one render function.
    /// Safe to run twice: every output is written on every pass,
    /// the legs of two passes over one row compare equal, and the commands are reused by value.
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

        Label = row.Name;
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
        // Gated on legs being offered at all: a pass before the catalog lands offers none,
        // while a stream the backend already supervises arrives watched on that same pass.
        // A row with nothing drawn states nothing.
        Assert.That(Legs.Count == 0 || !IsWatched || Legs.Any(leg => leg.IsOpen),
            "a watched stream offering legs has one to close", Name, row.WatchedOn.Count);
    }

    /// <summary>
    /// Command for one leg, made once and reused.
    /// Direction is read when it runs rather than captured,
    /// so a command made on one pass still does the right thing on the next.
    /// </summary>
    private PendingCommand ToggleOf(string transport)
    {
        if (_toggles.TryGetValue(transport, out var command))
        {
            return command;
        }

        // Opening a viewer launches a player on the backend's machine and closing one brings it down,
        // and neither is quick.
        // One command per leg, so a leg being opened says so while the ones beside it stay pressable.
        command = new PendingCommand(
            () => _watch(Name, transport, _watchedOn.Contains(transport)), _dispatch);

        _toggles[transport] = command;
        return command;
    }

    /// <summary>
    /// Command for one browser leg, made once and reused for the reason the leg toggles are.
    /// One direction and nothing read as it runs, a page this app opened being one it cannot find again.
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
/// One transport the backend offered as a leg: the value the effect takes, and the word a control shows.
/// Value is the catalog's, word is this side's.
/// </summary>
public sealed record WatchLeg(string Value, string Label);
