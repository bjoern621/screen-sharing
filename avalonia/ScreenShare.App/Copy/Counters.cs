namespace ScreenShare.App.Copy;

/// <summary>
/// One figure of the stats panel.
/// A label with no tip is a number nobody can act on, so the two are one entry and neither is written alone.
/// </summary>
/// <param name="Label">The name of the figure, as the row prints it.</param>
/// <param name="Tip">
/// What a reading of the figure is evidence of, never a restatement of the unit printed beside it
/// (<c>docs/tooltips.md</c>).
/// </param>
public readonly record struct Counter(string Label, string Tip);

/// <summary>
/// Every word of the stats panel: its headings, its rows, and the counters the transport's own elements keep.
///
/// Keyed on the identifiers the two sides share: the contract's field names for a decode's sample, and an
/// element's own names for its counters (<c>api/proto/screenshare/v1/text.proto</c>).
/// Which transport rows a decode reports follows from the leg it was opened on, SRT counting a link and RTSP
/// a jitter buffer per track, so two legs of one stream carry two different sets of evidence.
/// </summary>
public static class Counters
{
    /// <summary>The panel's headings, in the order frames pass through the stages they name.</summary>
    private static readonly Dictionary<string, Counter> Headings = new()
    {
        ["section.stream"] = new(
            "Arriving",
            "The encoded stream as it reaches this machine, read off the decoder's input. It describes what the publisher sent and what the relay carried, and nothing this machine did with it."),
        ["section.picture"] = new(
            "Picture",
            "What came out of the decoder, read off its output. This is the picture as it was encoded, at the size the publisher captured, before this window scaled anything."),
        ["section.decode"] = new(
            "Decode",
            "Which element decoded the stream and where it left the frames. A hardware decoder that leaves its frames in system memory has downloaded them, which costs the whole picture over the bus every frame."),
        ["section.render"] = new(
            "Render",
            "What happened between the decoder and this window: the chain that converted the frames, the memory they reached the sink in, and what the sink did with them."),
        ["section.timing"] = new(
            "Timing",
            "How the pipeline is paced. A live pipeline cannot slow down to catch up, so anything it cannot decode in time it drops."),
        ["section.delay"] = new(
            "Delay",
            "What each stage of the path costs a frame, from the publisher's screen to this window. One stage is missing from it: the relay terminates the incoming protocol and re-muxes for every viewer, and neither end can time that, so the total is a floor rather than the whole journey. The publishing stages are here only while this machine is the one publishing the stream, because nothing carries them over the relay."),
        ["section.audio"] = new(
            "Audio",
            "The sound track this decode is carrying. Absent on a stream published without one."),
        ["section.window"] = new(
            "This window",
            "What this window got and drew, which is the one part of the panel the backend cannot see. A compositor too slow to take a frame is only visible here."),
    };

    /// <summary>
    /// What each element of the transport is, keyed on its factory name.
    /// The counters under a block mean different things depending on which element keeps them.
    /// </summary>
    private static readonly Dictionary<string, Counter> Elements = new()
    {
        ["srtsrc"] = new(
            "SRT link",
            "The SRT connection this tile receives on, which is the watch leg from the relay to here. Its counters describe that hop alone and say nothing about the publisher's hop into the relay."),
        ["rtpjitterbuffer"] = new(
            "RTP jitter buffer",
            "The buffer that puts RTP packets back into order and asks the sender for the ones that did not arrive. A stream carrying video and audio has one per track, so two blocks here are two tracks rather than two problems."),
    };

