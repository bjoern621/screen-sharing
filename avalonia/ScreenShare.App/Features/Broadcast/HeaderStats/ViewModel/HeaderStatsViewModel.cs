using System.Collections.ObjectModel;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.HeaderStats.Model;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.HeaderStats.ViewModel;

/// <summary>
/// Header stat bar: sharing pill and promoted figures.
/// Promotion pays only while the row stays short enough to read at a glance.
///
/// <b>Input</b>: the reading, written from above.
/// <b>Outputs</b>: <see cref="Apply"/> sets every one on every pass.
/// </summary>
public sealed class HeaderStatsViewModel : Observable
{
    /// <summary>Asserted at the foot of <see cref="Apply"/>.</summary>
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
    private string _retryCause = "";
    private bool _hasRetryCause;
    private string _retryMessage = "";
    private bool _hasRetryMessage;

    public ObservableCollection<StatFigure> Figures { get; }

    /// <summary>Pill's running timer, zero-padded: <c>01:07:44</c>.</summary>
    public string Elapsed { get => _elapsed; private set => Set(ref _elapsed, value); }

    /// <summary>
    /// Whether the pill shows at all.
    /// No off-air pill: absence reads as not live, and the red is never spent on an idle state.
    /// </summary>
    public bool IsSharing { get => _isSharing; private set => Set(ref _isSharing, value); }

    /// <summary>
    /// Which relaunch the backend is waiting out, empty while none is.
    /// A stream between attempts was never stopped, so the pill stays up,
    /// and without this line a pipeline coming back reads as one carrying frames.
    /// </summary>
    public string Retry { get => _retry; private set => Set(ref _retry, value); }

    public bool IsRetrying { get => _isRetrying; private set => Set(ref _isRetrying, value); }

    /// <summary>
    /// What ended the pipeline the pending relaunch follows, empty where nothing named it.
    /// The counter says which attempt and not why, and why is the half a reader can act on.
    /// </summary>
    public string RetryCause { get => _retryCause; private set => Set(ref _retryCause, value); }

    public bool HasRetryCause { get => _hasRetryCause; private set => Set(ref _hasRetryCause, value); }

    /// <summary>
    /// That pipeline's own last words, verbatim and selectable: the string a reader takes to a search box
    /// or a bug report.
    /// </summary>
    public string RetryMessage { get => _retryMessage; private set => Set(ref _retryMessage, value); }

    public bool HasRetryMessage { get => _hasRetryMessage; private set => Set(ref _hasRetryMessage, value); }

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

        // Both belong to the relaunch: a cause left standing under a stream carrying frames would describe
        // a running pipeline.
        RetryCause = IsRetrying ? Statements.Of(reading.RetryCause) : "";
        HasRetryCause = RetryCause.Length > 0;
        RetryMessage = IsRetrying ? reading.RetryMessage : "";
        HasRetryMessage = RetryMessage.Length > 0;

        Reconcile.Onto(Figures,
        [
            new StatFigure(Figure.Of(reading.EgressMbps, "0.00"), "Mb/s"),
            new StatFigure(Figure.Of(reading.Fps, "0.0"), "fps"),
            // Unit names the stage: capture to encoded, the one part of a viewer's delay this machine causes,
            // not the windows the transports hold packets for.
            // The whole path is the viewer's panel to show, this end seeing only its own.
            new StatFigure(Figure.Of(reading.EncodeMs, "0.0"), "ms encode"),
            // Worst viewer's, said in the unit: an unqualified "ms rtt" beside a viewer count reads
            // as the stream's own, a figure nobody took (Model/BroadcastSnapshot.cs).
            new StatFigure(Figure.Of(reading.RttMs), "ms rtt worst", Untimed(reading, reading.RttMs)),
            new StatFigure(Figure.Of(reading.LossPercent, "0.00"), "% loss worst", Untimed(reading, reading.LossPercent)),
            new StatFigure(Figure.Of(reading.Viewers), "viewers"),
        ]);

        Assert.That(Figures.Count == PromotedCount, "five figures are promoted into the header", Figures.Count);
        Assert.That(IsRetrying == (Retry.Length > 0), "a retry note appears with the retry it describes", IsRetrying, Retry);
        Assert.That(IsRetrying || (RetryCause.Length == 0 && RetryMessage.Length == 0),
            "a cause belongs to the relaunch it is about", RetryCause, RetryMessage);
        Assert.That(HasRetryCause == (RetryCause.Length > 0), "a cause and its sentence agree", HasRetryCause);
        Assert.That(HasRetryMessage == (RetryMessage.Length > 0), "the raw words and their presence agree", HasRetryMessage);
    }

    /// <summary>
    /// Why a latency figure reads as unmeasured, null where nothing needs saying.
    ///
    /// Said only while the relay names a reader on the path: viewers and no round trip looks like a broken
    /// measurement and is a leg nobody times.
    /// An empty roster is answered by the viewer count beside it, a stream that is not live by the missing pill.
    ///
    /// Wording is the plot's own (<see cref="Cards.Untimed"/>): both surfaces read one roster, so a second
    /// phrasing would be a second answer.
    /// </summary>
    private static string? Untimed(BroadcastSnapshot reading, double? figure)
        => figure is null && reading.Viewers > 0 ? Cards.Untimed(reading.Legs) : null;
}
