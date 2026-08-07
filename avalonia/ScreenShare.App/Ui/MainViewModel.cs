using System.Collections.ObjectModel;
using Avalonia.Media;
using ScreenShare.App.Contracts;
using ScreenShare.App.Mvvm;
using ScreenShare.App.Relay;

namespace ScreenShare.App.Ui;

/// <summary>
/// The window's state, split in two halves that must not be confused:
///
/// <b>Inputs</b> (<see cref="Host"/>, <see cref="ApiPort"/>, <see cref="AutoRefresh"/>)
/// are owned by the user. Their setters are the named writes, and <see cref="Apply"/>
/// never touches them - a render pass that reassigned a text box would fight whoever is
/// typing in it.
///
/// <b>Outputs</b> are owned by <see cref="Apply"/> alone, the one render function. It
/// sets every one of them on every pass, including the branches that turn something off,
/// so the function by itself is enough to restore a correct view
/// (docs/development-principles.md, "One render function per component").
/// </summary>
public sealed class MainViewModel : Observable
{
    private static readonly TimeSpan RefreshInterval = TimeSpan.FromSeconds(2);

    private readonly RelayPoller _poller;

    public MainViewModel(RelayPoller poller)
    {
        Assert.NotNull(poller, "a view reads its relay state through a poller");

        _poller = poller;
        CheckNowCommand = new DelegateCommand(CheckNow, () => CanCheck);
        Paths = [];

        // The one subscription. Every change the poller reports re-runs the render
        // function against the poller's current state; nothing incremental is applied.
        _poller.Changed += Apply;
    }

    // --- Inputs -------------------------------------------------------------------

    private string _host = "127.0.0.1";
    private decimal? _apiPort = 9997;
    private bool _autoRefresh = true;

    /// <summary>Host the relay's HTTP API answers on.</summary>
    public string Host
    {
        get => _host;
        set
        {
            if (Set(ref _host, value))
            {
                Apply();
            }
        }
    }

    /// <summary>
    /// TCP port of the relay's HTTP API (default 9997), used for the live-now list. Held
    /// as a nullable decimal because that is exactly what a NumericUpDown binds to, an
    /// emptied field included; <see cref="Target"/> narrows it once and refuses the rest.
    /// </summary>
    public decimal? ApiPort
    {
        get => _apiPort;
        set
        {
            if (Set(ref _apiPort, value))
            {
                Apply();
            }
        }
    }