    /// <summary>
    /// Every row, keyed on the contract field it prints or, in a transport block, on the element's own name
    /// for the counter.
    /// </summary>
    private static readonly Dictionary<string, Counter> Fields = new()
    {
        // section.stream
        ["codec_description"] = new(
            "Codec",
            "The coding format the publisher encoded in, as the decoder identifies it. It is what the stream is, not what this machine asked for: the publisher chooses it and every viewer receives the same one."),
        ["profile"] = new(
            "Profile",
            "The subset of the codec the stream uses. It bounds which decoders can play it at all: a hardware decoder that supports a codec does not necessarily support every profile of it."),
        ["level"] = new(
            "Level",
            "The codec's own ceiling on resolution, frame rate and bitrate for this stream. A decoder refusing a stream it otherwise supports is usually refusing the level."),
        ["video_mbps"] = new(
            "Bitrate",
            "What the video is arriving at, measured over the last second. Compare it against what the publisher set: consistently below it means the path between here and the relay cannot carry the stream."),
        ["video_fps"] = new(
            "Frames arriving",
            "How many encoded frames reached this machine over the last second. Below the declared rate means frames are being lost on the way rather than dropped here."),
        ["declared_fps"] = new(
            "Declared rate",
            "The frame rate the stream says it runs at. It is what the publisher encoded for, and the figure the two measured rates are judged against."),
        ["keyframes"] = new(
            "Keyframes",
            "How many frames since this decode started could have been decoded from cold. A viewer joining a stream waits for the next one, so a long gap between them is a long black tile on arrival."),
        ["since_keyframe_sec"] = new(
            "Last keyframe",
            "How long ago the most recent one arrived. Growing past the publisher's keyframe interval means the stream has stopped delivering them, which is what a tile that never recovers from a glitch is waiting on."),
        ["video_bytes"] = new(
            "Video received",
            "Everything this decode has taken in since it opened. It is the running total behind the bitrate, and the figure a data cap is measured against."),

        // section.picture
        ["picture_size"] = new(
            "Size",
            "The picture as the publisher encoded it. It is the stream's own size rather than the tile's: what this window draws is further down, under Render."),
        ["pixel_format"] = new(
            "Pixel format",
            "How the decoded samples are laid out, in GStreamer's spelling. The depth and the chroma sampling beside it are read out of this name rather than sent separately, so they cannot disagree with it."),
        ["depth"] = new(
            "Depth",
            "Bits per component. Eight is the delivery default; ten is what a stream needs before it can carry more range than a standard display shows."),
        ["subsampling"] = new(
            "Chroma sampling",
            "How much colour resolution survived the encode. 4:4:4 keeps all of it and is what stops text and edges fringing; 4:2:0 keeps a quarter and is the video-call look."),
        ["colorimetry"] = new(
            "Colour",
            "The whole colour description the frames carry: range, matrix, primaries and transfer. A sink that has to guess any of it is a sink that will get the brightness wrong."),
        ["transfer"] = new(
            "Transfer",
            "The curve mapping code values to light. Two of them carry more range than a standard display shows, and a tile drawing one of those without converting it looks flat and dim rather than obviously wrong."),
        ["chroma_site"] = new(
            "Chroma siting",
            "Where a subsampled colour sample sits against its brightness samples. A mismatch between what the stream says and what a converter assumes shifts colour by half a pixel, which reads as a coloured edge on one side of thin lines."),
        ["pixel_aspect"] = new(
            "Pixel aspect",
            "The shape of one pixel. Anything other than 1:1 means the picture has to be stretched to look right, and a tile that ignored it would draw the stream squashed."),
        ["interlace"] = new(
            "Interlacing",
            "Whether the picture arrives as whole frames or as fields. A screen capture is progressive; anything else here came from a camera or a broadcast chain."),

        // section.decode
        ["decoder"] = new(
            "Decoder",
            "The element that decoded this stream, picked by the pipeline rather than chosen here. Which one is picked follows from what this machine registers, so two machines watching one stream can be running different decoders."),
        ["decode_memory"] = new(
            "Decoded into",
            "Where the decoder left its frames. A hardware decoder reporting system memory downloaded its own output, which is a copy of every frame across the bus that the next stage has to push straight back."),
        ["tone_map"] = new(
            "Tone mapping",
            "Whether this decode was built with the step that rolls an HDR stream down into the range this display shows. It is what ran rather than what was asked for: a machine with no element for it builds the pipeline without one."),

        // section.render
        ["chain"] = new(
            "Render chain",
            "The elements between the decoder and this window, and what they promise about colour. A chain states its colour or leaves it to the driver, and one that leaves it is why two machines can draw one stream at different brightness."),
        ["render_memory"] = new(
            "Reached the sink in",
            "Where the frames were when the chain handed them over. Compare it against what the decoder produced: the two differing is a download or an upload, and it is the cost the chain was chosen to avoid."),
        ["render_format"] = new(
            "Sink takes",
            "The pixel format and colour the sink negotiated. It is pinned rather than left open, because a sink that takes raw video and guesses the transfer function washes out desktop content."),
        ["render_size"] = new(
            "Drawn at",
            "The size the frames reach the sink at. Smaller than the decoded picture means the chain scaled the stream down to this tile, which is work that stops the moment the tile grows."),
        ["render_fps"] = new(
            "Frames drawn",
            "How many frames left the sink over the last second. Below the rate arriving means this machine is not keeping up with a stream it is receiving fine."),
        ["rendered"] = new(
            "Frames rendered",
            "Everything the sink has taken since this decode opened. It is the running total behind the drawn rate."),
        ["sink_dropped"] = new(
            "Dropped by the sink",
            "Frames the sink threw away for arriving after their play time. This is the pipeline being late rather than the network losing anything, and it climbs on a machine that cannot decode the stream in real time."),

        // section.timing
        ["live"] = new(
            "Live pipeline",
            "Whether the pipeline is running against a clock it cannot pause. Every relay leg is: what it cannot decode in time it drops, rather than falling behind and catching up later."),
        ["latency"] = new(
            "Latency window",
            "How long the pipeline holds a frame before playing it, which is the buffering it configured against jitter. Larger is steadier and later, and it is the floor under how fast this tile can be."),
        ["position_sec"] = new(
            "Position",
            "The running time the pipeline has reached. Frozen while the uptime beside it keeps climbing means the stream has stalled, which is the one reading that separates a stalled tile from a still picture."),
        ["uptime_sec"] = new(
            "Uptime",
            "How long this decode has been running. It restarts whenever the pipeline is rebuilt, which turning tone mapping on does."),

        // section.delay
        ["delay.publish"] = new(
            "Capture and encode",
            "How long the publishing machine held a frame between reading it off the screen and having it encoded and ready to send. It is the one stage a faster encoder preset or a shorter lookahead shortens, and it is measured on the publishing pipeline rather than inferred from a setting."),
        ["delay.publish_link"] = new(
            "Publisher to relay",
            "The delivery window the publisher's leg settled on with the relay: every packet is held for this long so a lost one has room to arrive again. It is paid on every frame whether or not anything is lost, which is what makes it the largest stage on a healthy path."),
        ["delay.relay"] = new(
            "Through the relay",
            "What the relay spends terminating the incoming protocol and re-muxing the stream for this viewer. Never a figure: the relay states no per-path delay and no leg carries a relay timestamp to subtract, so the only honest reading is that it is unmeasured and that the total below is short by it."),
        ["delay.watch_link"] = new(
            "Relay to here",
            "The same delivery window on this leg, held by the transport before the pipeline is handed a packet at all. Only SRT states one; a leg that buffers inside the pipeline instead pays it under the two rows below."),
        ["delay.receive"] = new(
            "Decode",
            "How long this machine held a frame between the leg's source stamping it and the sink taking it: depacketizing, decoding and the queues in between. Rising to meet the latency window is a decode about to start dropping frames."),
        ["delay.present"] = new(
            "Waiting at the sink",
            "How long the sink still held each frame after it arrived, so that it was drawn at the moment the latency window puts it. It shrinks as the decode above grows, the two together being that window."),
        ["delay.total"] = new(
            "At least, end to end",
            "The stages above that were measured, added up. A floor and never the whole delay: the relay's own share is missing from it, and so are the publishing stages whenever the stream comes from another machine."),

        // section.audio
        ["audio_codec_description"] = new(
            "Codec",
            "The coding format the sound track is in, as the decoder identifies it."),
        ["audio_decoder"] = new(
            "Decoder",
            "The element decoding the sound track."),
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
            "What the sound track is arriving at, measured over the last second."),
        ["audio_bytes"] = new(
            "Audio received",
            "Everything the sound track has taken in since this decode opened."),

