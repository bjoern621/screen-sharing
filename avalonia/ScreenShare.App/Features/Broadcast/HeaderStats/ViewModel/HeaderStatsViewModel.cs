using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Broadcast.HeaderStats.Model;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.HeaderStats.ViewModel;

/// <summary>
/// The header stat bar: the on-air pill, and the five numbers a publisher actually
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
    private bool _isOnAir;
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
    public bool IsOnAir { get => _isOnAir; private set => Set(ref _isOnAir, value); }

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

        IsOnAir = reading.IsLive;
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
            new StatFigure(Figure.Of(reading.RttMs), "ms rtt worst"),
            new StatFigure(Figure.Of(reading.LossPercent, "0.00"), "% loss worst"),
            new StatFigure(Figure.Of(reading.Viewers), "viewers"),
        ]);

        Assert.That(Figures.Count == PromotedCount, "five figures are promoted into the header", Figures.Count);
        Assert.That(IsRetrying == (Retry.Length > 0), "a retry note appears with the retry it describes", IsRetrying, Retry);
    }
}
