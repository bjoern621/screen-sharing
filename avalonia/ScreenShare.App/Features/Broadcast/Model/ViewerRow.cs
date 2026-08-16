using System.Globalization;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;

namespace ScreenShare.App.Features.Broadcast.Model;

/// <summary>
/// One viewer of this stream, as the relay reports it.
///
/// <b>Filled from the relay's reader roster, cell for cell.</b> The relay names each reader by the protocol it
/// watches over and joins it to that protocol's connection list, so a row is an address, a join time and whatever
/// the leg is instrumented for.
/// SRT is the one leg the relay times a round trip and states a loss rate on; the rest report what was sent to
/// them and what the relay's own queue threw away.
/// A cell with no measurement behind it prints <see cref="Figure.NoValue"/> and never a zero, so a viewer on an
/// untimed leg never reads as a perfect one.
///
/// <b>Two of the design's columns name figures the relay does not have, and carry what it does measure
/// instead.</b> Buffer fill and the decoder in use are the viewer's own facts and reach no publisher, so a column
/// for either prints an ellipsis in every row forever.
/// The fifth carries what was discarded on the way out, the sixth the leg it went out over
/// (<c>docs/field-availability.md</c>, "A figure with no measurement").
///
/// <see cref="IsLast"/> is nobody's measurement: the render function derives it, the last row sitting flush
/// against the card's rounded edge and carrying no separator.
/// </summary>
/// <param name="Name">Address the relay saw, or its handle on the connection.</param>
/// <param name="Joined">When the relay accepted this reader, on the reading machine's clock.</param>
/// <param name="Rtt">Round trip in ms, absent on every leg but SRT.</param>
/// <param name="Loss">Send-side loss in percent, absent on every leg but SRT.</param>
/// <param name="Dropped">What went out incomplete: packets given up on, or frames the relay's queue discarded.</param>
/// <param name="Via">Leg this viewer watches over, in the transport vocabulary.</param>
/// <param name="IsStruggling">Whether a measured figure reached a limit in <see cref="Strain"/>.</param>
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
    /// Percent. Loss at or above this reads as a viewer in trouble.
    /// SRT states resent data against sent data, so the figure is already a rate, and a link losing a fiftieth of
    /// what is sent to it is one the retransmits are working to hide.
    /// </summary>
    private const double StrainingLossPercent = 2;

    /// <summary>
    /// ms. Round trip at or above this reads the same way.
    /// Distance alone is not trouble, but SRT's retransmit window is a small multiple of the round trip, and past
    /// this the relay resends into a gap that has already played.
    /// </summary>
    private const double StrainingRttMs = 200;

    /// <summary>
    /// One discard, the whole threshold.
    /// A packet the sender gave up on and a frame the relay's queue threw away are both data this viewer was never
    /// sent, and either is a picture that did not arrive.
    /// </summary>
    private const double StrainingDiscards = 1;

    /// <summary>
    /// The whole severity rule: every figure that can put a row in the struggling state, and the measurement at
    /// which it does.
    ///
    /// A table rather than a condition in the render function or the view, on the rule every static fact here
    /// follows: the escalation lands in four places in the markup, fill, weight, the hot roles and the one red,
    /// and a rule restated at each is four rules that can disagree (<c>docs/development-principles.md</c>,
    /// "Stateless").
    /// The view asks this row whether it is struggling and asks nothing else.
    ///
    /// A figure the leg does not report takes part in nothing, hence each entry reading presence rather than
    /// value: a viewer over RTMP is untimed and not calm, and an absent round trip read as a zero certifies every
    /// one of them.
    /// </summary>
    private static readonly (Func<RelayReader, double?> Figure, double Limit)[] Strain =
    [
        (reader => reader.HasLossPercent ? reader.LossPercent : null, StrainingLossPercent),
        (reader => reader.HasRttMs ? reader.RttMs : null, StrainingRttMs),
        (Discards, StrainingDiscards),
    ];

    public bool IsLast { get; init; }

    /// <summary>
    /// One row from one reader on the roster.
    /// Nothing here is decided: a cell is the relay's figure for that reader, formatted, or the ellipsis saying
    /// the leg states none.
    /// </summary>
    /// <remarks>
    /// The two identifying cells fall back and then give up rather than asserting, and do it in
    /// <see cref="Readers"/>, the session log naming the same readers (<c>docs/development-principles.md</c>,
    /// "Contracts").
    /// </remarks>
    public static ViewerRow Of(RelayReader reader)
    {
        Assert.NotNull(reader, "a viewer row describes a reader the relay reported");

        return new ViewerRow(
            Name: Readers.NameOf(reader),
            Joined: JoinedAt(reader),
            Rtt: Figure.Of(reader.HasRttMs ? reader.RttMs : null, "0"),
            Loss: Figure.Of(reader.HasLossPercent ? reader.LossPercent : null, "0.00"),
            Dropped: Figure.Of(Discards(reader), "0"),
            Via: reader.Transport.Length > 0 ? reader.Transport : Figure.NoValue,
            IsStruggling: Strain.Any(rule => rule.Figure(reader) is { } measured && measured >= rule.Limit));
    }

    /// <summary>
    /// What this viewer was not sent: packets the sender gave up on, plus frames the relay's outgoing queue
    /// discarded.
    /// Summed rather than shown apart, the column asking one question, did anything fail to go out, and a leg
    /// reporting at most one of the two.
    /// Absent where the leg counts neither, which is not a leg counting zero.
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
    /// When the relay accepted this reader, as a wall clock time: <c>14:07</c>.
    /// An age reads better and is not used: this runs on every render pass, and an age would come from a clock
    /// nobody handed it, so one roster would render differently on two consecutive passes and repaint under the
    /// reader's pointer.
    /// The relay's stamp is its own clock's, so this converts it to the local zone and drops the date.
    /// </summary>
    private static string JoinedAt(RelayReader reader)
        => Readers.JoinedAt(reader) is { } joined
            ? joined.ToLocalTime().ToString("HH:mm", CultureInfo.CurrentCulture)
            : Figure.NoValue;
}
