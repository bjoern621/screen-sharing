namespace ScreenShare.App.Features.Setup.CostRail.ViewModel;

/// <summary>
/// One figure of the estimate, under the headline rate.
/// A record, so a pass over an unchanged estimate leaves the bound list alone.
/// </summary>
public sealed record CostMetricRow
{
    public required string Label { get; init; }

    /// <summary>
    /// Formatted, unit included: "1619 Mb/s".
    /// A prediction, never a measurement.
    /// </summary>
    public required string Value { get; init; }
}
