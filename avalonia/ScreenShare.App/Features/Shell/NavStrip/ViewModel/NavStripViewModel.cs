using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.NavStrip.ViewModel;

/// <summary>
/// Strip: which destination is showing, and whether this machine is sharing.
///
/// Owns neither.
/// The shell owns both and pushes them through <see cref="Show"/>, and the fields behind them are what
/// <see cref="Apply"/> refills on every pass rather than a second copy that can drift
/// (docs/development-principles.md, "State is written explicitly and read continuously").
/// The elapsed time is the encoder's, so no clock ticks here.
///
/// Every segment is reachable at all times, sharing or not: what a destination has to say about a stream that has
/// ended is what a publisher goes looking for once it has (docs/design-language.md, "Surfaces and shape").
///
/// The reader's click is the one thing the strip owns, so <see cref="SelectedTab"/> is its only public setter.
/// </summary>
public sealed class NavStripViewModel : Observable
{
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
    private bool _sharing;
    private string _elapsed = "";

    /// <summary>
    /// Strip's whole input, written in one go so its parts cannot disagree: a pill saying sharing with nothing
    /// publishing has no rendering.
    /// Idempotent.
    /// </summary>
    /// <param name="elapsed">
    /// Encoder run time, composed by the shell off the running state.
    /// Handed in rather than counted here: a clock in this strip could not agree with the encoder's, and the pill
    /// beside the header figures reads the same field.
    /// </param>
    public void Show(Destination current, bool sharing, string elapsed)
    {
        Assert.NotNull(elapsed, "a strip is told how long the stream it reports has been running");

        _current = current;
        _sharing = sharing;
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

            // A list box clearing its selection is not a reader asking for anywhere, and Apply below puts the
            // showing destination back.

            if (value is not null)
            {
                _select(value.Value);
            }

            Apply();
        }
    }

    // --- Outputs -------------------------------------------------------------------

    private string _onAirLabel = "";
    private string _onAirTimer = "";
    private bool _showsSharing;

    public string SharingLabel { get => _onAirLabel; private set => Set(ref _onAirLabel, value); }

    public string SharingTimer { get => _onAirTimer; private set => Set(ref _onAirTimer, value); }

    public bool ShowsSharing { get => _showsSharing; private set => Set(ref _showsSharing, value); }

    /// <summary>
    /// One render function.
    /// Every output on every pass, off branches included, so the pill's text cannot outlive the state that
    /// justified it.
    /// </summary>
    public void Apply()
    {
        // Through the field, not the property: the setter would send the shell's own answer back to it as a click.
        Set<DestinationTab?>(ref _selectedTab, TabFor(_current), nameof(SelectedTab));

        // The pill says whether this machine is sharing, never which destination is showing
        // (docs/design-language.md, "Status language").
        ShowsSharing = _sharing;
        SharingLabel = ShowsSharing ? SharingText : "";
        SharingTimer = ShowsSharing ? _elapsed : "";

        Assert.That(ShowsSharing == (SharingLabel.Length > 0), "the sharing pill and its text agree", ShowsSharing, SharingLabel);
    }

    private DestinationTab TabFor(Destination destination)
    {
        var tab = Tabs.FirstOrDefault(candidate => candidate.Value == destination);
        return Assert.NotNull(tab, "the strip holds a segment for every destination");
    }
}
