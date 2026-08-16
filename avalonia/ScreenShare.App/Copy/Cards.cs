namespace ScreenShare.App.Copy;

/// <summary>
/// What a card says in its own right: what it is, and what it shows in place of the thing it usually draws.
///
/// The other tables here are keyed on something the backend sent: an identifier (<see cref="Words"/>), a choice
/// (<see cref="Descriptions"/>), a field (<see cref="Fields"/>), a statement (<see cref="Statements"/>).
/// These are keyed on nothing, and live here because every word on screen belongs where the words are
/// (<c>avalonia/README.md</c>, "Layout") rather than in markup or spelled twice at two render functions.
///
/// A refusal is shown as the backend wrote it (<c>docs/ipc-api.md</c>, "Errors"); what is here covers the states
/// no call failed in.
/// </summary>
public static class Cards
{
    /// <summary>
    /// Names where the picture is taken, the whole of the difference between the two routes: both carry the same
    /// encode (<see cref="Features.Broadcast.Preview.Model.PreviewRoute"/>).
    /// The off segment takes none, and the question still reads over it.
    /// </summary>
    public const string PreviewRouteChoice = "Where this picture is taken:";

    /// <summary>Nowhere: the segment that draws no picture and holds no decode open.</summary>
    public const string PreviewOffLabel = "Off";

    public const string PreviewLocalLabel = "Local";

    /// <summary>Spelled out, because "E2E" is a word for whoever built the pipeline.</summary>
    public const string PreviewEndToEndLabel = "End to end";

    /// <summary>
    /// What the off segment costs, the reason it is worth more than a way to blank a tile: the end-to-end route's
    /// reader slot and downstream bandwidth go back with the picture.
    /// </summary>
    public const string PreviewOffCost =
        "Nothing is drawn here and no decode is held open. The stream is unaffected: viewers go "
        + "on receiving it while this card is off.";

    /// <summary>
    /// Local route's cost, and the warning with it: the picture is taken before the relay, so a congested uplink,
    /// a relay dropping packets and a viewer on a bad link all leave this card looking perfect
    /// (<c>docs/viewer-architecture.md</c>, "What the broadcast preview draws").
    /// It and <see cref="PreviewEndToEndCost"/> make opposite claims, so neither can stand in for the other.
    /// </summary>
    public const string PreviewLocalCost =
        "This is what is being sent, decoded on this machine from a copy the encoder makes: it "
        + "never reaches the relay, costs one local decode and no bandwidth, and adds no viewer "
        + "to the counts beside it. It shows nothing about what viewers receive. Switch to end "
        + "to end for that, or read the viewer table and the round-trip plot.";

    /// <summary>
    /// End-to-end route's cost: the decode is opened with <c>StartReceive</c> like any tile, so the relay counts
    /// it among the viewers reported beside this card and it pays a viewer's downstream bandwidth.
    /// </summary>
    public const string PreviewEndToEndCost =
        "This is what a viewer receives: this machine's own stream, pulled back off the relay "
        + "over the leg the viewer screen receives on. It crosses the uplink, the relay and the "
        + "way back, so what it shows is the whole path, and it pays for that as a viewer does, "
        + "spending downstream bandwidth and counting as one of the viewers beside it.";

    public const string PreviewNotPublishing = "Nothing is publishing, so there is nothing being sent to show.";

    /// <summary>
    /// Leads with the stream being untouched: a card gone dark is what a reader could take for a broadcast that
    /// had.
    /// Names the picture and not the stream, the red control in the header being the one that stops the
    /// broadcast.
    /// </summary>
    public const string PreviewOff = "The preview is off. The stream is unaffected.";

    /// <summary>
    /// A screen the backend has been asked to read and has not produced a picture from yet.
    /// A capture element takes a moment to start, and a tile with nothing in it and nothing to say reads as
    /// broken.
    /// </summary>
    public const string ScreenOpening = "Opening.";

    /// <summary>
    /// Leads with nothing being shared yet: a live picture of a screen, in an app whose purpose is sending one,
    /// is what a reader could take for a stream already on the air.
    /// The picture is the rectangle the stream would carry, read by the same element
    /// (<c>backend/internal/screensrc</c>).
    /// </summary>
    public const string ScreenPickerCost =
        "Nothing is being shared yet. Each picture is what that screen is showing now, read the "
        + "same way the stream would read it, so the one picked is what viewers would see.";

    /// <summary>
    /// A stream on the air with no preview behind it.
    /// The reason is in the backend's log rather than on the contract, so the sentence names the fact and not a
    /// cause.
    /// </summary>
    public const string PreviewNotPreviewed = "This stream is on the air and is not being previewed locally.";

    /// <summary>
    /// No leg to receive on yet.
    /// A decode is keyed by the stream and the protocol together, so there is nothing to ask for until the
    /// settings name one (<c>Features/Viewer/Tile/Model/TileLeg.cs</c>).
    /// </summary>
    public const string PreviewNoWatchLeg =
        "The settings have not said which protocol a viewer receives on yet.";

    /// <summary>
    /// Its own state and not the tile's "Connecting.": that one is a pipeline up with no frame out of it, this
    /// the round trip before there is a pipeline at all.
    /// </summary>
    public const string PreviewOpening = "Opening a decode of this stream off the relay.";

    /// <summary>
    /// Caption beside the nudge card's title.
    /// It and <see cref="NudgeInert"/> are one claim, kept side by side so neither can drift into promising a
    /// live-safe quality change.
    /// </summary>
    public const string NudgeCaveat = "quality changes take a restart";

