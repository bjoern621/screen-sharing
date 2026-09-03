namespace ScreenShare.App.Copy;

/// <summary>
/// One figure of the stats panel.
/// A label with no tip is a number nobody can act on, so the two are one entry and neither is written alone.
/// </summary>
/// <param name="Label">Name of the figure, as the row prints it.</param>
/// <param name="Tip">
/// What a reading of the figure is evidence of, beyond the unit already printed beside it
/// (<c>docs/tooltips.md</c>).
/// </param>
public readonly record struct Counter(string Label, string Tip);

/// <summary>
/// Every word of the stats panel: its headings, its rows, and the counters the transport's own elements keep.
/// Keyed on the identifiers the two sides share:
/// the contract's field names for a decode's sample, and an element's own names for its counters
/// (<c>api/proto/screenshare/v1/text.proto</c>).
/// Which transport rows a decode reports follows from the leg it was opened on,
/// SRT counting a link and RTSP a jitter buffer per track, so two legs of one stream carry two sets of evidence.
/// </summary>
public static class Counters
{
    /// <summary>Panel's headings, in the order frames pass through the stages they name.</summary>
    private static readonly Dictionary<string, Counter> Headings = new()
    {
        ["section.stream"] = new(
            "Arriving",
            "The encoded stream as it reaches this computer, read off the decoder's input. What the publisher sent and the relay carried."),
        ["section.picture"] = new(
            "Picture",
            "What came out of the decoder: the picture as encoded, at the publisher's size, before this window scaled anything."),
        ["section.decode"] = new(
            "Decode",
            "Which decoder took the stream, where it left the frames, and what it threw away to stay on time."),
        ["section.render"] = new(
            "Render",
            "What happened between the decoder and this window: the render chain, the memory the frames arrived in, and what this computer did with them."),
        ["section.timing"] = new(
            "Timing",
            "How the decode is paced. A live stream cannot slow down to catch up, so anything not decoded in time is dropped."),
        ["section.delay"] = new(
            "Delay",
            "What each stage costs a frame, from the publisher's screen to this window. The relay cannot be timed, so the total is a floor."),
        ["section.audio"] = new(
            "Audio",
            "The sound track this decode is carrying. Absent on a stream published without one."),
        ["section.window"] = new(
            "This window",
            "What this window got and drew, the one part the backend cannot see. A compositor too slow to take a frame shows only here."),
    };

    /// <summary>
    /// What each element of the transport is, keyed on its factory name.
    /// The counters under a block mean different things depending on which element keeps them.
    /// </summary>
    private static readonly Dictionary<string, Counter> Elements = new()
    {
        ["srtsrc"] = new(
            "SRT link",
            "The SRT connection this tile receives on: the leg from the relay to here. Its counters describe that hop alone, not the publisher's."),
        ["rtpjitterbuffer"] = new(
            "RTP jitter buffer",
            "Reorders RTP packets and requests missing ones again. One per track, so two blocks are two tracks rather than two problems."),
    };

