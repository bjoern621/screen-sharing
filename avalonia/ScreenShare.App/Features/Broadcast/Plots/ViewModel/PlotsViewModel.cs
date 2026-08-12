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
/// The annotations are the whole scale. With no axes and no ticks, the ceiling label and
/// the band label are the only things that say what a height or a moment means, so they
/// are derived from the same reading as the header figures rather than written into the
/// markup - a ceiling that disagreed with the encoder would be worse than no ceiling.
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
    /// The encoder samples of this run, oldest first, as the session accumulated them. The
    /// reading beside them says what the newest one holds; this is the shape it got there by.
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
    /// The relay snapshots of this run, oldest first, the same way. Which path in them is this
    /// stream's is the reading's answer, so the two are read together on every pass rather than
    /// this holding a name of its own.
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
    private string _band = "";
    private bool _hasEgress;
    private bool _hasLatency;
    private string _egressNotice = "";
    private string _latencyNotice = "";

    /// <summary>The coordinate space the series below are expressed in.</summary>
    public Size Extent { get => _extent; private set => Set(ref _extent, value); }

    public IReadOnlyList<Point> Egress { get => _egress; private set => Set(ref _egress, value); }

    /// <summary>
    /// The round trip to the worst-off viewer, one point per relay snapshot. Empty while the
    /// relay times nobody on this path - which is every snapshot with no viewer on a leg it
    /// measures, not a stream that is doing well.
    /// </summary>
    public IReadOnlyList<Point> Rtt { get => _rtt; private set => Set(ref _rtt, value); }

    /// <summary>
    /// The send-side loss to the worst-off viewer, over the same window. It replaces the
    /// design's buffer-fill series, which named a figure the viewer knows and never tells the
    /// publisher.
    /// </summary>
    public IReadOnlyList<Point> Loss { get => _loss; private set => Set(ref _loss, value); }

    /// <summary>The label naming the ceiling the running pipeline was built with.</summary>
    public string Ceiling { get => _ceiling; private set => Set(ref _ceiling, value); }

    /// <summary>
    /// Where the rule marking that ceiling sits, 0 at the top to 1 at the bottom, and
    /// <see cref="double.NaN"/> where the ceiling falls outside the drawn range and no rule is
    /// drawn. Derived from the curve's own scale rather than fixed by the design: a rule at a
    /// constant height would say the ceiling is wherever the mockup put it.
    /// </summary>
    public double CeilingFraction { get => _ceilingFraction; private set => Set(ref _ceilingFraction, value); }

    /// <summary>
    /// How much stream the plot covers, e.g. <c>60 s</c>, empty where it draws no curve. It is
    /// the axis rather than a measurement of the run: the width is that span whether or not the
    /// stream has been up that long, and the curve fills as much of it as has happened.
    /// </summary>
    public string Window { get => _window; private set => Set(ref _window, value); }

    /// <summary>The label over the shaded band: when the congestion the band marks happened.</summary>
    public string Band { get => _band; private set => Set(ref _band, value); }

    /// <summary>Whether the egress curve has a shape to draw. False before a run has two samples.</summary>
    public bool HasEgress { get => _hasEgress; private set => Set(ref _hasEgress, value); }

    /// <summary>Whether the latency plot has a shape to draw. False before two snapshots have timed somebody.</summary>
    public bool HasLatency { get => _hasLatency; private set => Set(ref _hasLatency, value); }

    /// <summary>What stands in for the egress curve while there is none.</summary>
    public string EgressNotice { get => _egressNotice; private set => Set(ref _egressNotice, value); }

    /// <summary>What stands in for the latency curves while there are none.</summary>
    public string LatencyNotice { get => _latencyNotice; private set => Set(ref _latencyNotice, value); }

    /// <summary>
    /// The one render function. The egress curve is the encoder samples' own shape and the two
    /// latency curves are the relay snapshots': each is drawn where it has something to draw and
    /// says why where it has not, because a shape with no measurement behind it would read as one.
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

        // Four absences, four facts, and the order they are asked in is the order they stop being
        // true in as a stream comes up. The third is the one that would otherwise be told as the
        // fourth and be a lie: a stream the relay has timed once has a measurement and no shape
        // yet, because one point is a reading and not a curve. The fourth is the one worth
        // spelling out - SRT is the only leg the relay times, so a stream watched entirely over
        // RTSP or a browser has viewers and no latency to plot, which is not the same as a stream
        // nobody is watching.
        HasLatency = Rtt.Count > 0;
        LatencyNotice = HasLatency ? ""
            : !reading.IsLive ? "nothing is publishing"
            : reading.Viewers is null or 0 ? "nobody is watching yet"
            : reading.RttMs is not null ? "waiting for the relay's next snapshot"
            : Cards.Untimed(reading.Legs);

        // The label names the axis, and the axis is fixed: the card is a minute of stream wide
        // whether or not a minute of it has happened yet, so a young run is a curve against the
        // right edge rather than one stretched over a span it does not cover. It is read off the
        // constant the points are placed by, so the two cannot come to say different things.
        Window = HasEgress ? $"{PlotSeries.WindowSeconds:0} s" : "";

        // The band names a congestion window nothing detects, so it carries the word alone
        // rather than a timestamp that would be invented. The ceiling is the setting the
        // running pipeline was built with, so it moves with the stream, and the rule marking it
        // is placed against the curve's own scale rather than drawn wherever the design put it.
        Ceiling = $"vbv ceiling {Figure.Of(reading.VbvCeilingMbps, "0")} Mb/s";
        CeilingFraction = PlotSeries.CeilingFraction(Samples, reading.VbvCeilingMbps);
        Band = reading.CongestionAt.Length > 0 ? $"congestion {reading.CongestionAt}" : "";

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
