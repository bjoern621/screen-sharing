namespace ScreenShare.App.Features.Broadcast.HeaderStats.Model;

/// <summary>
/// One promoted number and the unit hanging off it.
/// A 26px figure, a 13px unit, baseline aligned - the split is what lets the number be read from across a
/// room while the unit stays out of the way.
///
/// A record, so a pass that measured the same throughput twice compares equal and the header does not
/// repaint.
/// </summary>
/// <param name="Note">
/// Why this figure reads as unmeasured, and null where it is measured or where its absence explains itself.
/// A promoted number has no room for a clause beside it, so the note is a tooltip, which is where this design
/// puts the sentence a greyed or absent thing needs (<c>docs/field-availability.md</c>, "The rule").
/// Null rather than empty: an empty tip is a tooltip that opens on nothing.
/// </param>
public sealed record StatFigure(string Value, string Unit, string? Note = null);
