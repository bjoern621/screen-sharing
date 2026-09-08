using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Insights.Model;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.Update.ViewModel;
using ScreenShare.App.Mvvm;
using UpdateCopy = ScreenShare.App.Copy.Updates;

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
    /// The band puts its line where the build stands and presses it;
    /// the dialog behind that line reads the same view model
    /// (<c>Features/Shell/Update/ViewModel/UpdateViewModel.cs</c>).
    /// </param>
    public StatusBarViewModel(UpdateViewModel updates)
    {
        Assert.NotNull(updates, "a status band states what it knows about the published release");

        Updates = updates;

        // The release slot is composed out of what the update view model derived,
        // so a refusal or a check landing there re-renders the band without waiting for the shell's next pass.
        updates.PropertyChanged += (_, _) => Render();
    }

    /// <summary>
    /// The published release, as the band states it: one control, and the failure that takes its place.
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
    private string _release = "";
    private bool _showsRelease;
    private string _releaseHint = "";

    /// <summary>Whether this destination has figures worth stating.</summary>
    public bool ShowsMetrics { get => _showsMetrics; private set => Set(ref _showsMetrics, value); }

    /// <summary>Measurements, in the order the destination handed them over.</summary>
    public ObservableCollection<string> Load { get; } = [];

    /// <summary>Trailing sentence. Contextual rather than measured: it moves with the view.</summary>
    public string Hint { get => _hint; private set => Set(ref _hint, value); }

    public bool ShowsHint { get => _showsHint; private set => Set(ref _showsHint, value); }

    /// <summary>
    /// The band's one line about the release: what a check found, or the running build where it found nothing.
    /// The build is marked as a version so it reads as one beside figures that are measurements.
    /// </summary>
    public string Release { get => _release; private set => Set(ref _release, value); }

    /// <summary>
    /// Whether that control is drawn.
    /// A failure takes its place as selectable text, and a build nothing has answered leaves the slot empty.
    /// </summary>
    public bool ShowsRelease { get => _showsRelease; private set => Set(ref _showsRelease, value); }

    /// <summary>
    /// What the control says on hover: what the press does, and the build behind a line that displaced it.
    /// </summary>
    public string ReleaseHint { get => _releaseHint; private set => Set(ref _releaseHint, value); }

    /// <summary>
    /// Renders the release the band composes from, then the band.
    /// Idempotent.
    /// </summary>
    public void Apply()
    {
        Updates.Apply();
        Render();
    }

    /// <summary>
    /// One render function.
    /// Every output on every pass, so a viewer figure cannot outlive a step back into setup.
    /// Reads the update view model rather than rendering it, so a pass driven by that model's own change
    /// does not re-enter it.
    /// </summary>
    private void Render()
    {
        ShowsMetrics = _figuresLoad.Count > 0;

        // Emptied where nothing is carried, rather than left standing
        // (docs/development-principles.md, "One render function").
        Reconcile.Onto(Load, _figuresLoad);

        Hint = HintedIn(_current) ? _figuresHint : "";
        ShowsHint = Hint.Length > 0;

        // Every destination, the build being the app's rather than one screen's.
        var build = _build.Length > 0 ? "v" + _build : "";

        // One control for the release, so a found release displaces the build rather than standing beside it.
        // A failure takes the slot as selectable text and leaves no control at all.
        var line = Updates.IsFailure ? "" : Updates.Line;
        Release = line.Length > 0 ? line : build;
        ShowsRelease = !Updates.IsFailure && Release.Length > 0;
        ReleaseHint = line.Length > 0 ? UpdateCopy.Tip(build, Updates.PressHint) : Updates.PressHint;

        Assert.That(ShowsMetrics == (Load.Count > 0), "the figures and the flag drawing them agree", ShowsMetrics, Load.Count);
        Assert.That(ShowsHint == (Hint.Length > 0), "the trailing hint and its text agree", ShowsHint, Hint);
        Assert.That(
            ShowsRelease == (Release.Length > 0 && !Updates.IsFailure),
            "the release control and its text agree", ShowsRelease, Release);
        Assert.That(
            !(ShowsRelease && Updates.ShowsPlainLine),
            "the band says one thing about the release", ShowsRelease, Updates.ShowsPlainLine);
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
