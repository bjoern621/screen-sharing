namespace ScreenShare.App.Copy;

/// <summary>
/// What a card says in its own right: what it is, and what it shows in place of the thing it usually draws.
///
/// The other tables here are keyed on what the backend sent:
/// an identifier (<see cref="Words"/>), a choice (<see cref="Descriptions"/>), a field (<see cref="Fields"/>),
/// a statement (<see cref="Statements"/>).
/// These are keyed on nothing.
/// Here rather than in markup or spelled twice at two render functions (<c>avalonia/README.md</c>, "Layout").
///
/// A refusal reads as the backend wrote it (<c>docs/ipc-api.md</c>, "Errors").
/// These cover the states no call failed in.
/// </summary>
public static class Cards
{
    /// <summary>
    /// Names where the picture is taken, the whole difference between the two routes:
    /// both carry the same encode (<see cref="Features.Broadcast.Preview.Model.PreviewRoute"/>).
    /// The off segment takes none, and the question still reads over it.
    /// </summary>
    public const string PreviewRouteChoice = "Where this picture is taken:";

    /// <summary>Nowhere: draws no picture, holds no decode open.</summary>
    public const string PreviewOffLabel = "Off";

    public const string PreviewLocalLabel = "Local";

    /// <summary>Spelled out: "E2E" is a word for whoever built the pipeline.</summary>
    public const string PreviewEndToEndLabel = "End to end";

    /// <summary>
    /// What the off segment costs:
    /// the end-to-end route's reader slot and downstream bandwidth go back with the picture.
    /// </summary>
    public const string PreviewOffCost =
        "No picture is drawn and no decode is held open. The stream is unaffected. Viewers keep "
        + "receiving it while this card is off.";

    /// <summary>
    /// Local route's cost, and its warning.
    /// The picture is taken before the relay,
    /// so a congested uplink, a dropping relay and a bad viewer link all leave this card looking perfect
    /// (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws").
    /// Opposite claim to <see cref="PreviewEndToEndCost"/>, so neither stands in for the other.
    /// </summary>
    public const string PreviewLocalCost =
        "This is what is being sent, decoded on this computer from a copy the encoder makes. It "
        + "never reaches the relay, costs one local decode and no bandwidth, and adds no viewer "
        + "to the counts. It shows nothing about what viewers receive. Switch to end to end for "
        + "that, or read the viewer table and the round-trip plot.";

    /// <summary>
    /// End-to-end route's cost: decode opened with <c>StartReceive</c> like any tile,
    /// so the relay counts it among the viewers beside this card and it pays a viewer's downstream bandwidth.
    /// </summary>
    public const string PreviewEndToEndCost =
        "This is what a viewer receives: this computer's own stream, pulled back off the relay. "
        + "It crosses the uplink, the relay, and the way back, so it shows the whole path. It "
        + "pays for that as a viewer does, spending downstream bandwidth and counting as one of "
        + "the viewers beside it.";

    public const string PreviewNotPublishing = "Nothing is publishing, so there is nothing to show.";

    /// <summary>
    /// Leads with the stream being untouched: a dark card is what a reader could take for a stopped broadcast.
    /// Names the picture and not the stream, the header's red control being what stops the broadcast.
    /// </summary>
    public const string PreviewOff = "The preview is off. The stream is unaffected.";

    /// <summary>
    /// A screen the backend was asked to read and has produced no picture from.
    /// A capture element takes a moment to start, and a tile with nothing in it and nothing to say reads as broken.
    /// </summary>
    public const string ScreenOpening = "Opening.";

    /// <summary>
    /// Leads with nothing being shared: in an app whose purpose is sending a screen,
    /// a live picture of one is what a reader could take for a stream already live.
    /// The picture is the rectangle the stream would carry, read by the same element
    /// (<c>backend/internal/screensrc</c>).
    /// </summary>
    public const string ScreenPickerCost =
        "Nothing is being shared yet. Each picture shows what that screen displays now, read the "
        + "same way the stream would read it. The one picked is what viewers would see.";

    /// <summary>
    /// A stream live with no preview behind it.
    /// The reason is in the backend's log rather than on the contract, so the sentence names the fact alone.
    /// </summary>
    public const string PreviewNotPreviewed = "This stream is live and not being previewed locally.";

    /// <summary>
    /// No leg to receive on.
    /// A decode is keyed by stream and protocol together,
    /// so there is nothing to ask for until the settings name one (<c>Features/Viewer/Tile/Model/TileLeg.cs</c>).
    /// </summary>
    public const string PreviewNoWatchLeg =
        "The settings name no protocol for a viewer to receive on. Pick one under Watching.";

    /// <summary>
    /// Its own state, apart from the tile's "Connecting.":
    /// that one is a pipeline up with no frame out of it, this the round trip before there is a pipeline at all.
    /// </summary>
    public const string PreviewOpening = "Opening a decode of this stream off the relay.";

