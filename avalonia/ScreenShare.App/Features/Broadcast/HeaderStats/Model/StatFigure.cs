namespace ScreenShare.App.Features.Broadcast.HeaderStats.Model;

/// <summary>
/// One promoted number and the unit hanging off it. A 26px figure, a 13px unit, baseline
/// aligned - the split is what lets the number be read from across a room while the unit
/// stays out of the way.
///
/// A record, so a pass that measured the same throughput twice compares equal and the
/// header does not repaint.
/// </summary>
public sealed record StatFigure(string Value, string Unit);
