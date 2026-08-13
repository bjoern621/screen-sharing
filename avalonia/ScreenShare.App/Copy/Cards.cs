namespace ScreenShare.App.Copy;

/// <summary>
/// What a card says in its own right: the sentence naming what it is, and the sentences it shows in place of
/// the thing it usually draws.
///
/// The other files here key their wording on something the backend sent - an identifier
/// (<see cref="Words"/>), a choice (<see cref="Descriptions"/>), a field (<see cref="Fields"/>), a statement
/// (<see cref="Statements"/>).
/// These are keyed on nothing: they are what one card says about itself, and the backend has no opinion about
/// any of it.
/// They live here all the same, because every word on screen is this module's and belongs where the words are
/// (<c>avalonia/README.md</c>, "Layout") - not in the markup, where a sentence cannot be read beside the ones
/// it has to agree with, and not spelled twice at two render functions.
///
/// A card's own sentence is not a substitute for the backend's.
/// Where a call was refused, the refusal is shown as the backend wrote it (<c>docs/ipc-api.md</c>, "Errors");
/// what is here covers the states no call failed in.
/// </summary>
public static class Cards
{
    /// <summary>
    /// What the preview's route toggle chooses, said above the control rather than left to be worked out from
    /// the two labels.
    ///
    /// It names where the picture is taken, because that is the whole of the difference: both routes carry
    /// the same encode, and everything one shows and the other cannot is downstream of it
    /// (<see cref="Features.Broadcast.Preview.Model.PreviewRoute"/>).
    /// </summary>
    public const string PreviewRouteChoice = "Where this picture is taken:";

    /// <summary>What the local route's segment says.</summary>
    public const string PreviewLocalLabel = "Local";

    /// <summary>
    /// What the end-to-end route's segment says.
    /// Spelled out rather than abbreviated: "E2E" is a word for the people who built the pipeline, and the
    /// card is read by the person sharing their screen.
    /// </summary>
    public const string PreviewEndToEndLabel = "End to end";

    /// <summary>
    /// What the local picture is and is not, stated on the card rather than left to be discovered.
    ///
    /// <b>It used to say the opposite, and the facts moved under it.</b> The preview was a viewer of this
    /// machine's own stream, received back off the relay, so the sentence here named a round trip, downstream
    /// bandwidth, and a decode that stayed open because a tile on the viewer screen might be sharing it.
    /// None of those is true of this route: the publish child copies its already-encoded video to a loopback
    /// port, the backend decodes that, and the relay never sees the picture at all
    /// (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws").
    ///
    /// <b>What replaces them is one gain and one warning.</b> The gain is that it costs a local decode and no
    /// bandwidth, and that the relay counts no reader for it - the viewer figures beside this card are
    /// viewers, with nothing of this machine's own in them.
    /// The warning is the half a reader must not discover the hard way: the picture is taken <i>before</i>
    /// the relay, so it says what is being sent and nothing about what anybody receives.
    /// A congested uplink, a relay dropping packets and a viewer on a bad link all leave this card looking
    /// perfect.
    ///
    /// It ends by naming the other route rather than the viewer table alone, because the other route is now
    /// the direct answer to what it cannot show.
    /// </summary>
    public const string PreviewLocalCost =
        "This is what is being sent, decoded on this machine from a copy the encoder makes: it "
        + "never reaches the relay, costs one local decode and no bandwidth, and adds no viewer "
        + "to the counts beside it. It shows nothing about what viewers receive - switch to end "
        + "to end for that, or read the viewer table and the round-trip plot.";

    /// <summary>
    /// What the end-to-end picture is and what it costs.
    ///
    /// <b>It is a viewer of this machine's own stream, and the card says so plainly.</b> The decode is opened
    /// with <c>StartReceive</c> like any tile in the grid, so the relay serves it a reader slot, counts it
    /// among the viewers reported beside this card, and sends it the stream at the same downstream cost any
    /// other viewer pays.
    /// That is the price of the picture being honest, and a reader who does not want to pay it has the other
    /// route.
    ///
    /// The figures beside the card are what the warning is about.
    /// A reader comparing the two routes and then reading a viewer count has to know that one of those
    /// viewers is this window.
    /// </summary>
    public const string PreviewEndToEndCost =
        "This is what a viewer receives: this machine's own stream, pulled back off the relay "
        + "over the leg the viewer screen receives on. It crosses the uplink, the relay and the "
        + "way back, so what it shows is the whole path - and it pays for that as a viewer does, "
        + "spending downstream bandwidth and counting as one of the viewers beside it.";

