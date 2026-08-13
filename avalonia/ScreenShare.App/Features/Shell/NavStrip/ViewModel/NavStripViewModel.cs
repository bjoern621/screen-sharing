using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.NavStrip.ViewModel;

/// <summary>
/// The strip: which destination is showing, which others can be reached, and whether this machine is sharing.
///
/// It owns none of that.
/// The shell owns all of it and pushes it through <see cref="Show"/>, and the fields behind it are what
/// <see cref="Apply"/> refills on every pass rather than a second copy that can drift
/// (docs/development-principles.md, "State is written explicitly and read continuously").
/// The elapsed time is the encoder's, so no clock ticks here.
///
/// The reader's click is the one thing the strip owns, so <see cref="SelectedTab"/> is its only public setter.
/// </summary>
public sealed class NavStripViewModel : Observable
{
    /// <summary>Design copy for an unreachable destination, verbatim.</summary>
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

    /// <summary>A segment per destination, in the table's order. Fixed for the strip's life.</summary>
    public IReadOnlyList<DestinationTab> Tabs { get; }

    // --- What the shell says -------------------------------------------------------

    private Destination _current = Destination.Setup;
    private bool _broadcastAvailable;
    private string _elapsed = "";

    /// <summary>
    /// The strip's whole input, written in one go so its parts cannot disagree: a segment dimmed and selected
    /// at once has no rendering, and neither has a pill saying sharing with nothing publishing.
    /// Idempotent.
    /// </summary>
    /// <param name="elapsed">
    /// Encoder run time, composed by the shell off the running state.
    /// Handed in rather than counted here: a clock in this strip could not agree with the encoder's, and the
    /// pill beside the header figures reads the same field.
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
    /// The reader owns it, so the setter is the named write and reports the choice onwards.
    /// <see cref="Apply"/> writes the field behind it instead, so a render pass cannot look like a click.
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

            // A dimmed segment is still an item a list box lands on by keyboard.
            // Refusing here lets the shell assert availability rather than defend against its own strip, and
            // Apply below puts the selection back.
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
    /// Every output on every pass, off branches included, so the pill's text cannot outlive the state that
    /// justified it.
    /// </summary>
    public void Apply()
    {
        foreach (var tab in Tabs)
        {
            tab.SetAvailable(tab.Value != Destination.Broadcast || _broadcastAvailable);
        }

        // Through the field, not the property: the setter would send the shell's own answer back to it as a
        // click.
        var selected = TabFor(_current);
        Set<DestinationTab?>(ref _selectedTab, selected, nameof(SelectedTab));

        ShowsHint = _current == Destination.Setup;
        Hint = ShowsHint ? BroadcastHint : "";

        // The pill says whether this machine is sharing, never which destination is showing
        // (docs/design-language.md, "Status language").
        // Broadcast is reachable exactly while something publishes, so that flag is the same fact and the
        // strip keeps no second copy of it.
        ShowsSharing = _broadcastAvailable;
        SharingLabel = ShowsSharing ? SharingText : "";
        SharingTimer = ShowsSharing ? _elapsed : "";

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
