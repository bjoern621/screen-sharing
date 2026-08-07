namespace ScreenShare.App.Features.Setup.CostRail.ViewModel;

/// <summary>
/// One priced dimension of the current choice. The rail carries three - bitrate, latency
/// and GPU load - because those are the three things a quality setting spends, and a form
/// that shows only the first teaches the reader that the other two are free.
///
/// A record, so an unchanged pass leaves the bound list alone.
/// </summary>
public sealed record CostMetricRow
{
    public required string Label { get; init; }

    /// <summary>The figure, approximate where the estimate is.</summary>
    public required string Value { get; init; }
}
