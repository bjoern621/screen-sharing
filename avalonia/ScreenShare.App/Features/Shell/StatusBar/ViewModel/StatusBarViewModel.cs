using System.Collections.ObjectModel;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.Update.ViewModel;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Shell.StatusBar.ViewModel;

/// <summary>
/// Bottom band: what the window is carrying, the sentence saying what the view in front of it affords,
/// and the send-logs button.
///
/// The design states figures for the viewer alone.
/// Setup receives nothing and broadcast decodes nothing, so a figure there would be invented.
/// The band holds its height in every destination and says nothing where the design says nothing.
///
/// The button lives here because a report is about the app rather than one screen,
/// as the build beside it is.
/// </summary>
public sealed class StatusBarViewModel : Observable
{
    private readonly Action<Action> _dispatch;

    /// <param name="sendReport">Backend effect behind the send-logs button.</param>
    /// <param name="updates">
    /// What the app says about the release published beside this build, owned once for the window.
    /// The band draws its line and its version presses the check;
    /// the dialog behind that line reads the same view model
    /// (<c>Features/Shell/Update/ViewModel/UpdateViewModel.cs</c>).
    /// </param>
    /// <param name="dispatch">Hands a completion back to the UI loop.</param>
    public StatusBarViewModel(
        Func<CancellationToken, Task<string>> sendReport,
        UpdateViewModel updates,
        Action<Action> dispatch)
    {
        Assert.NotNull(sendReport, "a status band sends its report through the backend");
        Assert.NotNull(updates, "a status band states what it knows about the published release");
        Assert.NotNull(dispatch, "a status band marshals completions back to the UI loop");

        _dispatch = dispatch;
        Updates = updates;
        SendLogs = new PendingCommand(() => SendAsync(sendReport), dispatch);
    }

    /// <summary>Sends the report, holding the wait on the button.</summary>
    public PendingCommand SendLogs { get; }

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
    /// Band's whole input beside the send outcome, which the band's own effect writes.
    /// The figures arrive rather than being held here: the destination in front of the band derives them, and
    /// a band holding its own copy would go on printing the throughput of a torn-down decoder.
    ///
    /// The load figures arrive as a list rather than as named slots, what a destination reports being
    /// that destination's business.
    /// A field per figure is a band edited whenever one of them splits in two.
    ///
    /// <paramref name="build"/> is the backend's own, off the handshake, and is empty until it settles.
    /// It arrives with the figures rather than being set once, so the band keeps one render pass.
    /// Idempotent.
    /// </summary>
    public void Show(Destination current, IReadOnlyList<string> load, string hint, string build)
    {
        Assert.NotNull(load, "a status band is told what the link and the units are carrying");
        Assert.NotNull(hint, "a status band is told what the view in front of it affords");
        Assert.NotNull(build, "a status band is told which build is running");

        _current = current;
        _figuresLoad = load;
        _figuresHint = hint;
        _build = build;
        Apply();
    }

    // --- What the send landed ------------------------------------------------------

    private string _sentOutcome = "";
    private bool _sentFailed;

    /// <summary>
    /// Asks the backend for the report and keeps what came back.
    /// The answer belongs to the band that asked, the measurements' exception
    /// (<c>docs/ipc-api.md</c>): the id names a stored bundle, and no state reads it back.
    /// A refusal is the backend's sentence about the attempt and shows as it stands.
    /// </summary>
    private async Task SendAsync(Func<CancellationToken, Task<string>> sendReport)
    {
        try
        {
            var id = await sendReport(default).ConfigureAwait(false);
            Landed(Reports.Sent(id), failed: false);
        }
        catch (BackendUnavailableException e)
        {
            Landed(e.Message, failed: true);
        }
        catch (OperationCanceledException)
        {
        }
    }

    private void Landed(string outcome, bool failed)
    {
        _dispatch(() =>
        {
            _sentOutcome = outcome;
            _sentFailed = failed;
            Apply();
        });
    }

    // --- Outputs -------------------------------------------------------------------

    private bool _showsMetrics;
    private string _hint = "";
    private bool _showsHint;
    private string _version = "";
    private bool _showsVersion;
    private string _sendOutcome = "";
    private bool _showsSendOutcome;
    private bool _sendFailed;

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

    /// <summary>What the last send landed: the stored name, or the backend's refusal.</summary>
    public string SendOutcome { get => _sendOutcome; private set => Set(ref _sendOutcome, value); }

    public bool ShowsSendOutcome { get => _showsSendOutcome; private set => Set(ref _showsSendOutcome, value); }

    /// <summary>Whether the outcome is a refusal, drawn in the failure face and selectable either way.</summary>
    public bool SendFailed { get => _sendFailed; private set => Set(ref _sendFailed, value); }

    /// <summary>
    /// One render function.
    /// Every output on every pass, so a viewer figure cannot outlive a step back into setup.
    /// The send outcome survives every pass: it answers the band's own effect rather than a destination.
    /// </summary>
    public void Apply()
    {
        var speaks = SpeaksFor(_current);

        ShowsMetrics = speaks && _figuresLoad.Count > 0;

        // Emptied where the destination reports nothing, rather than left standing
        // (docs/development-principles.md, "One render function").
        Reconcile.Onto(Load, ShowsMetrics ? _figuresLoad : []);

        Hint = speaks ? _figuresHint : "";
        ShowsHint = Hint.Length > 0;

        // Every destination, the build being the app's rather than one screen's.
        Version = _build.Length > 0 ? "v" + _build : "";
        ShowsVersion = Version.Length > 0;

        // The version's own control and the line beside it, rendered on this pass so the two agree.
        Updates.Apply();

        SendOutcome = _sentOutcome;
        ShowsSendOutcome = SendOutcome.Length > 0;
        SendFailed = _sentFailed && ShowsSendOutcome;

        Assert.That(ShowsMetrics == (Load.Count > 0), "the figures and the flag drawing them agree", ShowsMetrics, Load.Count);
        Assert.That(ShowsHint == (Hint.Length > 0), "the trailing hint and its text agree", ShowsHint, Hint);
        Assert.That(ShowsVersion == (Version.Length > 0), "the version and its text agree", ShowsVersion, Version);
        Assert.That(ShowsSendOutcome == (SendOutcome.Length > 0), "the send outcome and its text agree", ShowsSendOutcome, SendOutcome);
        Assert.That(!SendFailed || ShowsSendOutcome, "a failure mark rides on a shown outcome", SendFailed, ShowsSendOutcome);
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
