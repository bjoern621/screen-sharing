using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Insights.Model;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.Update.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.StatusBar.ViewModel;

/// <summary>
/// Bottom band: what this computer's connection is carrying and the sentence saying what the view in front of it
/// affords.
///
/// The figures are the app's own fact and hold in every destination, a stream being published from one screen and
/// watched from another (<c>Features/Shell/StatusBar/Model/NetworkLoad.cs</c>).
/// The sentence beside them speaks for the view it belongs to.
/// The band holds its height in every destination and says nothing where there is nothing to state.
/// </summary>
public sealed class StatusBarViewModel : Observable
{
    /// <param name="updates">
    /// What the app says about the release published beside this build, owned once for the window.
    /// The band draws its line and its version presses the check;
    /// the dialog behind that line reads the same view model
    /// (<c>Features/Shell/Update/ViewModel/UpdateViewModel.cs</c>).
    /// </param>
    public StatusBarViewModel(UpdateViewModel updates)
    {
        Assert.NotNull(updates, "a status band states what it knows about the published release");

        Updates = updates;
    }

    /// <summary>
    /// The published release, as the band states it: the version's own control and the line beside it.
    /// Held rather than mirrored, so the band and the dialog read one answer.
    /// </summary>
    public UpdateViewModel Updates { get; }

    // --- What the shell says -------------------------------------------------------

    private Destination _current = Destination.Setup;
    private IReadOnlyList<string> _figuresLoad = [];
    private string _figuresHint = "";
    private string _build = "";

    /// <summary>
    /// Band's whole input.
    /// The figures arrive rather than being held here: the shell derives them off the running state, and
    /// a band holding its own copy would go on printing the throughput of a torn-down decoder.
    ///
    /// They arrive as a list rather than as named slots, so a direction with nothing to state is absent
    /// instead of reading zero.
    /// A field per figure is a band edited whenever one of them splits in two.
    ///
    /// <paramref name="build"/> is the backend's own, off the handshake, and is empty until it settles.
    /// It arrives with the figures rather than being set once, so the band keeps one render pass.
    /// Idempotent.
    /// </summary>
    public void Show(Destination current, IReadOnlyList<string> load, string hint, string build)
    {
        Assert.NotNull(load, "a status band is told what this computer's connection is carrying");
        Assert.NotNull(hint, "a status band is told what the view in front of it affords");
        Assert.NotNull(build, "a status band is told which build is running");

        _current = current;
        _figuresLoad = load;
        _figuresHint = hint;
        _build = build;
        Apply();
    }

    // --- Outputs -------------------------------------------------------------------

    private bool _showsMetrics;
    private string _hint = "";
    private bool _showsHint;
    private string _version = "";
    private bool _showsVersion;

    /// <summary>Whether this destination has figures worth stating.</summary>
    public bool ShowsMetrics { get => _showsMetrics; private set => Set(ref _showsMetrics, value); }

    /// <summary>Measurements, in the order the destination handed them over.</summary>
    public ObservableCollection<string> Load { get; } = [];

    /// <summary>Trailing sentence. Contextual rather than measured: it moves with the view.</summary>
    public string Hint { get => _hint; private set => Set(ref _hint, value); }

    public bool ShowsHint { get => _showsHint; private set => Set(ref _showsHint, value); }

    /// <summary>
    /// The running build, marked as a version so it reads as one beside figures that are measurements.
    /// </summary>
    public string Version { get => _version; private set => Set(ref _version, value); }

    /// <summary>Whether a build has been answered yet.</summary>
    public bool ShowsVersion { get => _showsVersion; private set => Set(ref _showsVersion, value); }

    /// <summary>
    /// One render function.
    /// Every output on every pass, so a viewer figure cannot outlive a step back into setup.
    /// </summary>
    public void Apply()
    {
        ShowsMetrics = _figuresLoad.Count > 0;

        // Emptied where nothing is carried, rather than left standing
        // (docs/development-principles.md, "One render function").
        Reconcile.Onto(Load, _figuresLoad);

        Hint = HintedIn(_current) ? _figuresHint : "";
        ShowsHint = Hint.Length > 0;

        // Every destination, the build being the app's rather than one screen's.
        Version = _build.Length > 0 ? "v" + _build : "";
        ShowsVersion = Version.Length > 0;

        // The version's own control and the line beside it, rendered on this pass so the two agree.
        Updates.Apply();

        Assert.That(ShowsMetrics == (Load.Count > 0), "the figures and the flag drawing them agree", ShowsMetrics, Load.Count);
        Assert.That(ShowsHint == (Hint.Length > 0), "the trailing hint and its text agree", ShowsHint, Hint);
        Assert.That(ShowsVersion == (Version.Length > 0), "the version and its text agree", ShowsVersion, Version);
    }

    /// <summary>
    /// Whether the band carries a sentence for this destination.
    /// Exhaustive, so a destination added without an answer fails here rather than printing the last one's.
    /// </summary>
    private static bool HintedIn(Destination destination) => destination switch
    {
        Destination.Setup => false,
        Destination.Insights => false,
        Destination.Viewer => true,
        _ => Assert.Never<bool>("unexpected destination", (int)destination),
    };
}
