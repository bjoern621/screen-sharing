using System.Collections.ObjectModel;
using System.Globalization;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.ViewModel;
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
/// <b>Outputs only, and that includes the uplink.</b> The panel used to carry the uplink's own
/// spinner and its Measure button, which made this rail a second place the setting was edited:
/// a figure the wizard already draws a control for, drawn again beside the bar. It reads the
/// figure now and names the step that owns it, so there is one control per setting and the
/// panel stays what it is - a reading of what the configuration costs.
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
    private string _uplinkLabel = "";
    private string _uplinkFigure = "";
    private string _uplinkHint = "";

    public CostRailViewModel()
    {
        Metrics = [];
        Checks = [];

        Apply(null, null, null, []);
    }

    /// <summary>The dimensions priced beside the headline rate, in the order the panel reads them.</summary>
    public ObservableCollection<CostMetricRow> Metrics { get; }

    /// <summary>Everything the form said about the settings as a whole, ranked where it ranked it.</summary>
    public ObservableCollection<PreflightCheckRow> Checks { get; }

    /// <summary>The headline figure: megabits per second, as the backend predicts them.</summary>
    public string Bitrate { get => _bitrate; private set => Set(ref _bitrate, value); }

    public string BitrateCaption { get => _bitrateCaption; private set => Set(ref _bitrateCaption, value); }

    /// <summary>The limit the bar's red marker stands at. The one red in the panel.</summary>
    public string UplinkCaption { get => _uplinkCaption; private set => Set(ref _uplinkCaption, value); }

    /// <summary>
    /// What the uplink figure is called, read off the field the form carries it as. Empty where
    /// the form offers no such field, which is the honest branch rather than a heading over
    /// nothing.
    /// </summary>
    public string UplinkLabel { get => _uplinkLabel; private set => Set(ref _uplinkLabel, value); }

    /// <summary>
    /// The stated uplink as the field reads it back, with its unit. It is a reading and not a
    /// control: the wizard already draws one for this setting, and a second box beside the bar
    /// would be two controls over one value.
    /// </summary>
    public string UplinkFigure { get => _uplinkFigure; private set => Set(ref _uplinkFigure, value); }

    /// <summary>
    /// Where the figure is changed and measured, named after the step that owns the control.
    /// Empty where nothing on this screen carries it, so the panel points at a step that exists
    /// or points nowhere.
    /// </summary>
    public string UplinkHint { get => _uplinkHint; private set => Set(ref _uplinkHint, value); }

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
    /// <param name="editedOn">The step that draws that control, null where no step of this flow does.</param>
    /// <param name="checks">The form's diagnostics, as the list draws them.</param>
    public void Apply(
        Estimate? estimate,
        FieldViewModel? uplink,
        SetupStepRow? editedOn,
        IReadOnlyList<PreflightCheckRow> checks)
    {
        Assert.NotNull(checks, "the rail draws the list the form's diagnostics became");

        IsResolved = estimate is not null;

        // Read off the field rather than held, so a label the copy changes and a figure the
        // reader types both reach this panel through the one control that owns them.
        HasUplink = uplink is not null;
        UplinkLabel = uplink?.Label ?? "";
        UplinkFigure = uplink is null
            ? ""
            : string.Join(' ', new[] { uplink.Readback, uplink.Unit }.Where(part => part.Length > 0));
        UplinkHint = uplink is null || editedOn is null
            ? ""
            : $"Change it or measure the line in step {editedOn.Number} · {editedOn.Label}.";

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

        Reconcile.Onto(Metrics, Rows(estimate));
        Reconcile.Onto(Checks, checks);
        ChecksSummary = PreflightChecks.SummaryOf(checks);

        Assert.That(FillShare is >= 0 and <= 1, "the bar's fill is a share of it", FillShare);
        Assert.That(UplinkShare is >= 0 and <= 1, "the uplink marker stands on the bar", UplinkShare);
        Assert.That(IsResolved || Metrics.Count == 0, "nothing is priced before a form arrives", Metrics.Count);
        Assert.That(ChecksSummary.Length > 0, "the terminal chip is told how much is owed", ChecksSummary);
        Assert.That(HasUplink || UplinkLabel.Length == 0, "an uplink reading names a field the form carries", UplinkLabel);
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