        // section.window
        ["window.size"] = new(
            "Handed over at",
            "The size of the frames this window is being given. It is the sink's output as the window sees it, and it is what a marker drawn over the picture is positioned against."),
        ["window.frames"] = new(
            "Frames taken",
            "How many frames this window has taken since it subscribed. It counts what arrived here, not what was drawn: a window behind the compositor takes frames it never puts on screen."),
        ["window.dropped"] = new(
            "Dropped waiting for this window",
            "Frames the backend discarded because this window was holding every slot of the pool it lends. It is the evidence that the drawing side is the slow half, and it is the one figure on this panel the backend cannot measure."),

        // srtsrc
        ["packets-received"] = new(
            "Packets",
            "Everything that arrived on the link, retransmits included. It is the denominator the three counters under it are read against."),
        ["packets-received-lost"] = new(
            "Lost",
            "Packets that never arrived at all, after retransmission had its chance. Anything above zero here is picture that cannot be reconstructed, which is what blocking and smearing are."),
        ["packets-received-retransmitted"] = new(
            "Retransmitted",
            "Packets that arrived only because the sender was asked again. They are the link working as intended: a count that climbs steadily with nothing lost beside it is a lossy path SRT is covering for."),
        ["packets-received-dropped"] = new(
            "Dropped",
            "Packets that arrived too late to be played and were thrown away. Raising the SRT latency setting trades delay for fewer of these."),
        ["receive-rate-mbps"] = new(
            "Receive rate",
            "What the link is delivering, as SRT measures it. It counts everything on the wire, so it sits above the video bitrate by the protocol's own overhead."),
        ["bandwidth-mbps"] = new(
            "Estimated capacity",
            "What SRT believes the path between the relay and here can carry. A stream whose bitrate approaches it is a stream that will start losing packets."),
        ["rtt-ms"] = new(
            "Round trip",
            "How long a packet takes to reach the relay and come back. It bounds how fast a retransmit can possibly arrive, so the latency window has to be several times this to be worth having."),
        ["negotiated-latency-ms"] = new(
            "Buffer",
            "The delay the two ends agreed to hold packets for, which is how long SRT has to notice a gap and fill it. The relay enforces a floor on this, so asking for less than the floor changes nothing."),

