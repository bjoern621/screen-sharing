using System.Collections.ObjectModel;
using System.Globalization;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Features.Fields.ViewModel;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Setup.CostRail.ViewModel;

/// <summary>
/// What the draft costs, and what stands between it and going live.
/// Beside the form rather than after it, so a choice is priced while it is being made.
///
/// <b>Every figure is the backend's.</b> The rate, the raw rate and the headroom come off
/// <c>Summary.estimate</c>, and the limit the bar is measured against is the same form's
/// <c>publish.uplink_mbps</c> field.
/// Nothing here predicts anything (<c>docs/ipc-api.md</c>, "The rule").
///
/// <b>Outputs only, the uplink included.</b> The step that owns the field draws the control for it, so this
/// panel reads the figure and names that step rather than being a second place one setting is edited.
/// </summary>
public sealed class CostRailViewModel : Observable
{
    /// <summary>
    /// Slack the bar's scale leaves above the larger of prediction and uplink,
    /// so a prediction that exactly meets the line keeps the marker off the end cap.
    /// </summary>
    private const double Headroom = 1.15;

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

    /// <summary>The estimate's figures under the headline, rebuilt from it on every pass.</summary>
    public ObservableCollection<CostMetricRow> Metrics { get; }

    /// <summary>The form's diagnostics as lines, in the order the form ranked them.</summary>
    public ObservableCollection<PreflightCheckRow> Checks { get; }

    /// <summary>Headline figure, Mb/s predicted.</summary>
    public string Bitrate { get => _bitrate; private set => Set(ref _bitrate, value); }

    /// <summary>Unit and what reads beside it: "Mb/s predicted · 1619 raw · 137:1".</summary>
    public string BitrateCaption { get => _bitrateCaption; private set => Set(ref _bitrateCaption, value); }

    /// <summary>What the marker stands at: "uplink 20 Mb/s", or the sentence for an uplink nobody stated.</summary>
    public string UplinkCaption { get => _uplinkCaption; private set => Set(ref _uplinkCaption, value); }

    /// <summary>
    /// Read off the uplink field.
    /// Empty where the form carries no such field, rather than a heading over nothing.
    /// </summary>
    public string UplinkLabel { get => _uplinkLabel; private set => Set(ref _uplinkLabel, value); }

    /// <summary>
    /// Stated uplink as the field reads it back, unit included: "20 Mb/s".
    /// A reading and not a control: the owning step already draws one for this setting.
    /// </summary>
    public string UplinkFigure { get => _uplinkFigure; private set => Set(ref _uplinkFigure, value); }

    /// <summary>
    /// Where the figure is changed and measured, named after the step owning the control.
    /// Empty where no step of this flow draws it.
    /// </summary>
    public string UplinkHint { get => _uplinkHint; private set => Set(ref _uplinkHint, value); }

    public bool HasUplink { get => _hasUplink; private set => Set(ref _hasUplink, value); }

    /// <summary>Share of the bar the prediction fills, 0..1.</summary>
    public double FillShare { get => _fillShare; private set => Set(ref _fillShare, value); }

    /// <summary>
    /// Share of the bar the uplink marker stands at, 0..1.
    /// Zero where no uplink was stated, which draws no marker.
    /// </summary>
    public double UplinkShare { get => _uplinkShare; private set => Set(ref _uplinkShare, value); }

    /// <summary>Prediction past the marker, the fault the bar exists to show.</summary>
    public bool IsOverUplink { get => _isOverUplink; private set => Set(ref _isOverUplink, value); }

    /// <summary>False before the first form lands, when there is nothing to price.</summary>
    public bool IsResolved { get => _isResolved; private set => Set(ref _isResolved, value); }

    /// <summary>
    /// The one line the terminal chip says about this list.
    /// Derived here rather than restated on the chip, so the strip and the rail cannot disagree about how much
    /// is owed.
    /// </summary>
    public string ChecksSummary { get => _checksSummary; private set => Set(ref _checksSummary, value); }

    /// <summary>
    /// The one render function.
    /// Idempotent: every output is read out of the arguments, and both lists hold records reconciled onto,
    /// so a second pass over one form notifies nothing.
    /// </summary>
    /// <param name="estimate">The backend's prediction for the draft, null before the first form.</param>
    /// <param name="uplink">The uplink field, null where the form carries none.</param>
    /// <param name="editedOn">The step drawing that field, null where no step of this flow does.</param>
    /// <param name="checks">The form's diagnostics, already ranked.</param>
    public void Apply(
        Estimate? estimate,
        FieldViewModel? uplink,
        SetupStepRow? editedOn,
        IReadOnlyList<PreflightCheckRow> checks)
    {
        Assert.NotNull(checks, "the rail draws the list the form's diagnostics became");

        IsResolved = estimate is not null;

        // Off the field rather than held, so a reworded label and a typed figure both reach the panel through
        // the one control that owns them.
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

        // Stated uplink recovered from the estimate rather than read off the settings field,
        // so both figures on the bar come out of the arithmetic that produced the headroom.
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
    /// The estimate's other figures, Mb/s: what the capture produces before coding, and what the line has left
    /// with the stream on it.
    /// A negative headroom is not clamped: it is the number saying the line cannot carry this, and the words
    /// for it are already in the checks list.
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
    /// The headline's unit, the uncompressed rate, and the ratio between the two,
    /// which is what makes the prediction legible as a compression rather than as a number.
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
