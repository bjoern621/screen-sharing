using ScreenShare.Api.V1;

namespace ScreenShare.App.Features.Viewer.Tile.Model;

/// <summary>
/// What a running decode turned out to be, as a tile prints it.
///
/// <b>It is reported rather than asked for, which is why it is worth drawing at all.</b> A
/// render chain falls back on a machine that cannot run its elements and a hardware decoder
/// may download its own frames, so a tile showing the chain the settings named would be
/// showing a request instead of a result.
///
/// <b>One shape with two producers, because it is one question.</b> A relay decode reports it
/// as <c>ReceiveStream</c> and the publish's local preview as <c>PublishState.Live.preview</c>,
/// and the two messages differ only in what owns them: a decode of somebody's stream is keyed
/// by the stream and the leg, and a preview is part of the publish it previews. What the
/// pipeline did is the same list of facts either way, and reading it into one shape here is
/// what lets there be one tile rather than one per screen.
/// </summary>
/// <param name="Live">Whether a decoded frame has left the pipeline. Until it has, the
/// pipeline is connecting or receiving something it cannot decode, and the rest is empty
/// because nothing has negotiated.</param>
/// <param name="Chain">The render chain the pipeline was built with.</param>
/// <param name="RenderMemory">The memory feature the sink's input pad carried.</param>
/// <param name="Decoder">The element the decoder autoplugged, and <paramref name="Hardware"/>
/// whether it ran on silicon.</param>
/// <param name="HasAudio">Whether the pipeline is carrying a sound track, with
/// <paramref name="Volume"/> and <paramref name="Muted"/> what it is playing it at.</param>
public readonly record struct TilePipeline(
    bool Live,
    string Chain,
    string RenderMemory,
    string Decoder,
    bool Hardware,
    bool HasAudio,
    double Volume,
    bool Muted)
{
    /// <summary>The state of one relay decode, and null where nothing is decoding that pair.</summary>
    public static TilePipeline? Of(ReceiveStream? decode) => decode is null
        ? null
        : new TilePipeline(
            decode.Live, decode.Chain, decode.RenderMemory, decode.Decoder, decode.Hardware,
            decode.HasAudio, decode.Volume, decode.Muted);

    /// <summary>
    /// The state of the publish's local preview, and null where the backend is running none.
    /// A publish with no preview is a real state - a format with no local carriage, a pipeline
    /// that would not start - and it reads here exactly as a decode nobody opened does.
    ///
    /// <b>A preview carries no sound, and that is the tap rather than an omission here.</b> The
    /// publish child copies video alone to the loopback port, so there is no track to play, no
    /// loudness to meter and nothing a volume would apply to - and the effect that would set
    /// one is keyed by a <c>WatchKey</c> the preview does not have. It reports silence the same
    /// way a video-only stream off the relay does, so the tile needs no case for it.
    /// </summary>
    public static TilePipeline? Of(PublishState.Types.Preview? preview) => preview is null
        ? null
        : new TilePipeline(
            preview.Live, preview.Chain, preview.RenderMemory, preview.Decoder, preview.Hardware,
            HasAudio: false, Volume: 1, Muted: false);

    /// <summary>
    /// The state of one monitor's preview, and null where the backend is reading no such screen.
    ///
    /// <b>It is the shortest of the three, and the empty fields are facts rather than gaps.</b>
    /// Nothing encoded these frames, so there is no decoder to name and no hardware verdict to
    /// give; nothing carried them, so there is no leg; and a screen has no sound track. What is
    /// left is whether a picture has come off the screen yet, which is what separates a preview
    /// that is opening from one that is drawing. The figures strip prints what it is given, so
    /// the empty ones simply do not appear.
    /// </summary>
    public static TilePipeline? Of(PreviewedMonitor? screen) => screen is null
        ? null
        : new TilePipeline(
            screen.Live, Chain: "", RenderMemory: "", Decoder: "", Hardware: false,
            HasAudio: false, Volume: 1, Muted: false);
}
