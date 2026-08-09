using System.Collections.ObjectModel;
using System.Globalization;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Setup.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.CostRail.ViewModel;

/// <summary>
/// The price of the current configuration, and everything standing between it and going
/// live. The rail is beside the form rather than after it so a choice is priced while it is
/// being made; a reader who learns the cost on a confirmation screen has already committed.
///
/// <b>Every figure is the backend's.</b> It used to be the mockup's own numbers, which meant
/// the panel read 11.8 Mb/s against a 20 Mb/s uplink whatever the encoder was set to, and the
/// real prediction sat in a diagnostic under the form saying 1619 Mb/s. Both come off
/// <c>Summary.estimate</c> now, and the uplink the bar is measured against is the
/// <c>uplink_mbps</c> field of the same form - so moving that control moves this panel,
/// because the two are one value rather than two.
///
/// <b>Outputs</b> only, written by <see cref="Apply"/> on every pass, including the branch
/// with no form behind it. The uplink control the view binds is a field view model this class
/// holds a reference to rather than a copy of, so the write still leaves through the field the
/// reader moved.
/// </summary>
public sealed class CostRailViewModel : Observable
{
    /// <summary>
    /// The headroom the bar leaves above whichever of the two figures is larger, so a
    /// prediction that exactly meets the uplink does not paint the marker on the end cap.
    /// </summary>
    private const double Headroom = 1.15;

    // --- Outputs ------------------------------------------------------------------

    private string _bitrate = "";
    private string _bitrateCaption = "";
    private string _uplinkCaption = "";
    private string _checksSummary = "";
    private double _fillShare;
    private double _uplinkShare;
    private bool _isOverUplink;
    private bool _isResolved;
    private bool _hasUplink;
    private FieldViewModel? _uplink;

    /// <param name="measure">
    /// Asks the backend to measure the line and write what it finds into the uplink field, and
    /// answers when it has. Injected because it is an effect on the control plane, which this
    /// class has no seam to.
    /// </param>
    /// <param name="dispatch">The UI loop the measurement's answer is marshalled back to.</param>
    public CostRailViewModel(Func<Task> measure, Action<Action> dispatch, Func<bool> canMeasure)
    {
        Assert.NotNull(measure, "the rail needs somewhere to send a measure request");
        Assert.NotNull(dispatch, "the rail needs a UI loop to marshal the measurement back to");
        Assert.NotNull(canMeasure, "the rail needs to know when a measurement can be asked for");

        Metrics = [];
        Checks = [];

        // The measurement uploads a real payload and takes seconds. The command holds whether
        // one is in flight, which is both what the button waits on and what refuses a second
        // press - one fact rather than a flag here and a guard somewhere else.
        MeasureCommand = new PendingCommand(measure, dispatch, canMeasure);

        Apply(null, null, []);
    }

    /// <summary>The dimensions priced beside the headline rate, in the order the panel reads them.</summary>
    public ObservableCollection<CostMetricRow> Metrics { get; }

    /// <summary>Everything the form said about the settings as a whole, ranked where it ranked it.</summary>
    public ObservableCollection<PreflightCheckRow> Checks { get; }

    /// <summary>Measures this machine's real upload throughput and adopts the figure.</summary>
    public PendingCommand MeasureCommand { get; }

    /// <summary>The headline figure: megabits per second, as the backend predicts them.</summary>
    public string Bitrate { get => _bitrate; private set => Set(ref _bitrate, value); }

    public string BitrateCaption { get => _bitrateCaption; private set => Set(ref _bitrateCaption, value); }

    /// <summary>The limit the bar's red marker stands at. The one red in the panel.</summary>
    public string UplinkCaption { get => _uplinkCaption; private set => Set(ref _uplinkCaption, value); }

    /// <summary>
    /// The uplink control itself, so the figure the bar is measured against is edited where it
    /// is read. Null where the form carries no such field, which is the honest branch rather
    /// than a box that writes nowhere.
    /// </summary>
    public FieldViewModel? Uplink { get => _uplink; private set => Set(ref _uplink, value); }

    public bool HasUplink { get => _hasUplink; private set => Set(ref _hasUplink, value); }

    /// <summary>How much of the bar the prediction fills, 0 to 1.</summary>
    public double FillShare { get => _fillShare; private set => Set(ref _fillShare, value); }

    /// <summary>Where along the bar the uplink marker stands, 0 to 1.</summary>
    public double UplinkShare { get => _uplinkShare; private set => Set(ref _uplinkShare, value); }

