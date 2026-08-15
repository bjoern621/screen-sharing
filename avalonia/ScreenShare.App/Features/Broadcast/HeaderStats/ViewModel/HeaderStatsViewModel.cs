using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.HeaderStats.Model;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.HeaderStats.ViewModel;

/// <summary>
/// Header stat bar: the sharing pill and the promoted figures.
/// Promotion is worth something only while the row stays short enough to read at a glance.
///
/// <b>Input</b> is the reading, written from above.
/// <b>Outputs</b> belong to <see cref="Apply"/>, which sets every one of them on every pass.
/// </summary>
public sealed class HeaderStatsViewModel : Observable
{
    /// <summary>Figures the design promotes, asserted at the foot of <see cref="Apply"/>.</summary>
    private const int PromotedCount = 6;

    public HeaderStatsViewModel()
    {
        Figures = [];
        Apply();
    }

    // --- Inputs, written from above -----------------------------------------------

    private BroadcastSnapshot _snapshot = BroadcastSnapshot.Unread;

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

    // --- Outputs, written by Apply alone -------------------------------------------

    private string _elapsed = "";
    private bool _isSharing;
    private string _retry = "";
    private bool _isRetrying;

    public ObservableCollection<StatFigure> Figures { get; }

    /// <summary>Pill's running timer, zero-padded: <c>01:07:44</c>.</summary>
    public string Elapsed { get => _elapsed; private set => Set(ref _elapsed, value); }

    /// <summary>
    /// Whether the pill shows at all.
    /// There is no off-air pill: a stream that is not live reads as the pill's absence, and the one red is
    /// never spent on an idle state.
    /// </summary>
    public bool IsSharing { get => _isSharing; private set => Set(ref _isSharing, value); }

    /// <summary>
    /// Which relaunch the backend is waiting out, empty while none is.
    /// A stream between attempts is one the reader never stopped, so the pill stays up and this says what is
    /// happening behind it.
    /// Without it a pipeline that died and is coming back reads as one carrying frames.
    /// </summary>
    public string Retry { get => _retry; private set => Set(ref _retry, value); }

    public bool IsRetrying { get => _isRetrying; private set => Set(ref _isRetrying, value); }

    /// <summary>
    /// One render function.
    /// Reads the snapshot through on every pass and formats through <see cref="Figure"/>, so a missing sample
    /// prints an ellipsis and never a zero.
    /// The row list is rebuilt only where a row differs, so a repeated pass raises no notification.
    /// </summary>
    public void Apply()
    {
        var reading = Snapshot;

        IsSharing = reading.IsLive;
        Elapsed = reading.Elapsed;
        IsRetrying = reading.IsRetrying;
        Retry = IsRetrying ? Cards.RetryAttempt(reading.Attempt, reading.Budget) : "";

        Reconcile.Onto(Figures,
        [
            new StatFigure(Figure.Of(reading.EgressMbps, "0.00"), "Mb/s"),
            new StatFigure(Figure.Of(reading.Fps, "0.0"), "fps"),
            // The unit names the stage, because the delay to a viewer has several and this is the one this
            // machine causes: capture to encoded, and not the windows the transports hold packets for.
            // The whole path is a viewer's panel to show, this being the end that cannot see the other.
            new StatFigure(Figure.Of(reading.EncodeMs, "0.0"), "ms encode"),
            // Round trip and loss are measured per viewer, so neither has a stream-wide value to promote.
            // Both are the worst viewer's, and the unit says so: an unqualified "ms rtt" beside a viewer
            // count reads as the stream's own, which is a figure nobody took (Model/BroadcastSnapshot.cs).
            new StatFigure(Figure.Of(reading.RttMs), "ms rtt worst", Untimed(reading, reading.RttMs)),
            new StatFigure(Figure.Of(reading.LossPercent, "0.00"), "% loss worst", Untimed(reading, reading.LossPercent)),
            new StatFigure(Figure.Of(reading.Viewers), "viewers"),
        ]);

        Assert.That(Figures.Count == PromotedCount, "five figures are promoted into the header", Figures.Count);
        Assert.That(IsRetrying == (Retry.Length > 0), "a retry note appears with the retry it describes", IsRetrying, Retry);
    }

    /// <summary>
    /// Why one of the two latency figures reads as unmeasured, null where nothing needs saying.
    ///
    /// Said only while the relay names a reader on the path, the one state where the ellipsis is worth
    /// explaining: viewers and no round trip looks like a broken measurement and is a leg nobody times.
    /// An empty roster is explained by the viewer count beside it, and a stream that is not live by the
    /// missing pill.
    ///
    /// The wording is the plot's own (<see cref="Cards.Untimed"/>): both surfaces read one roster, so a
    /// second phrasing would be a second answer.
    /// </summary>
    private static string? Untimed(BroadcastSnapshot reading, double? figure)
        => figure is null && reading.Viewers > 0 ? Cards.Untimed(reading.Legs) : null;
}
