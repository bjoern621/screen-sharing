using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Viewer.Model;
using ScreenShare.App.Mvvm;
using TablerIcons;

namespace ScreenShare.App.Features.Viewer.ViewModel;

/// <summary>
/// One stream on the relay, as the list draws it: what it is, what it is carrying, and the legs this machine
/// can open a viewer on.
///
/// <b>Outputs only.</b> The row has no input of its own - a click runs one of the leg commands, which asks
/// the backend and waits for the event stream to say what happened, exactly as the broadcast screen's stop
/// does.
///
/// It is kept across passes and updated in place rather than rebuilt, so the legs' commands stay the same
/// instances and an expanded row does not collapse under the pointer every time the relay is polled.
/// </summary>
public sealed class StreamRowViewModel : Observable
{
    private readonly Func<string, string, bool, Task> _watch;
    private readonly Func<string, string, Task> _browse;
    private readonly Action<Action> _dispatch;
    private readonly Dictionary<string, PendingCommand> _toggles = [];
    private readonly Dictionary<string, PendingCommand> _pages = [];

    /// <summary>
    /// Whether this stream is in the grid, read when the command runs rather than captured, so one command
    /// instance still does the right thing after any number of passes.
    /// </summary>
    private bool _tiled;

    /// <summary>The row's own view of which legs are open, read by the commands it hands out.</summary>
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

        // Made once, like the leg commands and for the same reason: a row is kept across passes, and a
        // command rebuilt per pass would be a button that loses its press.
        //
        // A tile is two calls to the backend and the decode behind them, so the toggle waits on the answer.
        // It used to start a second decode for every press that landed while the first was still being
        // opened.
        Show = new PendingCommand(() => tile(Name, _tiled), dispatch);
    }

    /// <summary>The stream name, carried and never parsed.</summary>
    public string Name { get; }

    // --- Outputs ------------------------------------------------------------------

    private string _detail = "";
    private string _tracks = "";
    private string _readers = "";
    private bool _isReady;
    private bool _isWatched;
    private bool _isTiled;

    /// <summary>The legs a viewer can be opened on, in the order the backend offered them.</summary>
    public ObservableCollection<WatchLegViewModel> Legs { get; }

    /// <summary>
    /// The legs the relay serves a player page for, in the order the backend offered them.
    ///
    /// A second list rather than a flag on the first, because the two rosters are different sets and neither
    /// contains the other: no player opens WHEP, and a browser reaches neither SRT nor RTSP.
    /// </summary>
    public ObservableCollection<BrowserLegViewModel> BrowserLegs { get; }

    /// <summary>The rate while the stream runs, and what it is doing otherwise.</summary>
    public string Detail { get => _detail; private set => Set(ref _detail, value); }

    /// <summary>The relay's own description of what the path carries.</summary>
    public string Tracks { get => _tracks; private set => Set(ref _tracks, value); }

    /// <summary>How many readers the relay counts, as the row prints it.</summary>
    public string Readers { get => _readers; private set => Set(ref _readers, value); }

    /// <summary>Whether a publisher is connected. A path that is not ready is drawn dimmed.</summary>
    public bool IsReady { get => _isReady; private set => Set(ref _isReady, value); }

    /// <summary>Whether this machine has any viewer open on this stream.</summary>
    public bool IsWatched { get => _isWatched; private set => Set(ref _isWatched, value); }

    /// <summary>
    /// Whether this stream has a tile in the grid.
    ///
    /// It is not the same fact as <see cref="IsWatched"/> and the two are deliberately kept apart: a viewer
    /// is a player window the backend launched, and a tile is a decode this window draws.
    /// One stream can have both, over different legs, at once.
    /// </summary>
    public bool IsTiled { get => _isTiled; private set => Set(ref _isTiled, value); }

    /// <summary>Puts this stream in the grid, or takes it out again.</summary>
    public PendingCommand Show { get; }

    private string _watchLabel = "";

    /// <summary>
    /// What the entry's action says it will do.
    ///
    /// It names the effect rather than the state, because the entry already draws the state: the dot says
    /// whether anything is publishing and the pressed toggle says whether it is in the grid, so a label
    /// repeating either would be a third opinion about the same fact.
    /// </summary>
    public string WatchLabel { get => _watchLabel; private set => Set(ref _watchLabel, value); }

    private Icons _watchGlyph = Icons.IconPlayerPlay;

    /// <summary>
    /// The action's glyph: put this stream on screen, or take it off.
    ///
    /// One glyph that changes rather than two controls one of which is hidden, so the entry's action never
    /// moves under the pointer.
    /// </summary>
    public Icons WatchGlyph { get => _watchGlyph; private set => Set(ref _watchGlyph, value); }

    /// <summary>
    /// The one render function.
    /// Safe to run twice: every output is written on every pass, the legs compare equal across two passes
    /// over one row, and the commands are reused by value.
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

            // A leg already open stays pressable whatever the availability pass says about it, because the
            // press closes it.
            // Greying it would leave a player running with the one control that ends it inert, which is a
            // worse state than the one the greying is about (docs/field-availability.md).
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
        Assert.That(IsWatched == Legs.Any(leg => leg.IsOpen) || row.WatchedOn.Count > 0,
            "a watched stream has a leg to close", Name, row.WatchedOn.Count);
    }

    /// <summary>
    /// The command for one leg, made once and reused.
    /// Whether it opens or closes is read when it runs rather than captured now, so a command made on one
    /// pass still does the right thing on the next.
    /// </summary>
    private PendingCommand ToggleOf(string transport)
    {
        if (_toggles.TryGetValue(transport, out var command))
        {
            return command;
        }

        // Opening a viewer launches a player on the backend's machine and closing one brings it down, and
        // neither is quick.
        // The command holds whether its own call is out, so a leg being opened says so while the ones beside
        // it stay pressable.
        command = new PendingCommand(
            () => _watch(Name, transport, _watchedOn.Contains(transport)), _dispatch);

        _toggles[transport] = command;
        return command;
    }

    /// <summary>
    /// The command for one browser leg, made once and reused for the reason the leg toggles are: a row
    /// outlives a pass, and a command rebuilt per pass is a row that loses a press.
    ///
    /// It reads nothing when it runs, which is what separates it from a toggle: there is one direction,
    /// because a page this app opened is one it cannot find again.
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
/// One transport the backend offered as a watch leg: the value <c>StartWatch</c> takes, the label a control
/// shows, and whether this machine can be opened on it at all.
/// All of it comes off an option of the form's watch-leg field.
///
/// The verdict travels with the entry rather than beside it, because a list of legs and a separate list of
/// the ones that are reachable are two facts free to disagree - and the one that would be wrong is the one a
/// menu draws.
/// </summary>
/// <param name="IsEnabled">Whether the option is reachable, as the availability pass answered.</param>
/// <param name="Reason">Why it is not, and empty where it is (<c>docs/field-availability.md</c>).</param>
public sealed record WatchLeg(string Value, string Label, bool IsEnabled, string Reason);
