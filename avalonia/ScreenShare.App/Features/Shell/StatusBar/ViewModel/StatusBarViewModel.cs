using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.StatusBar.ViewModel;

/// <summary>
/// Bottom band: what the window is carrying, and the sentence saying what the view in front of it affords.
///
/// The design states figures for the viewer alone.
/// Setup receives nothing and broadcast decodes nothing, so a figure there would be invented.
/// The band holds its height in every destination and says nothing where the design says nothing.
/// </summary>
public sealed class StatusBarViewModel : Observable
{
    // --- What the shell says -------------------------------------------------------

    private Destination _current = Destination.Setup;
    private string _figuresStreams = "";
    private IReadOnlyList<string> _figuresLoad = [];
    private string _figuresHint = "";

    /// <summary>
    /// Band's whole input.
    /// The figures arrive rather than being held here: the destination in front of the band derives them, and
    /// a band holding its own copy would go on printing the throughput of a torn-down decoder.
    ///
    /// The load figures arrive as a list rather than as named slots, what a destination reports being
    /// that destination's business.
    /// A field per figure is a band edited whenever one of them splits in two.
    /// Idempotent.
    /// </summary>
    public void Show(Destination current, string streams, IReadOnlyList<string> load, string hint)
    {
        Assert.NotNull(streams, "a status band is told how much of what arrives is on screen");
        Assert.NotNull(load, "a status band is told what the link and the units are carrying");
        Assert.NotNull(hint, "a status band is told what the view in front of it affords");

        _current = current;
        _figuresStreams = streams;
        _figuresLoad = load;
        _figuresHint = hint;
        Apply();
    }

    // --- Outputs -------------------------------------------------------------------

    private bool _showsMetrics;
    private string _streams = "";
    private string _hint = "";
    private bool _showsHint;

    /// <summary>Whether this destination has figures worth stating.</summary>
    public bool ShowsMetrics { get => _showsMetrics; private set => Set(ref _showsMetrics, value); }

    /// <summary>Prose: how much of what arrives is on screen.</summary>
    public string Streams { get => _streams; private set => Set(ref _streams, value); }

    /// <summary>Measurements, in the order the destination handed them over.</summary>
    public ObservableCollection<string> Load { get; } = [];

    /// <summary>Trailing sentence. Contextual rather than measured: it moves with the view.</summary>
    public string Hint { get => _hint; private set => Set(ref _hint, value); }

    public bool ShowsHint { get => _showsHint; private set => Set(ref _showsHint, value); }

    /// <summary>
    /// One render function.
    /// Every output on every pass, so a viewer figure cannot outlive a step back into setup.
    /// </summary>
    public void Apply()
    {
        var speaks = SpeaksFor(_current);

        ShowsMetrics = speaks && _figuresStreams.Length > 0;
        Streams = ShowsMetrics ? _figuresStreams : "";

        // Emptied where the destination reports nothing, rather than left standing
        // (docs/development-principles.md, "One render function").
        Reconcile.Onto(Load, ShowsMetrics ? _figuresLoad : []);

        Hint = speaks ? _figuresHint : "";
        ShowsHint = Hint.Length > 0;

        Assert.That(ShowsMetrics == (Streams.Length > 0), "the figures and the prose beside them appear together", ShowsMetrics, Streams);
        Assert.That(ShowsMetrics || Load.Count == 0, "a band that states no counts states no measurements either", ShowsMetrics, Load.Count);
        Assert.That(ShowsHint == (Hint.Length > 0), "the trailing hint and its text agree", ShowsHint, Hint);
    }

    /// <summary>
    /// Whether the band says anything in this destination.
    /// Exhaustive, so a destination added without an answer fails here rather than printing the last one's
    /// throughput.
    /// </summary>
    private static bool SpeaksFor(Destination destination) => destination switch
    {
        Destination.Setup => false,
        Destination.Broadcast => false,
        Destination.Viewer => true,
        _ => Assert.Never<bool>("unexpected destination", (int)destination),
    };
}