    /// <summary>Nothing is on the air, so there is nothing being sent to show.</summary>
    public const string PreviewNotPublishing = "Nothing is publishing, so there is nothing being sent to show.";

    /// <summary>
    /// What the screen picker's pictures are, said once above the grid.
    ///
    /// <b>The half worth stating is that nothing is being shared yet.</b> A live picture of a screen in an
    /// app whose whole purpose is sending one is exactly the thing a reader could misread, and the cost of
    /// that misreading is a person believing they are on the air when they are not, or the reverse.
    /// So the sentence leads with it.
    ///
    /// The second half is what the picture is for: it is the same rectangle the stream would carry, read by
    /// the same element, so what is in frame here is what viewers would get (<c>internal/screensrc</c>).
    /// </summary>
    /// <summary>
    /// A screen the backend has been asked to read and has not produced a picture from yet.
    /// It is a moment on a working machine and it is a state all the same: opening a screen means a capture
    /// element starting, and a tile with nothing in it and nothing to say reads as broken.
    /// </summary>
    public const string ScreenOpening = "Opening.";

    public const string ScreenPickerCost =
        "Nothing is being shared yet. Each picture is what that screen is showing now, read the "
        + "same way the stream would read it, so the one you pick is what viewers would see.";

    /// <summary>
    /// A stream is on the air and the backend is running no preview of it.
    /// The preview goes up with the publish child, so this is the backend saying it could not: a format with
    /// no local carriage, or a pipeline that would not start.
    /// The reason is in the backend's log rather than on the contract, which is why this sentence names the
    /// fact and not a cause.
    /// </summary>
    public const string PreviewNotPreviewed = "This stream is on the air and is not being previewed locally.";

    /// <summary>
    /// The end-to-end route has no leg to receive on, because no settings have arrived yet.
    ///
    /// It is the viewer's leg this route names and the same absence the viewer's grid reports, worded for
    /// this card: a decode is keyed by the stream and the protocol together, so there is nothing to ask for
    /// until the settings say which protocol (<c>Features/Viewer/Tile/Model/TileLeg.cs</c>).
    /// </summary>
    public const string PreviewNoWatchLeg =
        "The settings have not said which protocol a viewer receives on yet.";

    /// <summary>
    /// The end-to-end route has asked the backend for a decode of this stream and it is not running yet.
    ///
    /// It is its own state and not the tile's "Connecting.": that one is a pipeline that is up with no frame
    /// out of it, and this is the moment before there is a pipeline at all.
    /// A route that spends a round trip to the relay before it can draw anything is worth saying out loud,
    /// because the local route draws immediately and the difference reads as a stall.
    /// </summary>
    public const string PreviewOpening = "Opening a decode of this stream off the relay.";

    /// <summary>
    /// The caveat beside the nudge card's title, and it is the opposite of the permission slip that used to
    /// stand there.
    ///
    /// The caption read "applies without a reconnect" while the control below it was inert for precisely the
    /// reason that sentence denied.
    /// Two files stating opposite things about one slider is the case a copy table exists to make impossible:
    /// this sentence and <see cref="NudgeInert"/> are read side by side here, and neither can drift without
    /// the other being in view.
    /// </summary>
    public const string NudgeCaveat = "no live-safe quality change exists";

    /// <summary>
    /// Why the nudge track takes no hand, shown on the card rather than only in a comment.
    ///
    /// Both engines run a child process built from an argv and neither takes a value back afterwards, so
    /// there is no effect that changes an encoder's quality without rebuilding the pipeline.
    /// <c>ApplyToStream</c> is reachable from setup now and it restarts the stream, which is the honest thing
    /// to send a reader to - a slider wired to it would be a control whose whole promise is false
    /// (<c>docs/field-availability.md</c>, "The rule").
    /// </summary>
    public const string NudgeInert =
        "Both engines run a child built from an argv and neither takes a new quality back, so "
        + "changing it restarts the stream. Setup applies it, and says so before it does.";

