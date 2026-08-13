using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.StatusBar.ViewModel;

/// <summary>
/// The bottom band: what the window is carrying, and the one sentence that says what the view in front of it
/// affords.
///
/// The design draws a status bar on the viewer only.
/// Setup is receiving nothing and broadcast is decoding nothing, so their figures would be a lie and the
/// source states no copy for either.
/// The band keeps its height in every destination and says nothing where the design says nothing, rather than
/// inventing a number (scratchpad/spec/6a-nav-chrome.md, "Explicitly not specified in the source").
/// </summary>
public sealed class StatusBarViewModel : Observable
{
    // --- What the shell says -------------------------------------------------------

    private Destination _current = Destination.Setup;
    private string _figuresStreams = "";
    private IReadOnlyList<string> _figuresLoad = [];
    private string _figuresHint = "";

    /// <summary>
    /// The band's whole input.
    /// The figures arrive rather than being held here, because the destination in front of the band is the
    /// one that knows them: the viewer derives its counts from the chips a reader just toggled, and a band
    /// that kept its own copy would go on printing the throughput of a decoder that has already been torn
    /// down.
    ///
    /// The load figures arrive as a list and not as named slots.
    /// What a destination has to report is that destination's business, and a band with a field per figure
    /// would have to be edited every time one is split - which is exactly what splitting the pooled decode
    /// percent into GPU and cores would have cost.
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

    /// <summary>Whether this destination has figures worth stating at all.</summary>
    public bool ShowsMetrics { get => _showsMetrics; private set => Set(ref _showsMetrics, value); }

    /// <summary>Prose: how much of what is arriving is on screen.</summary>
    public string Streams { get => _streams; private set => Set(ref _streams, value); }

    /// <summary>
    /// The measurements, in the order the destination handed them over.
    /// Each reads a shade quieter than the prose beside it, in tabular figures so a tick does not reflow the
    /// row.
    /// </summary>
    public ObservableCollection<string> Load { get; } = [];

    /// <summary>The trailing sentence. Contextual, not a metric: it changes with the view.</summary>
    public string Hint { get => _hint; private set => Set(ref _hint, value); }

    public bool ShowsHint { get => _showsHint; private set => Set(ref _showsHint, value); }

    /// <summary>
    /// The one render function.
    /// Every output is written on every pass, so a figure from the viewer cannot survive a step back into
    /// setup.
    /// </summary>
    public void Apply()
    {
        var speaks = SpeaksFor(_current);

        ShowsMetrics = speaks && _figuresStreams.Length > 0;
        Streams = ShowsMetrics ? _figuresStreams : "";

        // Emptied rather than left standing where the destination reports nothing: the band keeps its height
        // in every destination, but a figure from the viewer must not survive a step back into setup
        // (docs/development-principles.md, "One render function").
        Reconcile.Onto(Load, ShowsMetrics ? _figuresLoad : []);

        Hint = speaks ? _figuresHint : "";
        ShowsHint = Hint.Length > 0;

        Assert.That(ShowsMetrics == (Streams.Length > 0), "the figures and the prose beside them appear together", ShowsMetrics, Streams);
        Assert.That(ShowsMetrics || Load.Count == 0, "a band that states no counts states no measurements either", ShowsMetrics, Load.Count);
        Assert.That(ShowsHint == (Hint.Length > 0), "the trailing hint and its text agree", ShowsHint, Hint);
    }

    /// <summary>
    /// Whether the band says anything at all in this destination.
    /// Exhaustive, so a destination added without an answer fails here rather than showing the previous one's
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