    /// <summary>
    /// Every row, keyed on the contract field it prints,
    /// or in a transport block on the element's own name for the counter.
    /// </summary>
    private static readonly Dictionary<string, Counter> Fields = new()
    {
        // section.stream
        ["codec_description"] = new(
            "Codec",
            "The format the publisher encoded in, as the decoder identifies it. The publisher chooses it, and every viewer receives the same one."),
        ["profile"] = new(
            "Profile",
            "The subset of the codec the stream uses. A hardware decoder that supports the codec does not necessarily support every profile."),
        ["level"] = new(
            "Level",
            "The codec's ceiling on resolution, frame rate, and bitrate. A decoder refusing a stream it otherwise supports is usually refusing the level."),
        ["video_mbps"] = new(
            "Bitrate",
            "What the video arrives at, measured over the last second. Consistently below the publisher's setting means the path from the relay cannot carry the stream."),
        ["video_fps"] = new(
            "Frames arriving",
            "Encoded frames that reached this computer in the last second. Below the declared rate means frames are lost on the way, not dropped here."),
        ["declared_fps"] = new(
            "Declared rate",
            "The frame rate the stream says it runs at. The figure the two measured rates are judged against."),
        ["keyframes"] = new(
            "Keyframes",
            "Frames decodable from cold since this decode started. A joining viewer waits for the next one, so a long gap means a long black tile."),
        ["since_keyframe_sec"] = new(
            "Last keyframe",
            "How long ago the last one arrived. Growing past the publisher's keyframe interval means the stream stopped delivering them."),
        ["video_bytes"] = new(
            "Video received",
            "Everything this decode has taken in since it opened. The running total behind the bitrate, and what a data cap counts."),

        // section.picture
        ["picture_size"] = new(
            "Size",
            "The picture as the publisher encoded it, not the tile's size. What this window draws is under Render."),
        ["pixel_format"] = new(
            "Pixel format",
            "How the decoded samples are laid out, in GStreamer's spelling. Depth and chroma sampling beside it are read out of this name."),
        ["depth"] = new(
            "Depth",
            "Bits per component. Eight is the delivery default. Ten is needed to carry more range than a standard display shows."),
        ["subsampling"] = new(
            "Chroma sampling",
            "How much color resolution survived the encode. 4:4:4 keeps all of it and text stays sharp. 4:2:0 keeps a quarter, the video-call look."),
        ["colorimetry"] = new(
            "Color",
            "The full color description: range, matrix, primaries, and transfer. Anything unstated is guessed, and a wrong guess draws the picture at the wrong brightness."),
        ["transfer"] = new(
            "Transfer",
            "The curve mapping code values to light. An HDR curve drawn without conversion looks flat and dim rather than obviously wrong."),
        ["chroma_site"] = new(
            "Chroma siting",
            "Where a subsampled color sample sits against its brightness samples. A mismatch shifts color by half a pixel: a colored edge on thin lines."),
        ["pixel_aspect"] = new(
            "Pixel aspect",
            "The shape of one pixel. Anything other than 1:1 means the picture must be stretched to look right."),
        ["interlace"] = new(
            "Interlacing",
            "Whether the picture arrives as whole frames or fields. A screen capture is progressive. Anything else came from a camera or broadcast chain."),

        // section.decode
        ["decoder"] = new(
            "Decoder",
            "What decoded this stream, picked by this computer. Two computers watching one stream can run different decoders."),
        ["decode_memory"] = new(
            "Decoded into",
            "Where the decoder left its frames. A hardware decoder reporting system memory copied every frame across the bus."),
        ["discarded_fps"] = new(
            "Discarded to keep up",
            "Frames dropped each second to keep the picture current. A steady rate means the sender could lower its frame rate."),
        ["tone_map"] = new(
            "Tone mapping",
            "Whether this decode rolls an HDR stream down to this display's range. A computer without a converter opens the decode without it."),

        // section.render
        ["chain"] = new(
            "Render chain",
            "What converts frames between the decoder and this window. A chain that leaves color to the driver is why two computers draw different brightness."),
        ["render_memory"] = new(
            "Handed over in",
            "Where the frames were when the render chain handed them over. Differing from the decoder's memory means a download or upload."),
        ["render_format"] = new(
            "Drawn from",
            "The pixel format and color this window draws from. Pinned, because a guessed transfer function washes out desktop content."),
        ["render_size"] = new(
            "Drawn at",
            "The size the frames reach this window at. Smaller than the decoded picture means the chain scaled the stream down to this tile."),
        ["render_fps"] = new(
            "Frames drawn",
            "Frames drawn over the last second. Below the arriving rate means this computer is not keeping up with a stream it receives fine."),
        ["rendered"] = new(
            "Frames rendered",
            "Everything drawn since this decode opened. The running total behind the drawn rate."),
        ["sink_dropped"] = new(
            "Dropped at the last step",
            "Frames dropped after everything was already spent on them. Zero on a healthy decode. Frames shed to stay current are dropped much earlier instead."),

        // section.timing
        ["live"] = new(
            "Live timing",
            "Whether the decode runs against a clock it cannot pause. Every relay leg does. What is not decoded in time is dropped."),
        ["latency"] = new(
            "Latency window",
            "How long a frame is held before playing, the buffer against jitter. Larger is steadier and later. The floor under this tile's speed."),
        ["position_sec"] = new(
            "Position",
            "The running time the decode has reached. Frozen while uptime climbs means the stream stalled, which separates a stalled tile from a still picture."),
        ["uptime_sec"] = new(
            "Uptime",
            "How long this decode has been running. It restarts whenever the decode is rebuilt, which turning tone mapping on does."),

        // section.delay
        ["delay.publish"] = new(
            "Capture and encode",
            "How long the publisher held a frame between reading the screen and having it encoded. A faster effort step or a shorter lookahead shortens this stage."),
        ["delay.path"] = new(
            "Publisher to here",
            "The whole way between the publishing machine and this one, relay included. Read from a clock in each frame, so H.264 and H.265 carry it and other formats do not."),
        ["delay.receive"] = new(
            "Decode",
            "Time from packet arrival to a frame ready to draw. Rising to meet the latency window means the decode is about to drop frames."),
        ["delay.receive_peak"] = new(
            "Decode, worst",
            "The longest a single frame has taken on this decode. It only rises, so one slow frame shows here where an average hides it."),
        ["delay.present"] = new(
            "Held for play time",
            "How long each frame waited to be drawn at its play time. It shrinks as the decode above grows, the two together making the window."),
        ["delay.total"] = new(
            "At least, end to end",
            "Everything measured, added up, nothing counted twice. A floor wherever a stage is missing: a stream without its own timestamp leaves the relay out."),

        // section.audio
        ["audio_codec_description"] = new(
            "Codec",
            "The coding format the sound track is in, as the decoder identifies it."),
        ["audio_decoder"] = new(
            "Decoder",
            "What decodes the sound track."),
        ["audio_format"] = new(
            "Sample format",
            "How the decoded audio samples are laid out."),
        ["audio_rate"] = new(
            "Sample rate",
            "How many samples a second the track carries."),
        ["audio_channels"] = new(
            "Channels",
            "How many channels the track carries."),
        ["audio_kbps"] = new(
            "Bitrate",
            "What the sound track arrives at, measured over the last second."),
        ["audio_bytes"] = new(
            "Audio received",
            "Everything the sound track has taken in since this decode opened."),

        // section.window
        ["window.size"] = new(
            "Handed over at",
            "The size of the frames this window is given, as the window sees it. What a marker over the picture is positioned against."),
        ["window.frames"] = new(
            "Frames taken",
            "Frames this window has taken since it subscribed. A window behind the compositor takes frames it never puts on screen."),
        ["window.dropped"] = new(
            "Dropped waiting for this window",
            "Frames the backend discarded because this window held every slot of the lent pool. Evidence that the drawing side is the slow half."),

        // srtsrc
        ["packets-received"] = new(
            "Packets",
            "Everything that arrived on the link, retransmits included. The denominator for the three counters under it."),
        ["packets-received-lost"] = new(
            "Lost",
            "Packets that never arrived, after retransmission had its chance. Anything above zero is picture that cannot be reconstructed: blocking and smearing."),
        ["packets-received-retransmitted"] = new(
            "Retransmitted",
            "Packets that arrived only because the sender was asked again. Climbing steadily with nothing lost beside it means a lossy path SRT covers for."),
        ["packets-received-dropped"] = new(
            "Dropped",
            "Packets that arrived too late to be played and were thrown away. Raising the SRT latency setting trades delay for fewer of these."),
        ["receive-rate-mbps"] = new(
            "Receive rate",
            "What the link delivers, as SRT measures it. It counts everything on the wire, so it sits above the video bitrate by protocol overhead."),
        ["bandwidth-mbps"] = new(
            "Estimated capacity",
            "What SRT believes the path from the relay can carry. A stream whose bitrate approaches it will start losing packets."),
        ["rtt-ms"] = new(
            "Round trip",
            "How long a packet takes to the relay and back. A retransmit cannot arrive faster, so the latency window needs several times this."),
        ["negotiated-latency-ms"] = new(
            "Buffer",
            "The delay both ends agreed to hold packets for: SRT's time to notice a gap and fill it. The relay enforces a floor."),

        // rtpjitterbuffer
        ["num-pushed"] = new(
            "Pushed",
            "Packets handed to the decoder in the right order: the buffer doing its job."),
        ["num-lost"] = new(
            "Lost",
            "Packets the buffer gave up waiting for. Every one is a hole the decoder has to conceal."),
        ["num-late"] = new(
            "Late",
            "Packets that arrived after their play time. A climbing count means the buffer is too short for this path."),
        ["num-duplicates"] = new(
            "Duplicates",
            "Packets that arrived more than once. A few are normal where retransmits cross the original. Many mean the path echoes traffic."),
        ["rtx-count"] = new(
            "Retransmits asked for",
            "How many times the buffer noticed a gap and asked the sender to send the packet again."),
        ["rtx-success-count"] = new(
            "Retransmits recovered",
            "How many of those requests arrived in time to be used. The difference from the count above is recovery that came too late."),
    };

    public static Counter Heading(string id) => Look(Headings, id);

    public static Counter Element(string factory) => Look(Elements, factory);

    public static Counter Field(string key) => Look(Fields, key);

    /// <summary>
    /// Entry for an identifier, falling back to the identifier itself with nothing said about it.
    /// A counter this build has no words for is still one the backend sends,
    /// and a row printing its raw key is one a reader can search for and report.
    /// </summary>
    private static Counter Look(Dictionary<string, Counter> table, string id) =>
        id.Length > 0 && table.TryGetValue(id, out var entry) ? entry : new Counter(id, "");
}
