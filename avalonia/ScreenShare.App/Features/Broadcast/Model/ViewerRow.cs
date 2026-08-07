namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// One viewer of this stream, as the relay would report it. This row is what turns "it's laggy
/// for me" into something a publisher can point at.
///
/// <b>Nothing fills it today, and the table says so instead of standing empty.</b> The relay
/// snapshot on the contract carries a reader <i>count</i> per path and no reader identities,
/// and nothing anywhere measures a viewer's round trip, loss or buffer fill. Composing rows
/// from the count would be inventing viewers; dropping the table would hide that the
/// measurement is missing rather than that there is nobody watching. So the table renders the
/// count it does have and states what it cannot yet name
/// (<c>avalonia/README.md</c>, "What is not backed yet").
///
/// <see cref="IsStruggling"/> is the relay's verdict and drives the whole escalation when
/// there is one: a filled row, a bold name, and the one red in the table on the metric that is
/// actually out of bounds. <see cref="IsLast"/> is not reported by anyone - it is derived by
/// the render function, because the last row sits flush against the card's rounded edge and
/// carries no separator.
/// </summary>
public sealed record ViewerRow(
    string Name,
    string Joined,
    string Rtt,
    string Loss,
    string Buffer,
    string Decoder,
    bool IsStruggling)
{
    public bool IsLast { get; init; }
}