    /// <summary>
    /// Why the configuration card is read-only, and what a change to it now costs.
    ///
    /// It used to say quality and latency did not need a restart, which was a statement about a live-safe
    /// apply that has never existed.
    /// Every setting on this card reaches a running pipeline the one way there is - setup's commit, which
    /// tears the encoder down and launches it again on the new settings.
    /// </summary>
    public const string ConfigReadOnly =
        "Read-only while live. Every setting here reaches a running stream by restarting it, "
        + "which is what setup's commit does.";

    /// <summary>
    /// What the configuration card says before its rows arrive.
    ///
    /// The obvious sentence for an empty card - nothing is publishing - is the one state it can never be in,
    /// because the destination it sits on is unreachable unless a stream is live.
    /// What it is really showing is a resolve that has not answered yet, and saying so is the difference
    /// between a card that looks broken and one that looks busy.
    /// </summary>
    public const string ConfigUndescribed = "Reading what the running stream was built from.";

    /// <summary>
    /// What a preset covers, said where the reader is about to make one.
    ///
    /// It is the card's one paragraph because the review beside it shows more than a preset holds: the relay
    /// tile is on that screen too, and a reader who saved "work" and found their relay host changed on
    /// another machine would have learnt the boundary the hard way.
    /// The reason is the boundary itself - the relay belongs to a deployment and the watch settings to this
    /// machine's drivers, so a preset carrying either would break exactly where it was meant to help
    /// (<c>docs/presets.md</c>).
    /// </summary>
    public const string PresetsCovers =
        "A preset is one way of publishing: the source, the quality, the audio and the transport. "
        + "The relay and the watch settings stay as they are, because they belong to where you "
        + "are rather than to what you send.";

    /// <summary>
    /// The store holds nothing.
    /// Said as a state rather than as a hint, and separate from <see cref="Statements"/>' unreadable-store
    /// notice: that one is the store failing, and this one is a reader who has not saved anything yet.
    /// </summary>
    public const string PresetsEmpty = "Nothing saved yet. Name the configuration below to keep it.";

    /// <summary>
    /// Why the list can be behind, on the button that answers it.
    ///
    /// Presets are a file the backend does not run on, so no event says one appeared - a second window's save
    /// is invisible here until this asks again (<c>Backend/IBackend.cs</c>, <c>PresetsAsync</c>).
    /// The re-read is what the contract's own gap leaves the reader, and a tooltip is where that belongs
    /// rather than a paragraph on the card.
    /// </summary>
    public const string PresetsReread = "Read the saved presets again. Nothing announces a preset another window saved.";

    /// <summary>
    /// Why a stream that is being watched has no round trip and no loss to state: the legs its viewers are
    /// actually on, beside the one leg the relay times.
    ///
    /// The legs are named from the roster rather than left out, so the sentence describes this stream and
    /// leaves the reader something to change.
    /// "No viewer is on a leg the relay times" is true and says neither which leg they are on nor which one
    /// would answer, so it is what a roster this shell can read no leg off falls back to.
    ///
    /// That SRT is the leg that answers is the contract's own statement about what a relay measures, not a
    /// rule invented at a screen (<c>api/proto/screenshare/v1/session.proto</c>, <c>RelayReader</c>).
    /// Two surfaces show this one sentence - the latency plot in place of its curves, and the header stat bar
    /// on the two figures it is the reason for - and it is written here once so they cannot come to say
    /// different things about the same roster.
    /// </summary>
    /// <param name="legs">
    /// <see cref="Features.Broadcast.Model.BroadcastSnapshot.Legs"/>: the legs the stream is watched over,
    /// empty where none is named.
    /// </param>
    public static string Untimed(string legs)
        => legs.Length == 0
            ? "no viewer is on a leg the relay times"
            : $"the relay times srt legs only, and this stream is watched over {legs}";
}
