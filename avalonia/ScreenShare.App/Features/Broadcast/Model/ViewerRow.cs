using System.Globalization;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// One viewer of this stream, as the relay reports it. This row is what turns "it's laggy for
/// me" into something a publisher can point at.
///
/// <b>It is filled from the relay's reader roster, and every cell is a figure that roster
/// carried.</b> The relay names each reader by the protocol it is watching over and joins it to
/// that protocol's own connection list, so a row is an address, a join time, and whatever the
/// leg is instrumented for. SRT is the one leg the relay times a round trip and states a loss
/// rate on; the rest report what was sent to them and what the relay's own queue had to throw
/// away. A cell with no measurement behind it prints <see cref="Figure.NoValue"/> and never a
/// zero, so a viewer on a leg nobody times reads as untimed rather than as perfect.
///
/// <b>Two of the design's columns name figures the relay does not have, and they were replaced
/// rather than left empty.</b> Buffer fill and the decoder in use are the viewer's own facts and
/// nothing reports them to the publisher; a column that printed an ellipsis in every row forever
/// would be a permanent hole where the relay does in fact measure something. So the fifth column
/// carries what was discarded on the way out and the sixth the leg it went out over
/// (<c>docs/field-availability.md</c>, "A figure with no measurement").
///
/// <see cref="IsLast"/> is not reported by anyone - it is derived by the render function, because
/// the last row sits flush against the card's rounded edge and carries no separator.
/// </summary>
/// <param name="Name">Who the reader is: the address the relay saw, or its handle on the connection.</param>
/// <param name="Joined">When the relay accepted this reader, on the clock of whoever is reading the screen.</param>
/// <param name="Rtt">Round trip in milliseconds, absent on every leg but SRT.</param>
/// <param name="Loss">Send-side loss as a percentage, absent on every leg but SRT.</param>
/// <param name="Dropped">What went out incomplete: packets given up on, or frames the relay's queue discarded.</param>
/// <param name="Via">The leg this viewer is watching over, in the transport vocabulary.</param>
/// <param name="IsStruggling">Whether a measured figure crossed a limit in <see cref="Strain"/>.</param>
public sealed record ViewerRow(
    string Name,
    string Joined,
    string Rtt,
    string Loss,
    string Dropped,
    string Via,
    bool IsStruggling)
{
    /// <summary>
    /// Loss at or above this reads as a viewer in trouble. SRT states this figure as resent data
    /// against sent data, so it is already a rate rather than a count, and a link losing a
    /// fiftieth of what is sent to it is one the retransmits are working to hide.
    /// </summary>
    private const double StrainingLossPercent = 2;

    /// <summary>
    /// Round trip at or above this reads the same way. It is not slowness in itself - a viewer on
    /// another continent is far away rather than in trouble - but SRT's retransmit window is a
    /// small multiple of the round trip, and past this the relay is resending into a gap that has
    /// already played.
    /// </summary>
    private const double StrainingRttMs = 200;

    /// <summary>
    /// Any discard at all. Unlike the two above this needs no threshold: a packet the sender gave
    /// up on and a frame the relay's queue threw away are both data this viewer was never sent,
    /// and one of them is already a picture that did not arrive.
    /// </summary>
    private const double StrainingDiscards = 1;

    /// <summary>
    /// The whole severity rule, stated once. Every figure that can put a row in the struggling
    /// state, and the measurement at which it does.
    ///
    /// It is a table and not a condition spread through the render function or the view, for the
    /// reason every other static fact in this codebase is one: the escalation shows up in four
    /// places in the markup - fill, weight, the hot roles and the one red - and a rule restated at
    /// each of them is four rules that can disagree (<c>docs/development-principles.md</c>,
    /// "Stateless"). The view asks this row whether it is struggling and nothing else.
    ///
    /// A figure the leg does not report takes part in nothing. That is the point of reading
    /// presence rather than value: a viewer over RTMP is not calm, it is untimed, and a rule that
    /// read its absent round trip as a zero would quietly certify every one of them.
    /// </summary>
    private static readonly (Func<RelayReader, double?> Figure, double Limit)[] Strain =
    [
        (reader => reader.HasLossPercent ? reader.LossPercent : null, StrainingLossPercent),
        (reader => reader.HasRttMs ? reader.RttMs : null, StrainingRttMs),
        (Discards, StrainingDiscards),
    ];

    public bool IsLast { get; init; }

    /// <summary>
    /// One row from one reader on the roster. Nothing here decides anything about the stream:
    /// each cell is the figure the relay stated for that reader, formatted, or the ellipsis that
    /// says the leg states none.
    /// </summary>
    /// <remarks>
    /// The two identifying cells fall back and then give up rather than asserting. They are the
    /// relay's own words, and a relay that named a reader nothing is an environment condition
    /// this screen has to survive - so an unnameable viewer renders as one, which is still a row
    /// saying somebody is connected (<c>docs/development-principles.md</c>, "Contracts").
    /// </remarks>
    public static ViewerRow Of(RelayReader reader)
    {
        Assert.NotNull(reader, "a viewer row describes a reader the relay reported");

        var name = reader.HasRemoteAddr && reader.RemoteAddr.Length > 0 ? reader.RemoteAddr : reader.Id;

        return new ViewerRow(
            Name: name.Length > 0 ? name : Figure.NoValue,
            Joined: JoinedAt(reader),
            Rtt: Figure.Of(reader.HasRttMs ? reader.RttMs : null, "0"),
            Loss: Figure.Of(reader.HasLossPercent ? reader.LossPercent : null, "0.00"),
            Dropped: Figure.Of(Discards(reader), "0"),
            Via: reader.Transport.Length > 0 ? reader.Transport : Figure.NoValue,
            IsStruggling: Strain.Any(rule => rule.Figure(reader) is { } measured && measured >= rule.Limit));
    }

    /// <summary>
    /// What this viewer was not sent: packets the sender gave up on, plus frames the relay's own
    /// outgoing queue discarded. They are summed rather than shown apart because the column asks
    /// one question - did anything fail to go out - and a leg reports at most one of the two.
    /// Absent where the leg counts neither, which is not the same as a leg that counts zero.
    /// </summary>
    private static double? Discards(RelayReader reader)
    {
        if (!reader.HasPacketsDropped && !reader.HasFramesDiscarded)
        {
            return null;
        }

        return (reader.HasPacketsDropped ? reader.PacketsDropped : 0)
             + (reader.HasFramesDiscarded ? reader.FramesDiscarded : 0);
    }

    /// <summary>
    /// When the relay accepted this reader, as a wall clock time rather than an age.
    ///
    /// An age would read better and is not written here: this is a render function, it runs
    /// whenever anything on the screen moves, and an age would have to come from a clock nobody
    /// handed it - which would make the same roster render differently on two consecutive passes
    /// and put a repaint under the reader's pointer every time. The relay's timestamp is its own
    /// clock's, so this only converts it to the reader's zone and drops the date.
    /// </summary>
    private static string JoinedAt(RelayReader reader)
    {
        if (!reader.HasJoined || !DateTimeOffset.TryParse(
                reader.Joined, CultureInfo.InvariantCulture, DateTimeStyles.RoundtripKind, out var joined))
        {
            return Figure.NoValue;
        }

        return joined.ToLocalTime().ToString("HH:mm", CultureInfo.CurrentCulture);
    }
}
