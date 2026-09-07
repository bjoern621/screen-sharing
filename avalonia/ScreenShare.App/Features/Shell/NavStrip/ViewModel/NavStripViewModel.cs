using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Shell.Go.ViewModel;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.NavStrip.ViewModel;

/// <summary>
/// Strip: which destination is showing.
///
/// The shell owns it and pushes it through <see cref="Show"/>, and the field behind it is what
/// <see cref="Apply"/> refills on every pass rather than a second copy that can drift
/// (<c>docs/development-principles.md</c>, "State is written explicitly and read continuously").
///
/// Every segment is reachable at all times, sharing or not: what a destination has to say about a stream that has
/// ended is what a publisher goes looking for once it has (<c>docs/design-language.md</c>, "Surfaces and shape").
///
/// The reader's click is the one thing the strip owns, so <see cref="SelectedTab"/> is its only public setter.
/// </summary>
public sealed class NavStripViewModel : Observable
{
    private readonly Action<Destination> _select;

    /// <param name="go">
    /// The commit beside the pill, the shell's.
    /// Optional so a test of the segments or the pill builds no publish graph behind it;
    /// the view draws the control only where one is given.
    /// </param>
    /// <param name="openSettings">
    /// Opens the settings about the app, the shell owning whether the dialog stands.
    /// Optional on the same terms as the commit beside it.
    /// </param>
    public NavStripViewModel(
        Action<Destination> select, GoViewModel? go = null, DelegateCommand? openSettings = null)
    {
        Assert.NotNull(select, "a strip needs somewhere to send the destination it was asked for");

        _select = select;
        Go = go;
        OpenSettings = openSettings;
        Tabs = [.. Destinations.All.Select(destination => new DestinationTab(destination))];

        Assert.That(Tabs.Count == Destinations.All.Count, "a segment per destination", Tabs.Count, Destinations.All.Count);
    }

    /// <summary>Strip commit and its menu. Null in a strip built without one.</summary>
    public GoViewModel? Go { get; }

    /// <summary>Opens the app settings. Null in a strip built without them.</summary>
    public DelegateCommand? OpenSettings { get; }

    /// <summary>A segment per destination, in the table's order. Fixed for the strip's life.</summary>
    public IReadOnlyList<DestinationTab> Tabs { get; }

    // --- What the shell says -------------------------------------------------------

    private Destination _current = Destination.Setup;

    /// <summary>Strip's whole input. Idempotent.</summary>
    public void Show(Destination current)
    {
        _current = current;
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

            // A list box clearing its selection is not a reader asking for anywhere,
            // and Apply below puts the showing destination back.

            if (value is not null)
            {
                _select(value.Value);
            }

            Apply();
        }
    }

    // --- Outputs -------------------------------------------------------------------

    /// <summary>One render function: the whole strip off the shell's state, on every pass.</summary>
    public void Apply()
    {
        // Through the field, not the property: the setter would send the shell's own answer back to it as a click.
        Set<DestinationTab?>(ref _selectedTab, TabFor(_current), nameof(SelectedTab));
    }

    private DestinationTab TabFor(Destination destination)
    {
        var tab = Tabs.FirstOrDefault(candidate => candidate.Value == destination);
        return Assert.NotNull(tab, "the strip holds a segment for every destination");
    }
}