    /// <summary>
    /// Caption beside the nudge card's title.
    /// One claim with <see cref="NudgeInert"/>, side by side so neither promises a live-safe quality change.
    /// </summary>
    public const string NudgeCaveat = "quality changes take a restart";

    /// <summary>
    /// Why the nudge track takes no hand: both engines run a child built from an argv,
    /// and neither takes a quality back, so no effect changes it without rebuilding the pipeline
    /// (<c>docs/field-availability.md</c>, "The rule").
    /// </summary>
    public const string NudgeInert =
        "Changing quality restarts the stream. Setup applies it, and says so before it does.";

    public const string ConfigReadOnly =
        "Read-only while live. Every setting here reaches a running stream by restarting it, "
        + "through setup's commit.";

    /// <summary>Unanswered resolve, what an empty configuration card shows while a stream runs.</summary>
    public const string ConfigUndescribed = "Reading what the running stream was built from.";

    /// <summary>
    /// Other empty configuration card: nothing to describe rather than nothing back.
    /// Names setup: this card describes a pipeline that was built, setup holds what the next one is built from.
    /// </summary>
    public const string ConfigIdle = "Nothing is publishing. Setup holds what a stream would be built from.";

    /// <summary>
    /// What a preset covers, said where the reader is about to make one.
    /// The relay and the watch settings are outside it, one a deployment's and the other this machine's drivers
    /// (<c>docs/presets.md</c>).
    /// </summary>
    public const string PresetsCovers =
        "A preset is one way of publishing: the source, the quality, the audio, and the transport. "
        + "The relay and watch settings stay as they are. They belong to this computer rather "
        + "than to what it sends.";

    /// <summary>
    /// What the viewer table says in place of rows.
    /// One sentence per absence, each leaving the publisher a different thing to do next:
    /// start a stream, wait for the relay to be asked, or send somebody the link.
    /// </summary>
    public const string ViewersIdle = "Nothing is publishing, so nobody can be watching this computer.";

    public const string ViewersUnasked = "The relay has not been asked yet, so there is nobody to list.";

    public const string ViewersNone = "Nobody is connected to this stream yet.";

    // --- The group -----------------------------------------------------------------

    /// <summary>Heading over the member list.</summary>
    public const string MembersTitle = "In the group";

    /// <summary>
    /// Beside a row the relay is carrying a stream from.
    /// Sending only: who watches what belongs to the machine doing it,
    /// so the group states presence and publication and nothing else.
    /// </summary>
    public const string MemberPublishing = "sending";

    /// <summary>Beside the row this machine holds, which nothing else on it distinguishes.</summary>
    public const string MemberSelf = "this computer";

    /// <summary>
    /// What a row says about a member beside their name, empty for one that is neither.
    /// In words rather than a mark, so the list needs no legend to be read.
    /// </summary>
    public static string MemberDetail(bool isSelf, bool isPublishing) => (isSelf, isPublishing) switch
    {
        (true, true) => $"{MemberSelf}, {MemberPublishing}",
        (true, false) => MemberSelf,
        (false, true) => MemberPublishing,
        _ => "",
    };

    public const string MembersJoin = "Join the group";

    public const string MembersLeave = "Leave the group";

    /// <summary>
    /// What the member list says in place of rows.
    /// One sentence per state, each leaving the reader a different thing to do: wait, join, or hand somebody the group key.
    /// </summary>
    public const string MembersUnread = "Reading who is in this group.";

    public const string MembersOutside =
        "This computer is outside the group. Joining lists it here beside everyone else, and lets the relay "
        + "carry what it publishes.";

    public const string MembersNone = "Nobody is listed in this group.";

    // --- The synthetic publishers ---------------------------------------------------

    /// <summary>Heading over the test-stream slots.</summary>
    public const string TestStreamsTitle = "Test streams";

    /// <summary>
    /// What the card describes, said once over the rows.
    /// A slot is the identity and the child is not, so a relaunch comes back as the row that was already there.
    /// </summary>
    public const string TestStreamsCovers =
        "Synthetic streams this computer publishes to the relay, one slot each, on paths of their own.";

    /// <summary>How much of the set is live, beside the heading.</summary>
    public static string TestStreamsRunning(int running, int slots) => $"{running} of {slots} sending";

    /// <summary>One slot, by its position in the set and the path it publishes to.</summary>
    public static string TestStreamSlotLabel(int slot, string name)
        => name.Length > 0 ? $"slot {slot} · {name}" : $"slot {slot}";

    /// <summary>Which relaunch the slot is on, counting from one.</summary>
    public static string TestStreamAttempt(int attempt) => $"attempt {attempt}";

    public const string TestStreamSending = "sending";

    /// <summary>
    /// A slot the set still holds with nothing publishing into it, what a relaunch waiting out its backoff reads as.
    /// </summary>
    public const string TestStreamStopped = "not sending";

    public const string TestStreamsUnread = "Reading the test streams.";

    public const string TestStreamsNone = "No synthetic streams are running on this computer.";