        // rtpjitterbuffer
        ["num-pushed"] = new(
            "Pushed",
            "Packets handed on to the decoder in the right order, which is the buffer doing its job."),
        ["num-lost"] = new(
            "Lost",
            "Packets the buffer gave up waiting for. They are gone from the picture, and every one of them is a hole the decoder has to conceal."),
        ["num-late"] = new(
            "Late",
            "Packets that turned up after their play time had passed. The buffer cannot use them, so a count that climbs means the buffer is too short for this path."),
        ["num-duplicates"] = new(
            "Duplicates",
            "Packets that arrived more than once. A few are normal where retransmits cross with the original; many mean something on the path is echoing traffic."),
        ["rtx-count"] = new(
            "Retransmits asked for",
            "How many times the buffer noticed a gap and asked the sender to send the packet again."),
        ["rtx-success-count"] = new(
            "Retransmits recovered",
            "How many of those requests arrived in time to be used. The difference between this and the count above is the recovery that did not make it."),
    };

    public static Counter Heading(string id) => Look(Headings, id);

    public static Counter Element(string factory) => Look(Elements, factory);

    public static Counter Field(string key) => Look(Fields, key);

    /// <summary>
    /// The entry for an identifier, falling back to the identifier itself with nothing said about it.
    /// A counter this build has no words for is still one the backend sends, and a row printing its raw key
    /// is one a reader can search for and report.
    /// </summary>
    private static Counter Look(Dictionary<string, Counter> table, string id) =>
        id.Length > 0 && table.TryGetValue(id, out var entry) ? entry : new Counter(id, "");
}
