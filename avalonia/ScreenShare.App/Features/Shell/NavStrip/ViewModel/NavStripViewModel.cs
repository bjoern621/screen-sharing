using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.NavStrip.ViewModel;

/// <summary>
/// The strip that answers three questions at once: where you are, where else you may go, and whether the
/// world is being changed right now.
///
/// It owns none of that.
/// The shell owns the destination, its availability and how long the stream has been running, and pushes all
/// three in through <see cref="Show"/>; the fields behind them are the cache <see cref="Apply"/> refills on
/// every pass, never a second copy that can drift (docs/development-principles.md, "State is written
/// explicitly and read continuously").
/// The timer in particular is the encoder's own, which is why no clock ticks in here.
///
/// The one thing the strip does own is the user's click, which is why <see cref="SelectedTab"/> is the only
/// public setter here.
/// </summary>
public sealed class NavStripViewModel : Observable
{
    /// <summary>The only reason copy the design states for an unreachable destination.</summary>
    private const string BroadcastHint = "Broadcast opens once you start sharing";

    private const string SharingText = "Sharing";

    private readonly Action<Destination> _select;

    public NavStripViewModel(Action<Destination> select)
    {
        Assert.NotNull(select, "a strip needs somewhere to send the destination it was asked for");

        _select = select;
        Tabs = [.. Destinations.All.Select(destination => new DestinationTab(destination))];

        Assert.That(Tabs.Count == Destinations.All.Count, "a segment per destination", Tabs.Count, Destinations.All.Count);
    }

    /// <summary>One segment per destination, in the table's order. Fixed for the strip's life.</summary>
    public IReadOnlyList<DestinationTab> Tabs { get; }

    // --- What the shell says -------------------------------------------------------

    private Destination _current = Destination.Setup;
    private bool _broadcastAvailable;
    private string _elapsed = "";

    /// <summary>
    /// The strip's whole input, written in one go so its three parts cannot disagree: a segment that is
    /// dimmed and selected at once has no rendering, and neither has a pill that says sharing with nothing
    /// publishing.
    /// Idempotent.
    /// </summary>
    /// <param name="elapsed">
    /// How long the encoder has been running, as the shell composed it from the running state.
    /// It is handed in rather than counted here: no clock in this strip could agree with the encoder's own,
    /// and the pill beside the header figures reads the same field.
    /// </param>
    public void Show(Destination current, bool broadcastAvailable, string elapsed)
    {
        Assert.That(
            current != Destination.Broadcast || broadcastAvailable,
            "a strip shows broadcast only while broadcast can be reached",
            (int)current);
        Assert.NotNull(elapsed, "a strip is told how long the stream it reports has been running");

        _current = current;
        _broadcastAvailable = broadcastAvailable;
        _elapsed = elapsed;
        Apply();
    }

    // --- Inputs --------------------------------------------------------------------

    private DestinationTab? _selectedTab;

    /// <summary>
    /// What the segmented control has selected.
    /// The user owns it, so its setter is the named write and reports the choice onwards; <see cref="Apply"/>
    /// writes the field behind it instead, which is what stops a render pass from looking like a click.
    /// </summary>
    public DestinationTab? SelectedTab
    {
        get => _selectedTab;
        set
        {
            if (!Set(ref _selectedTab, value))
            {
                return;
            }

            // A dimmed segment is still an item, and a list box will land on one by keyboard.
            // Refusing here is what lets the shell assert availability rather than defend against its own
            // strip; Apply below puts the selection back.
            if (value is { IsAvailable: true })
            {
                _select(value.Value);
            }

            Apply();
        }
    }

    // --- Outputs -------------------------------------------------------------------

    private string _hint = "";
    private bool _showsHint;
    private string _onAirLabel = "";
    private string _onAirTimer = "";
    private bool _showsSharing;

    public string Hint { get => _hint; private set => Set(ref _hint, value); }

    public bool ShowsHint { get => _showsHint; private set => Set(ref _showsHint, value); }

    public string SharingLabel { get => _onAirLabel; private set => Set(ref _onAirLabel, value); }

    public string SharingTimer { get => _onAirTimer; private set => Set(ref _onAirTimer, value); }

    public bool ShowsSharing { get => _showsSharing; private set => Set(ref _showsSharing, value); }

    /// <summary>
    /// The one render function.
    /// Every output is written on every pass, the off branches included, so the pill's text cannot survive
    /// the state that justified it.
    /// </summary>
    public void Apply()
    {
        foreach (var tab in Tabs)
        {
            tab.SetAvailable(tab.Value != Destination.Broadcast || _broadcastAvailable);
        }

        // Written through the field, not the property: going through the setter would send the shell's own
        // answer back to it as though the user had clicked.
        var selected = TabFor(_current);
        Set<DestinationTab?>(ref _selectedTab, selected, nameof(SelectedTab));

        ShowsHint = _current == Destination.Setup;
        Hint = ShowsHint ? BroadcastHint : "";

        // The pill answers "is this machine broadcasting", not "which destination is showing"
        // (docs/design-language.md, "Status language"): the one red is spent on sharing, so standing on the
        // viewer while publishing nothing must not light it.
        // The shell's availability flag is that same fact - broadcast is reachable exactly while something
        // publishes - so the strip reads it rather than keeping a second copy that can drift.
        ShowsSharing = _broadcastAvailable;
        SharingLabel = ShowsSharing ? SharingText : "";
        SharingTimer = ShowsSharing ? _elapsed : "";

        // The pair the whole strip turns on: the segment standing lit is the one the shell showed, and it is
        // one the reader could have reached.
        Assert.That(selected.IsAvailable, "the destination a strip stands in is one it can reach", (int)selected.Value);
        Assert.That(ShowsHint == (Hint.Length > 0), "the setup hint and its text agree", ShowsHint, Hint);
        Assert.That(ShowsSharing == (SharingLabel.Length > 0), "the sharing pill and its text agree", ShowsSharing, SharingLabel);
    }

    private DestinationTab TabFor(Destination destination)
    {
        var tab = Tabs.FirstOrDefault(candidate => candidate.Value == destination);
        return Assert.NotNull(tab, "the strip holds a segment for every destination");
    }
}
