namespace ScreenShare.App.Features.Broadcast.HeaderStats.Model;

/// <summary>
/// One promoted number and its unit, split so each takes its own size.
/// Record: two passes over one reading compare equal, so the header repaints nothing.
/// </summary>
/// <param name="Note">
/// Why the figure reads as unmeasured.
/// Null where it is measured, and where its absence already explains itself.
/// Drawn as a tooltip, since a promoted number has no room for a clause beside it
/// (<c>docs/field-availability.md</c>, "The rule").
/// Null rather than empty: an empty tip opens on nothing.
/// </param>
public sealed record StatFigure(string Value, string Unit, string? Note = null);
