using Avalonia;
using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Features.Broadcast.Plots.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Broadcast.Plots.ViewModel;

/// <summary>
/// The two sparklines: what is going out, and what the far end is doing with it.
///
/// The annotations carry the whole scale.
/// With no axes and no ticks, the ceiling label and the window label are the only things saying what a height
/// or a moment means, so both are derived from the reading the header figures come from rather than written
/// into the markup.
/// </summary>
public sealed class PlotsViewModel : Observable
{
    public PlotsViewModel() => Apply();

    // --- Inputs -------------------------------------------------------------------

    private BroadcastSnapshot _snapshot = BroadcastSnapshot.Unread;
    private IReadOnlyList<PublishStats> _samples = [];
    private IReadOnlyList<RelayReading> _relaySamples = [];

    public BroadcastSnapshot Snapshot
    {
        get => _snapshot;
        set
        {
            Assert.NotNull(value, "a plot renders a reading");

            if (Set(ref _snapshot, value))
            {
                Apply();
            }
        }
    }

    /// <summary>
    /// Encoder samples of this run, oldest first, owned and evicted by the session.
    /// Nothing is kept here: each pass windows the list again by the clock every sample carries.
    /// </summary>
    public IReadOnlyList<PublishStats> Samples
    {
        get => _samples;
        set
        {
            Assert.NotNull(value, "a plot renders the samples that were taken");

            if (Set(ref _samples, value))
            {
                Apply();
            }
        }
    }

    /// <summary>
    /// Relay snapshots of this run, oldest first, on the same terms.
    /// Which path in them is this stream's is the reading's answer, so both are read together on every pass
    /// and no stream name is held here.
    /// </summary>
    public IReadOnlyList<RelayReading> RelaySamples
    {
        get => _relaySamples;
        set
        {
            Assert.NotNull(value, "a plot renders the relay snapshots that were taken");

            if (Set(ref _relaySamples, value))
            {
                Apply();
            }
        }
    }

    // --- Outputs ------------------------------------------------------------------

    private Size _extent;
    private IReadOnlyList<Point> _egress = [];
    private IReadOnlyList<Point> _rtt = [];
    private IReadOnlyList<Point> _loss = [];
    private string _ceiling = "";
    private double _ceilingFraction = double.NaN;
    private string _window = "";
    private bool _hasEgress;
    private bool _hasLatency;
    private string _egressNotice = "";
    private string _latencyNotice = "";

    /// <summary>Source space the series below are placed in.</summary>
    public Size Extent { get => _extent; private set => Set(ref _extent, value); }

    public IReadOnlyList<Point> Egress { get => _egress; private set => Set(ref _egress, value); }

    /// <summary>
    /// Round trip to the worst-off viewer, one point per relay snapshot.
    /// Empty while the relay times nobody on this path, which is a snapshot with no viewer on a leg it
    /// measures and not a stream that is doing well.
    /// </summary>
    public IReadOnlyList<Point> Rtt { get => _rtt; private set => Set(ref _rtt, value); }

    /// <summary>Send-side loss to the worst-off viewer, over the same window as <see cref="Rtt"/>.</summary>
    public IReadOnlyList<Point> Loss { get => _loss; private set => Set(ref _loss, value); }

    /// <summary>Label naming the ceiling the running pipeline was built with: <c>vbv ceiling 12 Mb/s</c>.</summary>
    public string Ceiling { get => _ceiling; private set => Set(ref _ceiling, value); }

    /// <summary>
    /// Height of the rule marking that ceiling, 0 at the top to 1 at the bottom, and <see cref="double.NaN"/>
    /// where the ceiling falls outside the drawn range and no rule is drawn.
    /// Derived from the curve's own scale, since a rule at a constant height marks the ceiling only by
    /// coincidence.
    /// </summary>
    public double CeilingFraction { get => _ceilingFraction; private set => Set(ref _ceilingFraction, value); }

