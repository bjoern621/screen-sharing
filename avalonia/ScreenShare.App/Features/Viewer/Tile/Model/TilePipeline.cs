using ScreenShare.Api.V1;

namespace ScreenShare.App.Features.Viewer.Tile.Model;

/// <summary>
/// What a decode turned out to be once it ran, as a tile prints it.
///
/// Reported rather than asked for: a render chain falls back on a machine that cannot run its elements,
/// and a hardware decoder may download its own frames,
/// so a tile drawing what the settings named would draw a request instead of a result.
///
/// One shape for every producer, it being one question.
/// A relay decode reports it on <c>ReceiveStream</c>,
/// the publish's local preview on <c>PublishState.Live.preview</c>, a screen's on <c>PreviewedMonitor</c>,
/// the messages differing only in what owns them.
/// One shape means one tile rather than one per producer.
/// </summary>
/// <param name="Live">
/// A decoded frame has left the pipeline.
/// Before that the pipeline is connecting or holding something it cannot decode, and the rest is empty for want
/// of a negotiation.
/// </param>
/// <param name="HasAudio">
/// The pipeline carries a sound track, played at <paramref name="Volume"/> and <paramref name="Muted"/>.
/// Volume is linear gain from zero, 1 untouched.
/// </param>
/// <param name="Transfer">
/// Transfer characteristic of the decoded frames as GStreamer spells it, "smpte2084".
/// <paramref name="Hdr"/> is the backend's verdict on whether that curve carries more range than a standard
/// display shows.
/// </param>
/// <param name="ToneMap">
/// The rung rolling that range down was built into the pipeline, so what ran and not what was asked for.
/// </param>
/// <param name="CanToneMap">
/// This machine has an element that rolls the range down,
/// <paramref name="ToneMapMissing"/> being the first one it needs and does not register.
/// That name is empty both where the machine can and where the platform has no such route at all,
/// the two separated by <paramref name="CanToneMap"/>.
/// </param>
/// <param name="Failure">
/// Why no picture is coming, absent while the pipeline is merely opening.
/// Separates a decode still connecting from one nothing is coming on, the whole of what a dark tile has to say.
/// </param>
public readonly record struct TilePipeline(
    bool Live,
    bool HasAudio,
    double Volume,
    bool Muted,
    string Transfer = "",
    bool Hdr = false,
    bool ToneMap = false,
    bool CanToneMap = false,
    string ToneMapMissing = "",
    Text? Failure = null)
{
    /// <summary>State of one relay decode, null where nothing is decoding that pair.</summary>
    public static TilePipeline? Of(ReceiveStream? decode) => decode is null
        ? null
        : new TilePipeline(
            decode.Live,
            decode.HasAudio, decode.Volume, decode.Muted,
            decode.Transfer, decode.Hdr, decode.ToneMap, decode.CanToneMap, decode.ToneMapMissing,
            decode.Failure);

    /// <summary>
    /// State of the publish's local preview, null where the backend runs none.
    /// A publish with no preview is a real state, a format with no local carriage or a pipeline that would not start,
    /// and it reads here as a decode nobody opened does.
    ///
    /// A preview carries no sound, which is the tap and not an omission here.
    /// The publish child copies video alone to the loopback port, so there is no track to play and none to meter,
    /// and the call that would set a level is keyed by a <c>StreamRef</c> no preview has.
    /// It reports silence as a video-only stream off the relay does, so the tile needs no case for it.
    /// </summary>
    public static TilePipeline? Of(PublishState.Types.Preview? preview) => preview is null
        ? null
        : new TilePipeline(
            preview.Live,
            HasAudio: false, Volume: 1, Muted: false);

    /// <summary>
    /// State of one monitor's preview, null where the backend reads no such screen.
    /// The empty fields are facts rather than gaps: nothing encoded these frames, so no decoder and no hardware
    /// verdict, nothing carried them, so no leg, and a screen has no sound track.
    /// What is left is whether a picture has come off the screen, separating a preview opening from one drawing.
    /// The figures strip prints what it is given, so the empty ones do not appear.
    /// </summary>
    public static TilePipeline? Of(PreviewedMonitor? screen) => screen is null
        ? null
        : new TilePipeline(
            screen.Live,
            HasAudio: false, Volume: 1, Muted: false);
}
