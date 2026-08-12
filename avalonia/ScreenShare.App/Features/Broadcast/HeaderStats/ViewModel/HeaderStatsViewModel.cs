using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.HeaderStats.Model;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.HeaderStats.ViewModel;

/// <summary>
/// The header stat bar: the sharing pill, and the five numbers a publisher actually
/// watches. Five and no more - the promotion only means something while the list is short
/// enough to read at a glance.
///
/// <b>Input</b> is the reading, written from above. <b>Outputs</b> are owned by
/// <see cref="Apply"/> alone, which sets every one of them on every pass.
/// </summary>
public sealed class HeaderStatsViewModel : Observable
{
    /// <summary>How many figures the design promotes. The count is the design, not a coincidence.</summary>
    private const int PromotedCount = 5;

    public HeaderStatsViewModel()
    {
        Figures = [];
        Apply();
    }

    // --- Inputs -------------------------------------------------------------------

    private BroadcastSnapshot _snapshot = BroadcastSnapshot.Unread;

    /// <summary>The reading the bar renders. Writing one re-runs the render function.</summary>
    public BroadcastSnapshot Snapshot
    {
        get => _snapshot;
        set
        {
            Assert.NotNull(value, "a stat bar renders a reading");

            if (Set(ref _snapshot, value))
            {
                Apply();
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private string _elapsed = "";
    private bool _isSharing;
    private string _retry = "";
    private bool _isRetrying;

    public ObservableCollection<StatFigure> Figures { get; }

    /// <summary>The pill's running timer, zero-padded <c>HH:MM:SS</c>.</summary>
    public string Elapsed { get => _elapsed; private set => Set(ref _elapsed, value); }

    /// <summary>
    /// Whether the pill shows at all. The design draws no off-air pill, so the honest
    /// rendering of a stream that is not live is the pill's absence rather than a second
    /// label spending the one red on something that is merely idle.
    /// </summary>
    public bool IsSharing { get => _isSharing; private set => Set(ref _isSharing, value); }

    /// <summary>
    /// Which relaunch the backend is waiting out, empty while none is. A stream between
    /// attempts is still a stream the reader asked for and has not stopped, so the pill stays
    /// and this says what is happening behind it - without it, a pipeline that died and is
    /// coming back looks identical to one carrying frames.
    /// </summary>
    public string Retry { get => _retry; private set => Set(ref _retry, value); }

    public bool IsRetrying { get => _isRetrying; private set => Set(ref _isRetrying, value); }

    /// <summary>
    /// The one render function. Reads the snapshot through on every pass, formats each
    /// figure through <see cref="Figure"/> so a missing sample prints as an ellipsis
    /// rather than as a zero, and rebuilds the row list only when a row actually differs.
    /// </summary>
    public void Apply()
    {
        var reading = Snapshot;

        IsSharing = reading.IsLive;
        Elapsed = reading.Elapsed;
        IsRetrying = reading.IsRetrying;
        Retry = IsRetrying ? $"reconnecting — attempt {reading.Attempt} of {reading.Budget}" : "";

        Reconcile.Onto(Figures,
        [
            new StatFigure(Figure.Of(reading.EgressMbps, "0.00"), "Mb/s"),
            new StatFigure(Figure.Of(reading.Fps, "0.0"), "fps"),
            // The relay measures round trip and loss per viewer, so neither figure has a
            // stream-wide value to promote and each of these is the worst viewer's. The label
            // says so: a bare "ms rtt" beside a viewer count reads as the stream's round trip,
            // which is a figure nobody took (Model/BroadcastSnapshot.cs).
            new StatFigure(Figure.Of(reading.RttMs), "ms rtt worst", Untimed(reading, reading.RttMs)),
            new StatFigure(Figure.Of(reading.LossPercent, "0.00"), "% loss worst", Untimed(reading, reading.LossPercent)),
            new StatFigure(Figure.Of(reading.Viewers), "viewers"),
        ]);

        Assert.That(Figures.Count == PromotedCount, "five figures are promoted into the header", Figures.Count);
        Assert.That(IsRetrying == (Retry.Length > 0), "a retry note appears with the retry it describes", IsRetrying, Retry);
    }

    /// <summary>
    /// Why one of the two latency figures reads as unmeasured, and null where it needs no saying.
    ///
    /// It is said only while the relay names a reader on the path, because that is the one state
    /// in which the ellipsis is worth explaining. A stream with viewers and no round trip looks
    /// like a broken measurement and is a leg nobody times, which is a thing the publisher can
    /// act on. A stream nobody is watching is already explained by the viewer count beside it,
    /// and a stream that is not live is explained by the pill that is not there.
    ///
    /// The sentence itself is the plot's (<see cref="Cards.Untimed"/>), not a second wording of
    /// it: both surfaces describe the same roster and a reader moving between them is entitled to
    /// one answer.
    /// </summary>
    private static string? Untimed(BroadcastSnapshot reading, double? figure)
        => figure is null && reading.Viewers > 0 ? Cards.Untimed(reading.Legs) : null;
}
