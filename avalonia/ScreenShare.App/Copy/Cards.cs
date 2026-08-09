namespace ScreenShare.App.Copy;

/// <summary>
/// What a card says in its own right: the sentence naming what it is, and the sentences it
/// shows in place of the thing it usually draws.
///
/// The other files here key their wording on something the backend sent - an identifier
/// (<see cref="Words"/>), a choice (<see cref="Descriptions"/>), a field (<see cref="Fields"/>),
/// a statement (<see cref="Statements"/>). These are keyed on nothing: they are what one card
/// says about itself, and the backend has no opinion about any of it. They live here all the
/// same, because every word on screen is this module's and belongs where the words are
/// (<c>avalonia/README.md</c>, "Layout") - not in the markup, where a sentence cannot be read
/// beside the ones it has to agree with, and not spelled twice at two render functions.
///
/// A card's own sentence is not a substitute for the backend's. Where a call was refused, the
/// refusal is shown as the backend wrote it (<c>docs/ipc-api.md</c>, "Errors"); what is here
/// covers the states no call failed in.
/// </summary>
public static class Cards
{
    /// <summary>
    /// What the broadcast preview is and is not, stated on the card rather than left to be
    /// discovered.
    ///
    /// <b>It used to say the opposite, and the facts moved under it.</b> The preview was a
    /// viewer of this machine's own stream, received back off the relay, so the sentence here
    /// named a round trip, downstream bandwidth, and a decode that stayed open because a tile
    /// on the viewer screen might be sharing it. None of those is true any more: the publish
    /// child copies its already-encoded video to a loopback port, the backend decodes that, and
    /// the relay never sees the preview at all
    /// (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws").
    ///
    /// <b>What replaces them is one gain and one warning.</b> The gain is that it costs a local
    /// decode and no bandwidth, and that the relay counts no reader for it - the viewer figures
    /// beside this card are viewers, with nothing of this machine's own in them. The warning is
    /// the half a reader must not discover the hard way: the picture is taken <i>before</i> the
    /// relay, so it says what is being sent and nothing about what anybody receives. A
    /// congested uplink, a relay dropping packets and a viewer on a bad link all leave this
    /// card looking perfect, and what answers them is the viewer table and the round-trip plot.
    /// </summary>
    public const string PreviewCost =
        "This is what is being sent, decoded on this machine from a copy the encoder makes: it "
        + "never reaches the relay, costs one local decode and no bandwidth, and adds no viewer "
        + "to the counts beside it. It shows nothing about what viewers receive - for that, read "
        + "the viewer table and the round-trip plot.";

    /// <summary>Nothing is on the air, so there is nothing being sent to show.</summary>
    public const string PreviewNotPublishing = "Nothing is publishing, so there is nothing being sent to show.";

    /// <summary>
    /// A stream is on the air and the backend is running no preview of it. The preview goes up
    /// with the publish child, so this is the backend saying it could not: a format with no
    /// local carriage, or a pipeline that would not start. The reason is in the backend's log
    /// rather than on the contract, which is why this sentence names the fact and not a cause.
    /// </summary>
    public const string PreviewNotPreviewed = "This stream is on the air and is not being previewed locally.";

    /// <summary>
    /// The caveat beside the nudge card's title, and it is the opposite of the permission slip
    /// that used to stand there.
    ///
    /// The caption read "applies without a reconnect" while the control below it was inert for
    /// precisely the reason that sentence denied. Two files stating opposite things about one
    /// slider is the case a copy table exists to make impossible: this sentence and
    /// <see cref="NudgeInert"/> are read side by side here, and neither can drift without the
    /// other being in view.
    /// </summary>
    public const string NudgeCaveat = "no live-safe quality change exists";

    /// <summary>
    /// Why the nudge track takes no hand, shown on the card rather than only in a comment.
    ///
    /// Both engines run a child process built from an argv and neither takes a value back
    /// afterwards, so there is no effect that changes an encoder's quality without rebuilding
    /// the pipeline. <c>ApplyToStream</c> is reachable from setup now and it restarts the
    /// stream, which is the honest thing to send a reader to - a slider wired to it would be a
    /// control whose whole promise is false (<c>docs/field-availability.md</c>, "The rule").
    /// </summary>
    public const string NudgeInert =
        "Both engines run a child built from an argv and neither takes a new quality back, so "
        + "changing it restarts the stream. Setup applies it, and says so before it does.";

    /// <summary>
    /// Why the configuration card is read-only, and what a change to it now costs.
    ///
    /// It used to say quality and latency did not need a restart, which was a statement about a
    /// live-safe apply that has never existed. Every setting on this card reaches a running
    /// pipeline the one way there is - setup's commit, which tears the encoder down and launches
    /// it again on the new settings.
    /// </summary>
    public const string ConfigReadOnly =
        "Read-only while live. Every setting here reaches a running stream by restarting it, "
        + "which is what setup's commit does.";

    /// <summary>
    /// What the configuration card says before its rows arrive.
    ///
    /// The obvious sentence for an empty card - nothing is publishing - is the one state it can
    /// never be in, because the destination it sits on is unreachable unless a stream is live.
    /// What it is really showing is a resolve that has not answered yet, and saying so is the
    /// difference between a card that looks broken and one that looks busy.
    /// </summary>
    public const string ConfigUndescribed = "Reading what the running stream was built from.";
}