    /// <summary>Whether the prediction is past the marker, which is the whole point of drawing both.</summary>
    public bool IsOverUplink { get => _isOverUplink; private set => Set(ref _isOverUplink, value); }

    /// <summary>False before the first form lands, when there is nothing to price.</summary>
    public bool IsResolved { get => _isResolved; private set => Set(ref _isResolved, value); }

    /// <summary>
    /// What the terminal chip says about this list. Derived here rather than restated on the
    /// chip, so the strip and the rail cannot disagree about how much is owed.
    /// </summary>
    public string ChecksSummary { get => _checksSummary; private set => Set(ref _checksSummary, value); }

    /// <summary>
    /// The one render function. Idempotent: every value is read out of the arguments, and the
    /// two lists are records reconciled onto, so a second pass over one form fires nothing.
    /// </summary>
    /// <param name="estimate">What the settings are predicted to cost, null before the first form.</param>
    /// <param name="uplink">The uplink control, null where the form does not carry one.</param>
    /// <param name="checks">The form's diagnostics, as the list draws them.</param>
    public void Apply(Estimate? estimate, FieldViewModel? uplink, IReadOnlyList<PreflightCheckRow> checks)
    {
        Assert.NotNull(checks, "the rail draws the list the form's diagnostics became");

        IsResolved = estimate is not null;
        Uplink = uplink;
        HasUplink = uplink is not null;

        var predicted = estimate?.BitrateMbps ?? 0;
        var raw = estimate?.RawMbps ?? 0;

        // The stated uplink, recovered from the prediction and the headroom the backend
        // computed against it, so the two figures on the bar are one arithmetic rather than
        // this class reading a settings field and hoping it is the one the headroom used.
        var capacity = estimate is null ? 0 : predicted + estimate.HeadroomMbps;
        var scale = Math.Max(Math.Max(predicted, capacity), 1) * Headroom;

        Bitrate = Figure(predicted);
        BitrateCaption = Caption(raw, predicted);
        UplinkCaption = capacity > 0 ? $"uplink {Figure(capacity)} Mb/s" : "no uplink stated";

        FillShare = Share(predicted, scale);
        UplinkShare = Share(capacity, scale);
        IsOverUplink = capacity > 0 && predicted > capacity;

        MeasureCommand.Refresh();

        Reconcile.Onto(Metrics, Rows(estimate));
        Reconcile.Onto(Checks, checks);
        ChecksSummary = PreflightChecks.SummaryOf(checks);

        Assert.That(FillShare is >= 0 and <= 1, "the bar's fill is a share of it", FillShare);
        Assert.That(UplinkShare is >= 0 and <= 1, "the uplink marker stands on the bar", UplinkShare);
        Assert.That(IsResolved || Metrics.Count == 0, "nothing is priced before a form arrives", Metrics.Count);
        Assert.That(ChecksSummary.Length > 0, "the terminal chip is told how much is owed", ChecksSummary);
    }

    /// <summary>
    /// The two figures that ride beside the headline. Both are the estimate's own: what the
    /// capture produces before coding, and what the line has left once the stream is on it.
    /// A negative headroom is not clamped - it is the number that says the line cannot carry
    /// this, and the diagnostic saying so in words is already in the list below.
    /// </summary>
    private static IReadOnlyList<CostMetricRow> Rows(Estimate? estimate)
    {
        if (estimate is null)
        {
            return [];
        }

        return
        [
            new() { Label = "Uncompressed capture", Value = $"{Figure(estimate.RawMbps)} Mb/s" },
            new() { Label = "Uplink headroom", Value = $"{Figure(estimate.HeadroomMbps)} Mb/s" },
        ];
    }

    /// <summary>
    /// What the headline figure is in, and what it is worth knowing beside it: the
    /// uncompressed rate the capture produces, and the ratio between the two, which is what
    /// makes the prediction legible as a compression rather than as a number.
    /// </summary>
    private static string Caption(double raw, double coded)
    {
        if (raw <= 0)
        {
            return "Mb/s predicted";
        }

        var ratio = coded > 0
            ? $" · {(raw / coded).ToString("0.#", CultureInfo.InvariantCulture)}:1"
            : "";

        return $"Mb/s predicted · {Figure(raw)} raw{ratio}";
    }

    private static string Figure(double mbps) => mbps.ToString(
        Math.Abs(mbps) >= 100 ? "0" : "0.#", CultureInfo.InvariantCulture);

    private static double Share(double value, double scale)
        => scale > 0 ? Math.Clamp(value / scale, 0, 1) : 0;
}