    /// <summary>Whether the poller keeps refreshing, or only answers the check button.</summary>
    public bool AutoRefresh
    {
        get => _autoRefresh;
        set
        {
            if (Set(ref _autoRefresh, value))
            {
                Apply();
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private string _statusLabel = "";
    private Color _statusColor;
    private bool _statusPulses;
    private bool _statusSpins;
    private bool _statusDotVisible;
    private string _errorText = "";
    private bool _hasError;
    private string _summary = "";
    private bool _isEmpty;
    private bool _canCheck;

    public ObservableCollection<PathRow> Paths { get; }

    public DelegateCommand CheckNowCommand { get; }

    public string StatusLabel { get => _statusLabel; private set => Set(ref _statusLabel, value); }

    public Color StatusColor { get => _statusColor; private set => Set(ref _statusColor, value); }

    public bool StatusPulses { get => _statusPulses; private set => Set(ref _statusPulses, value); }

    public bool StatusSpins { get => _statusSpins; private set => Set(ref _statusSpins, value); }

    public bool StatusDotVisible { get => _statusDotVisible; private set => Set(ref _statusDotVisible, value); }

    public string ErrorText { get => _errorText; private set => Set(ref _errorText, value); }

    public bool HasError { get => _hasError; private set => Set(ref _hasError, value); }

    /// <summary>The one-line count under the header: how many paths, how many watchers.</summary>
    public string Summary { get => _summary; private set => Set(ref _summary, value); }

    public bool IsEmpty { get => _isEmpty; private set => Set(ref _isEmpty, value); }

    public bool CanCheck { get => _canCheck; private set => Set(ref _canCheck, value); }

    // --- Lifecycle ----------------------------------------------------------------

    /// <summary>
    /// Brings the view up. Idempotent, and the same call the render function makes, so
    /// starting an already-started view model changes nothing.
    /// </summary>
    public void Start() => Apply();

    /// <summary>
    /// The one render function. Reads the poller through on every pass and never keeps a
    /// copy of what it found. Safe to run twice: with unchanged state no property differs,
    /// so no binding fires.
    /// </summary>
    public void Apply()
    {
        var target = Target;

        // Reconciling from the render pass is what keeps the loop honest: editing the host
        // retargets the poller through the same path that first started it.
        _poller.Reconcile(AutoRefresh ? target : null);

        var status = _poller.Latest;
        var kind = StatusFaces.KindOf(status, _poller.IsPolling);
        var face = StatusFaces.Of(kind);

        StatusLabel = face.Label;
        StatusColor = face.Color;
        StatusPulses = face.Pulses;
        StatusSpins = face.Spins;
        StatusDotVisible = !face.Spins;

        HasError = kind == StatusKind.Failed && status.Error.Length > 0;
        ErrorText = HasError ? status.Error : "";

        var rows = status.Reachable
            ? status.Paths.Select(PathRow.From).ToList()
            : [];
        Reconcile(rows);

        IsEmpty = rows.Count == 0;
        Summary = SummaryOf(kind, status);

        // Not gated on IsPolling: with auto-refresh on there is a poll in flight every
        // couple of seconds, and a button that greys itself out on each one reads as a
        // fault rather than as progress. An overlapping check is harmless - both answers
        // land through the same write.
        CanCheck = target is not null;
        CheckNowCommand.Refresh();

        Assert.That(HasError == (ErrorText.Length > 0), "an error banner and its text agree", HasError, ErrorText);
        Assert.That(IsEmpty == (Paths.Count == 0), "the empty state and the list agree", IsEmpty, Paths.Count);
    }

    /// <summary>
    /// The desired poll target, narrowed from the inputs the user owns, and null while
    /// they do not name a relay. Refusing here is what keeps the client's preconditions
    /// from being an assertion the user can trip by clearing a field.
    /// </summary>
    private RelayTarget? Target
    {
        get
        {
            var host = Host.Trim();
            if (host.Length == 0 || _apiPort is not (> 0 and <= 65535))
            {
                return null;
            }

            return new RelayTarget(host, (int)_apiPort.Value, RefreshInterval);
        }
    }

    private void CheckNow()
    {
        var target = Assert.NotNull(Target, "the check button is enabled only for a named relay");

        // Fire and forget: the poller reports the answer through Changed like any other
        // poll, and a failure to reach the relay is part of that answer rather than a throw.
        _ = _poller.CheckOnceAsync(target);
    }

    /// <summary>
    /// Converges the bound collection onto the rendered rows. Rows are records, so an
    /// unchanged snapshot compares equal and the list is left untouched - which is what
    /// keeps a two-second refresh from resetting scroll position and selection.
    /// </summary>
    private void Reconcile(IReadOnlyList<PathRow> rows)
    {
        if (Paths.SequenceEqual(rows))
        {
            return;
        }

        Paths.Clear();
        foreach (var row in rows)
        {
            Paths.Add(row);
        }

        Assert.That(Paths.Count == rows.Count, "a row per rendered path", Paths.Count, rows.Count);
    }

    private static string SummaryOf(StatusKind kind, RelayStatus status)
    {
        if (kind != StatusKind.Live)
        {
            return "";
        }

        var live = status.Paths.Count(path => path.Ready);
        var watching = status.Paths.Sum(path => path.Readers);
        return $"{live} live · {watching} watching";
    }
}