    /// <summary>
    /// Span the plot covers, <c>60 s</c>, empty where it draws no curve.
    /// It names the axis and not the run: the width is that span whether or not the stream has been up that
    /// long, and the curve fills as much of it as has happened.
    /// </summary>
    public string Window { get => _window; private set => Set(ref _window, value); }

    /// <summary>
    /// The latency plot's legend, beside the rule drawn in each series' stroke.
    /// Fixed words, from the one place the on-screen words live (<c>avalonia/README.md</c>, "Layout").
    /// </summary>
    public static string RoundTripLegend => Cards.PlotRoundTripLegend;

    public static string LossLegend => Cards.PlotLossLegend;

    /// <summary>Whether the egress curve has a shape. False until a run has two samples.</summary>
    public bool HasEgress { get => _hasEgress; private set => Set(ref _hasEgress, value); }

    /// <summary>Whether the latency plot has a shape. False until two snapshots have timed somebody.</summary>
    public bool HasLatency { get => _hasLatency; private set => Set(ref _hasLatency, value); }

    /// <summary>Stands in for the egress curve while there is none.</summary>
    public string EgressNotice { get => _egressNotice; private set => Set(ref _egressNotice, value); }

    /// <summary>Stands in for the latency curves while there are none.</summary>
    public string LatencyNotice { get => _latencyNotice; private set => Set(ref _latencyNotice, value); }

    /// <summary>
    /// The one render function.
    /// Each curve is drawn where it has a shape and says why where it has not, because a shape with no
    /// measurement behind it reads as a measurement.
    /// </summary>
    public void Apply()
    {
        var reading = Snapshot;

        Extent = PlotSeries.Extent;
        Egress = PlotSeries.Egress(Samples);

        var latency = PlotSeries.Latency(RelaySamples, reading.Stream);
        Rtt = latency.Rtt;
        Loss = latency.Loss;

        HasEgress = Egress.Count > 0;
        EgressNotice = HasEgress ? ""
            : reading.IsLive ? "waiting for the encoder's first samples"
            : "nothing is publishing";

        // Four absences, asked in the order they stop being true as a stream comes up.
        // The third would otherwise be told as the fourth and be a lie: a path timed once has a measurement
        // and no curve yet, since one point is a reading and not a shape.
        // The fourth names the legs, because SRT is the leg the relay times: a stream watched over RTSP or a
        // browser has viewers and nothing to plot.
        HasLatency = Rtt.Count > 0;
        LatencyNotice = HasLatency ? ""
            : !reading.IsLive ? "nothing is publishing"
            : reading.Viewers is null or 0 ? "nobody is watching yet"
            : reading.RttMs is not null ? "waiting for the relay's next snapshot"
            : Cards.Untimed(reading.Legs);

        // Read off the constant the points are placed by, so the label and the axis cannot disagree.
        Window = HasEgress ? $"{PlotSeries.WindowSeconds:0} s" : "";

        Ceiling = $"vbv ceiling {Figure.Of(reading.VbvCeilingMbps, "0")} Mb/s";
        CeilingFraction = PlotSeries.CeilingFraction(Samples, reading.VbvCeilingMbps);

        Assert.That(Extent.Width > 0 && Extent.Height > 0,
            "a plot maps from a source space with area", Extent.Width, Extent.Height);
        Assert.That(Rtt.Count == Loss.Count,
            "the two latency series cover the same window", Rtt.Count, Loss.Count);
        Assert.That(HasEgress == (EgressNotice.Length == 0),
            "a curve and the sentence standing in for it are never both on screen", HasEgress, EgressNotice);
        Assert.That(HasLatency == (LatencyNotice.Length == 0),
            "a latency curve and the sentence standing in for it are never both on screen", HasLatency, LatencyNotice);
        Assert.That(double.IsNaN(CeilingFraction) || CeilingFraction is >= 0 and <= 1,
            "a ceiling rule that is drawn sits inside the plot", CeilingFraction);
        Assert.That(HasEgress || Window.Length == 0,
            "a plot with no curve states no window", HasEgress, Window);
    }
}
