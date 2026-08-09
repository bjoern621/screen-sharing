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
    /// What the broadcast preview costs, and it is stated on the card rather than left to be
    /// discovered.
    ///
    /// <b>The preview is a viewer of this machine's own stream.</b> It is not a local mirror
    /// of the encoder's input: the frames go out to the relay and come back over a watch leg,
    /// which is what makes the card answer "what are my viewers actually seeing", degradation
    /// included (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws"). The
    /// two costs of that are real and they are the reader's to know - the round trip shows as
    /// lag, and the leg spends downstream bandwidth on this machine for as long as it runs.
    ///
    /// The third sentence is the one that would otherwise be a surprise. The preview never
    /// closes the decode it opened, because a decode is keyed by the stream and the leg alone
    /// and a tile on the viewer screen watching the same pair is watching the same decode:
    /// closing it here would close it there. So the bandwidth is spent until the stream ends.
    /// </summary>
    /// <param name="leg">What the watch leg is called, empty before the settings have been
    /// resolved once - in which case the sentence names the cost without naming the protocol.</param>
    public static string PreviewCost(string leg) => leg.Length == 0
        ? "This is the stream received back from the relay, exactly as a viewer receives it: "
          + "it lags by the watch leg's own latency and it spends downstream bandwidth. The "
          + "decode stays open until the stream ends, because closing it would close it for a "
          + "tile watching the same stream."
        : $"This is the stream received back from the relay over {leg}, exactly as a viewer "
          + "receives it: it lags by that leg's own latency and it spends downstream bandwidth. "
          + "The decode stays open until the stream ends, because closing it would close it for "
          + "a tile watching the same stream.";

    /// <summary>Nothing is on the air, so there is no stream to ask the relay for.</summary>
    public const string PreviewNotPublishing = "Nothing is publishing, so there is nothing to receive back.";

    /// <summary>
    /// The settings have not been resolved yet, so no leg has been named. The preview picks
    /// no protocol of its own (<c>Features/Viewer/Tile/Model/TileLeg.cs</c>), so this is what
    /// it says instead of guessing one.
    /// </summary>
    public const string PreviewNoLeg = "The settings have not said which protocol a tile receives on yet.";

    /// <summary>
    /// The relay's snapshot carries no path by this stream's name. A stream that has just
    /// started publishes before the relay's next poll sees it, so this is the ordinary state
    /// of the first seconds of a broadcast rather than a failure.
    /// </summary>
    public const string PreviewRelayHasNoPath = "The relay is not carrying this stream yet.";

    /// <summary>A decode has been asked for and the backend has not answered yet.</summary>
    public const string PreviewOpening = "Asking the relay for this stream back.";

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
}