    /// <summary>
    /// What the preflight list says when the form found nothing to say.
    /// A line rather than an empty panel: a card that vanishes with the last warning reads as a card that broke.
    /// </summary>
    public const string PreflightClear = "Nothing to fix. These settings publish as they stand.";

    /// <summary>
    /// The relay check: its heading, the button that runs it, what the button does,
    /// and the line standing where the results go before one has run.
    ///
    /// The heading names what is reached rather than what is run,
    /// the reader having come to this card from the address and the ports above it.
    /// The unchecked-relay line is a line and not an empty card, for the reason <see cref="PreflightClear"/> is one.
    /// </summary>
    public const string RelayCheckTitle = "What answers on this relay";

    public const string RelayCheckAction = "Check";

    public const string RelayCheckHint =
        "Opens a connection to each of the relay's listeners and says which answered. "
        + "Sends no stream and changes nothing.";

    public const string RelayCheckUnrun = "No check has run on this relay.";

    /// <summary>Heading over the quality group's raw knobs, the one word on that card the form does not supply.</summary>
    public const string AdvancedTitle = "Advanced";

    /// <summary>
    /// Latency plot's two series, named beside the rule drawn in each one's stroke.
    /// Short: they sit inside the plot, and the card's caption says whose figures these are.
    /// </summary>
    public const string PlotRoundTripLegend = "rtt";

    public const string PlotLossLegend = "loss";

    /// <summary>
    /// What a stream waiting out a relaunch says beside the pill.
    /// The pill stays up through a backoff, the reader having stopped nothing,
    /// so this is the whole of what separates a stream carrying frames from one coming back.
    /// </summary>
    public static string RetryAttempt(int attempt, int budget) => $"reconnecting, attempt {attempt} of {budget}";

    /// <summary>
    /// What the turning arc beside an unreachable backend means: waiting is enough, nothing has to be restarted.
    /// Shared by every screen drawing the arc, so it names no button only one of them offers.
    /// </summary>
    public const string Redialling =
        "This window is dialing the backend, and keeps doing so until it answers or the window closes.";

    /// <summary>
    /// Quantizer track's three labels, each carrying the number on the track it sits over.
    /// Formatted and not written, the scale being the codec's and the encoder's:
    /// 51 ends the H.26x scale, and libvpx or a raw quantizer index would show somebody else's ceiling
    /// (<see cref="Features.Setup.Model.QualityLayout"/>).
    /// </summary>
    public static string QuantizerFloor(int at) => $"{at}: visually lossless, huge";

    public static string QuantizerCeiling(int at) => $"{at}: unusable";

    public static string QuantizerBand(int from, int to) => $"{from}-{to} recommended for screen content";

    /// <summary>
    /// What the button over the audio list says.
    /// It carries the form's own row past the end of the list, whose value is the absent kind,
    /// so the face names what pressing it does rather than what the row holds
    /// (<see cref="Features.Setup.AudioStep.ViewModel.AudioStepViewModel.Add"/>).
    /// </summary>
    public const string AudioAdd = "Add a source";

    /// <summary>What a stream with an empty audio list sends, in place of the rows.</summary>
    public const string AudioEmpty = "Nothing is listed, so the stream goes out silent.";

    /// <summary>
    /// Which of the list's controls reach a running stream,
    /// said once over the columns rather than beside every entry's control.
    /// The controls are named off the form, <c>Field.live</c> deciding which are in the sentence at all:
    /// the engine behind the capture backend answers it, and it moves with the settings
    /// (<c>docs/field-availability.md</c>, "A live stream blocks no field").
    /// </summary>
    public static string AudioLive(string control) =>
        $"{control} reaches a running stream, so moving it costs no viewer a reconnect.";

    public static string AudioLive(string first, string second) =>
        $"{first} and {second} reach a running stream, so moving either costs no viewer a reconnect.";

    /// <summary>
    /// An empty store, apart from <see cref="Statements"/>' unreadable-store notice:
    /// that one is the store failing, this one a reader who has saved nothing.
    /// </summary>
    public const string PresetsEmpty = "Nothing saved yet. Name the configuration below to keep it.";

    /// <summary>
    /// Why the list can be behind: presets are a file the backend does not run on, so no event says one appeared
    /// (<c>Backend/IBackend.cs</c>, <c>PresetsAsync</c>).
    /// </summary>
    public const string PresetsReread = "Read the saved presets again. Nothing announces a preset another window saved.";

    /// <summary>
    /// Why a watched stream states no round trip and no loss: SRT is the leg the relay times
    /// (<c>api/proto/screenshare/v1/session.proto</c>, <c>RelayReader</c>).
    /// Written once, so the latency plot and the header stat bar cannot say different things about one roster.
    /// </summary>
    /// <param name="legs">
    /// <see cref="Features.Broadcast.Model.BroadcastSnapshot.Legs"/>, empty where the roster names no leg.
    /// </param>
    public static string Untimed(string legs)
        => legs.Length == 0
            ? "no viewer is on a leg the relay times"
            : $"the relay times SRT legs only, and this stream is watched over {legs}";
}
