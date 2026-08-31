using ScreenShare.Api.V1;
using ScreenShare.App.Copy;

namespace ScreenShare.App.Features.Broadcast.TestStreams.Model;

/// <summary>
/// One slot of the synthetic set, as the card prints it.
///
/// The slot is the identity and the child is not, so a relaunch comes back as the row that was already there and
/// the count beside it moves rather than the list.
///
/// Record, so a pass over an unchanged answer compares equal and the bound list is left where it is.
/// </summary>
/// <param name="Label">Which slot, and the relay path it publishes to.</param>
/// <param name="IsRunning">A child is filling the slot, false while a relaunch waits out its backoff.</param>
/// <param name="Attempt">Which relaunch the slot is on, counting from one.</param>
/// <param name="Cause">Why the slot carries no publisher, empty while one is running.</param>
/// <param name="Message">
/// The child's own last words, raw and never matched against (<c>api/proto/screenshare/v1/text.proto</c>).
/// </param>
/// <param name="LogPath">Whole run log on disk, as the backend named it.</param>
/// <param name="IsLast">
/// Ends the set, so the row carries no separator under it and sits flush against the card's edge.
/// Derived by the render pass: which row ends a list is not a fact the backend has.
/// </param>
public sealed record TestStreamSlotRow(
    string Label,
    bool IsRunning,
    string Attempt,
    string Cause,
    string Message,
    string LogPath,
    bool IsLast = false)
{
    public bool HasMessage => Message.Length > 0;

    public bool HasLogPath => LogPath.Length > 0;

    public bool HasCause => Cause.Length > 0;

    /// <summary>What the state column says: sending, or a slot the set holds with nothing in it.</summary>
    public string State => IsRunning ? Cards.TestStreamSending : Cards.TestStreamStopped;

    /// <summary>
    /// One slot read into a row.
    /// A running slot has nothing to answer for, so the cause and the last words are dropped rather than carried
    /// from the attempt before it.
    /// </summary>
    public static TestStreamSlotRow Of(TestStreamSlot slot) => new(
        Cards.TestStreamSlotLabel(slot.Slot, slot.Name),
        slot.Running,
        Cards.TestStreamAttempt(slot.Attempt),
        slot.Running ? "" : Statements.Of(slot.Cause),
        slot.Running ? "" : slot.Message,
        slot.LogPath);
}