    /// <summary>
    /// Why the nudge track takes no hand: both engines run a child built from an argv and neither takes a new
    /// quality back, so no effect changes it without rebuilding the pipeline (<c>docs/field-availability.md</c>,
    /// "The rule").
    /// </summary>
    public const string NudgeInert =
        "Changing quality restarts the stream. Setup applies it, and says so before it does.";

    public const string ConfigReadOnly =
        "Read-only while live. Every setting here reaches a running stream by restarting it, "
        + "which is what setup's commit does.";

    /// <summary>A resolve that has not answered, what an empty configuration card shows while a stream runs.</summary>
    public const string ConfigUndescribed = "Reading what the running stream was built from.";

    /// <summary>
    /// Other empty configuration card: nothing to describe rather than nothing back yet.
    /// Names setup, this card describing a pipeline that was built and setup holding what the next one would be
    /// built from.
    /// </summary>
    public const string ConfigIdle = "Nothing is publishing. Setup holds what a stream would be built from.";

    /// <summary>
    /// What a preset covers, said where the reader is about to make one.
    /// The relay and the watch settings are outside it, one belonging to a deployment and the other to this
    /// machine's drivers (<c>docs/presets.md</c>).
    /// </summary>
    public const string PresetsCovers =
        "A preset is one way of publishing: the source, the quality, the audio and the transport. "
        + "The relay and the watch settings stay as they are, because they belong to this "
        + "machine rather than to what it sends.";

    /// <summary>
    /// What the viewer table says in place of rows.
    /// Three absences and three sentences, each leaving a publisher with a different thing to do next: start a
    /// stream, wait for the relay to be asked, or send somebody the link.
    /// </summary>
    public const string ViewersIdle = "Nothing is publishing, so nobody can be watching this machine.";

    public const string ViewersUnasked = "The relay has not been asked yet, so there is nobody to list.";

    public const string ViewersNone = "Nobody is connected to this stream yet.";

    /// <summary>
    /// What the preflight list says when the form found nothing to say.
    /// A line rather than an empty panel: a card that vanishes with the last warning reads as a card that broke.
    /// </summary>
    public const string PreflightClear = "Nothing to fix: these settings publish as they stand.";

    /// <summary>Heading over the quality group's raw knobs, the one word on that card the form does not supply.</summary>
    public const string AdvancedTitle = "Advanced";

    /// <summary>
    /// Latency plot's two series, named beside the rule drawn in each one's stroke.
    /// Short because they sit inside the plot: the card's own caption says whose figures these are.
    /// </summary>
    public const string PlotRoundTripLegend = "rtt";

    public const string PlotLossLegend = "loss";

    /// <summary>
    /// What a stream waiting out a relaunch says beside the pill.
    /// The pill stays up through a backoff, the reader having stopped nothing, so this is the whole of what
    /// separates a stream carrying frames from one coming back.
    /// </summary>
    public static string RetryAttempt(int attempt, int budget) => $"reconnecting, attempt {attempt} of {budget}";

    /// <summary>
    /// What the turning arc beside an unreachable backend means: waiting is enough, nothing has to be restarted.
    /// Shared by every screen drawing the arc, so it names no button only one of them offers.
    /// </summary>
    public const string Redialling =
        "This window is dialling the backend, and goes on doing so until it answers or the window closes.";

    /// <summary>
    /// Quantizer track's three labels, each carrying the number on the track it sits over.
    /// Formatted and not written, the scale being the codec's and the encoder's: 51 is where the H.26x scale
    /// ends, and a reader on libvpx or on an encoder exposing a raw quantizer index would be shown somebody
    /// else's ceiling (<see cref="Features.Setup.Model.QualityLayout"/>).
    /// </summary>
    public static string QuantizerFloor(int at) => $"{at}: visually lossless, huge";

    public static string QuantizerCeiling(int at) => $"{at}: unusable";

    public static string QuantizerBand(int from, int to) => $"{from}–{to} recommended for screen content";

    /// <summary>
    /// What the button over the audio list says. It carries the form's own row past the end of the list, whose
    /// value is the absent kind, so the face names what pressing it does rather than what the row holds
    /// (<see cref="Features.Setup.AudioStep.ViewModel.AudioStepViewModel.Add"/>).
    /// </summary>
    public const string AudioAdd = "Add a source";

    /// <summary>What a stream with an empty audio list sends, in place of the rows.</summary>
    public const string AudioEmpty = "Nothing is listed, so the stream goes out silent.";

    /// <summary>
    /// Which of the list's controls reach a stream that is already running, said once over the columns rather
    /// than beside every entry's control.
    /// The controls are named off the form, <c>Field.live</c> deciding which are in the sentence at all: the
    /// engine behind the capture backend answers it, and it moves with the settings
    /// (<c>docs/field-availability.md</c>, "A live stream blocks no field").
    /// </summary>
    public static string AudioLive(string control) =>
        $"{control} reaches a running stream, so moving it costs no viewer a reconnect.";

    public static string AudioLive(string first, string second) =>
        $"{first} and {second} reach a running stream, so moving either costs no viewer a reconnect.";

    /// <summary>
    /// An empty store, not <see cref="Statements"/>' unreadable-store notice: that one is the store failing, this
    /// one a reader who has saved nothing.
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
    /// Written once, the latency plot and the header stat bar both showing it and being unable to come to say
    /// different things about one roster.
    /// </summary>
    /// <param name="legs">
    /// <see cref="Features.Broadcast.Model.BroadcastSnapshot.Legs"/>, empty where the roster names no leg.
    /// </param>
    public static string Untimed(string legs)
        => legs.Length == 0
            ? "no viewer is on a leg the relay times"
            : $"the relay times srt legs only, and this stream is watched over {legs}";
}
