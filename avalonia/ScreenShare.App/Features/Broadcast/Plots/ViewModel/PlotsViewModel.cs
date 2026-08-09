using Avalonia;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
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

    // --- Outputs ------------------------------------------------------------------

    private Size _extent;
    private IReadOnlyList<Point> _egress = [];
    private IReadOnlyList<Point> _rtt = [];
    private IReadOnlyList<Point> _buffer = [];
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

    /// <summary>Empty always: nothing in the pipeline measures a round trip.</summary>
    public IReadOnlyList<Point> Rtt { get => _rtt; private set => Set(ref _rtt, value); }

    /// <summary>Empty always: nothing in the pipeline measures buffer fill.</summary>
    public IReadOnlyList<Point> Buffer { get => _buffer; private set => Set(ref _buffer, value); }

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
    /// How much stream the plot covers, e.g. <c>240 s</c>, empty where it covers none. It is
    /// measured off the samples on screen rather than stated as a fixed window, because the run
    /// is younger than the window it will eventually fill.
    /// </summary>
    public string Window { get => _window; private set => Set(ref _window, value); }

    /// <summary>The label over the shaded band: when the congestion the band marks happened.</summary>
    public string Band { get => _band; private set => Set(ref _band, value); }

    /// <summary>Whether the egress curve has a shape to draw. False before a run has two samples.</summary>
    public bool HasEgress { get => _hasEgress; private set => Set(ref _hasEgress, value); }

    /// <summary>Whether the latency plot has anything at all. Never true, for the reason its notice states.</summary>
    public bool HasLatency { get => _hasLatency; private set => Set(ref _hasLatency, value); }

    /// <summary>What stands in for the egress curve while there is none.</summary>
    public string EgressNotice { get => _egressNotice; private set => Set(ref _egressNotice, value); }

    /// <summary>What stands in for the latency curves, which is the same sentence on every pass.</summary>
    public string LatencyNotice { get => _latencyNotice; private set => Set(ref _latencyNotice, value); }

    /// <summary>
    /// The one render function. The egress curve is the samples' own shape; the two latency
    /// series draw nothing and say why, because a shape with no measurement behind it would
    /// read as one.
    /// </summary>
    public void Apply()
    {
        var reading = Snapshot;

        Extent = PlotSeries.Extent;
        Egress = PlotSeries.Egress(Samples);
        Rtt = [];
        Buffer = [];

        HasEgress = Egress.Count > 0;
        EgressNotice = HasEgress ? ""
            : reading.IsLive ? "waiting for the encoder's first samples"
            : "nothing is publishing";

        HasLatency = false;
        LatencyNotice = "round trip and buffer fill are measured by nothing in the pipeline";

        // The window is the samples' own span. It is stated rather than fixed because the plot
        // stretches whatever the session holds across the card: a run a minute old and a run an
        // hour old fill the same width, and a constant label under one of them is wrong.
        var span = PlotSeries.Span(Samples);
        Window = HasEgress && span is not null ? $"{span.Value:0} s" : "";

        // The band names a congestion window nothing detects, so it carries the word alone
        // rather than a timestamp that would be invented. The ceiling is the setting the
        // running pipeline was built with, so it moves with the stream, and the rule marking it
        // is placed against the curve's own scale rather than drawn wherever the design put it.
        Ceiling = $"vbv ceiling {Figure.Of(reading.VbvCeilingMbps, "0")} Mb/s";
        CeilingFraction = PlotSeries.CeilingFraction(Samples, reading.VbvCeilingMbps);
        Band = reading.CongestionAt.Length > 0 ? $"congestion {reading.CongestionAt}" : "";

        Assert.That(Extent.Width > 0 && Extent.Height > 0,
            "a plot maps from a source space with area", Extent.Width, Extent.Height);
        Assert.That(Rtt.Count == Buffer.Count,
            "the two latency series cover the same window", Rtt.Count, Buffer.Count);
        Assert.That(HasEgress == (EgressNotice.Length == 0),
            "a curve and the sentence standing in for it are never both on screen", HasEgress, EgressNotice);
        Assert.That(double.IsNaN(CeilingFraction) || CeilingFraction is >= 0 and <= 1,
            "a ceiling rule that is drawn sits inside the plot", CeilingFraction);
        Assert.That(HasEgress || Window.Length == 0,
            "a plot with no curve states no window", HasEgress, Window);
    }
}
