namespace ScreenShare.App.Copy;

/// <summary>
/// What each control is called, what it teaches, and where to read more.
/// The backend sends a field as a key, <c>bitrate_mbps</c>, <c>capture_memory</c>,
/// and this turns that key into a heading and a paragraph.
/// Which controls exist, in what order and which are reachable is the backend's answer
/// (<c>docs/ipc-api.md</c>).
///
/// Help answers three things in this order: what the control does, what moving it costs, what to do about it.
/// A control whose help only expands its own label is a control left without help.
///
/// A key with no entry renders with the key as its heading and no paragraph, a defect left visible.
/// </summary>
public static class Fields
{
    /// <summary>
    /// What a control the backend marked live costs to change on a running stream.
    /// Sits beside the label in a chip's width, so it stays short.
    /// Which controls carry it is the backend's answer, the wording this side's.
    /// </summary>
    public const string LiveNotice = "applies without reconnecting";

    /// <summary>
    /// Names where a choice control's ruled-out entries are rather than what pressing the disclosure does,
    /// like every other row carrying a state (<c>docs/design-language.md</c>, "Menus").
    /// </summary>
    public const string RefusedTitle = "Unavailable options";

    /// <summary>
    /// Names where a step's folded controls are, on the same terms as <see cref="RefusedTitle"/>:
    /// the presets and the shipped defaults already cover them.
    /// </summary>
    public const string AdvancedTitle = "Advanced options";

    /// <summary>Entries held back, beside a disclosure: the figure says whether opening it is worth the trip.</summary>
    public static string OptionCount(int count) => count == 1 ? "1 option" : $"{count} options";

    /// <summary>
    /// One control's copy.
    /// <c>Doc</c> is the article for the concept, where one exists.
    /// </summary>
    public sealed record Entry(string Label, string Help, string Doc = "");

    /// <summary>
    /// One group's copy.
    /// A group heading is a wizard step name too, so it stays short enough for a chip.
    /// </summary>
    public sealed record GroupEntry(string Title, string Help);

    private const string DocChroma = "https://en.wikipedia.org/wiki/Chroma_subsampling";
    private const string DocYCbCr = "https://en.wikipedia.org/wiki/YCbCr";
    private const string DocBitrate = "https://en.wikipedia.org/wiki/Variable_bitrate";
    private const string DocQuantization = "https://en.wikipedia.org/wiki/Quantization_(image_processing)";
    private const string DocVbv = "https://en.wikipedia.org/wiki/Video_buffering_verifier";
    private const string DocGop = "https://en.wikipedia.org/wiki/Group_of_pictures";
    private const string DocNvenc = "https://en.wikipedia.org/wiki/Nvidia_NVENC";
    private const string DocEncoderTuning = "https://en.wikipedia.org/wiki/X264";
    private const string DocDrm = "https://en.wikipedia.org/wiki/Direct_Rendering_Manager";
    private const string DocDrmPrime = "https://en.wikipedia.org/wiki/Direct_Rendering_Manager#DRM_PRIME";
    private const string DocSrt = "https://en.wikipedia.org/wiki/Secure_Reliable_Transport";
    private const string DocRtsp = "https://en.wikipedia.org/wiki/Real-Time_Streaming_Protocol";

    private static readonly Dictionary<string, Entry> Entries = new()
    {
        ["relay.tls"] = new(
            "Relay uses TLS",
            "Whether the relay answers on one address behind a certificate, or directly on the ports below. It follows the relay address rather than being set here. A relay on this computer or the local network is reached directly. Anything further away is always encrypted."),

        ["relay.discord_mode"] = new(
            "Follow Discord",
            "Ties the group to the voice channel this computer's linked Discord account is in. "
            + "Joining a channel joins its group, and whoever leaves the channel can no longer watch. "
            + "The group key and the name come from the channel while this is on."),

        ["relay.group_key"] = new(
            "Group key",
            "The secret that decides who can watch. Everyone holding it sees your streams, and nobody else does. Share it like a meeting link. Setting a key puts this computer in that group, and clearing the field takes it out. Change it to cut someone off. Sharing needs one, so nothing is published while this is empty."),

        ["relay.display_name"] = new(
            "Name in the group",
            "What this computer goes by in the group: its row in the member list, and the name beside each stream it publishes. The first claim on a name holds it, so a name another member uses cannot be taken. Sharing needs one, so nothing is published while this is empty."),

        ["relay.host"] = new(
            "Relay address",
            "The computer running the relay. This computer pushes to it, and every viewer pulls from it, so both sides need to reach it. A LAN address works for a LAN stream. Anyone further away needs a server with a public address."),

        ["publish.capture"] = new(
            "How to capture",
            "How frames leave the desktop. Set this first. It fixes which encoder software runs, so almost everything below follows from it. Prefer the system's own: Desktop Duplication on Windows, the screen picker on Wayland."),

        ["publish.monitor"] = new(
            "Which screen",
            "The screen to share. Only what it shows is sent. Windows on other screens stay private."),

        ["publish.output_resolution"] = new(
            "Size sent",
            "The size the encoder is fed. Leave it at the screen's own size to send exactly what the screen shows. Sending smaller costs sharpness and saves everything at once: encoding, upload, and decoding. The most effective setting when the connection is the problem."),

        ["publish.fps"] = new(
            "Frame rate",
            "How many pictures a second. Higher is smoother and costs proportionally more upload and encoding. Above the screen's refresh rate the extra frames are copies. They cost bandwidth and add no smoothness.",
            ""),

        ["publish.capture_memory"] = new(
            "Where frames travel",
            "Whether frames stay on the graphics card on the way to the encoder, or pass through main memory. Staying on the card is free, but only works when capture and encoder can share it. Automatic is right for almost everyone.",
            DocDrmPrime),

        ["publish.cursor"] = new(
            "Mouse pointer",
            "Whether the pointer appears in what viewers see. Drawn into the picture is the usual screen-share look, at a little bandwidth for the area it moves through. Not shared leaves it out. Sent beside the picture costs nothing, but only the local preview draws it. Viewers receive the picture without a pointer."),

        ["publish.drm_map"] = new(
            "Frame download route",
            "How scanout frames reach main memory. They are stored in a GPU-specific layout, so a device that understands it has to read them back. Leave it automatic unless the capture does not start.",
            DocDrm),

        ["publish.format"] = new(
            "Video format",
            "How the picture is compressed. Every viewer has to decode it. The newer formats need fewer bits and are decoded by fewer devices. H.264 plays everywhere."),

        ["publish.encoder"] = new(
            "Encoded by",
            "What produces that format on this computer. A graphics card encodes on a chip of its own, so a stream costs almost nothing while a game runs. The CPU encoders compress harder and use the cores for it."),

        ["publish.chroma"] = new(
            "Color detail",
            "How much color information is kept. Video normally throws away three quarters of it. Nobody notices in a film, everyone notices in colored text. Keeping more costs bits and narrows which GPUs decode it. Everything still plays, on the CPU if nothing else.",
            DocChroma),

        ["publish.color_range"] = new(
            "Color range",
            "Which code values carry picture. A desktop is full range, and broadcast video is not. A mismatch makes the stream look washed out or crushed at the other end.",
            DocYCbCr),

        ["publish.effort"] = new(
            "Encoder effort",
            "How hard the encoder looks for savings. More effort means a smaller stream at the same quality, paid in encoding time. On a graphics card that time is nearly free. On the CPU it competes with everything else this computer runs. The steps are the encoder's own, so they read differently on each.",
            DocNvenc),

        ["publish.tune"] = new(
            "What to tune for",
            "What the encoder aims at while it spends that effort. Low-latency tuning drops the tricks that need future frames, keeping a live picture close to the moment it happened. The quality tunings keep them and send a smaller, better stream a fraction of a second behind. The rest describe the picture itself, such as film grain or flat animation.",
            DocEncoderTuning),

        ["publish.mode"] = new(
            "What to hold steady",
            "Whether the encoder holds a bitrate or a quality. The decision behind everything else in this step. Hold a bitrate when the connection has a known limit. Hold a quality when it does not, and let the rate follow the picture.",
            DocBitrate),

        ["publish.cq"] = new(
            "Quality target",
            "How much detail the encoder may discard. Lower keeps more and costs more, higher is smaller and softer. Around 20 is visually clean for a desktop, around 30 starts to show. The scale is the encoder's own, so the same number means a different quality on another one.",
            DocQuantization),

        ["publish.bitrate_mbps"] = new(
            "Bitrate",
            "How much bandwidth the stream aims at. Set it below what the connection reliably uploads, not at it. A stream that fills the line has no room for motion."),

        ["publish.maxrate_mbps"] = new(
            "Burst ceiling",
            "The most bandwidth the stream may use when the picture moves. The rate rises to here on motion and falls back on a still screen. Set it below what the line carries, and above the target where there is one. In constant quality, zero means no ceiling."),

        ["publish.vbv_ms"] = new(
            "Rate buffer",
            "How many milliseconds of slack the encoder has to even out bandwidth. Smaller holds the rate tighter and adds less delay. Larger keeps quality steadier across bursts. Zero keeps the encoder's own default.",
            DocVbv),

        ["publish.gop"] = new(
            "Keyframe interval",
            "How many frames between complete pictures. A viewer cannot start until one arrives. A long interval saves bandwidth, slows joining, and makes packet loss last longer on screen. Zero means twice the frame rate, a good default.",
            DocGop),

        ["publish.bframes"] = new(
            "Look-ahead frames",
            "Frames that also reference the future. They save bandwidth and add delay in equal measure. Zero is right for anything interactive.",
            DocGop),

        ["publish.audio_sources[].source"] = new(
            "Source",
            "Where one row of the mix comes from: everything this computer plays, or one program's own sound. Picking no audio on a row removes it from the list."),

        ["publish.audio_sources[].device"] = new(
            "Device",
            "Which output or program this row records, where more than one exists. The default follows the system setting, so it keeps working when a headset is plugged in."),

        ["publish.audio_sources[].gain"] = new(
            "Level",
            "How loud this source is in the mix. Above 100 amplifies a quiet source. It reaches a running stream, so the mix can be balanced while people watch."),

        ["publish.audio_sources[].mute"] = new(
            "Mute",
            "Silences this source without removing it, keeping its device and level for later. It reaches a running stream."),

        ["publish.audio_codec"] = new(
            "Audio format",
            "What compresses the sound. Opus unless something on the other end insists otherwise. It has lower delay, and browsers negotiate only it."),

        ["publish.publish_transport"] = new(
            "How to send",
            "The protocol carrying the stream to the relay. Viewers pick their own protocol back, so a stream sent over SRT can be watched over RTSP. The protocols differ in loss handling and tunable delay."),

        ["publish.srt_publish_latency_ms"] = new(
            "Retransmit window, sending",
            "How long the relay waits for a lost packet to arrive again. Longer survives a worse connection, at the cost of delay. This window and the viewer's add up. The relay enforces at least 120 ms, so anything lower is raised to that.",
            DocSrt),

        ["publish.rtsp_publish_protocol"] = new(
            "RTSP transport, sending",
            "How the media travels inside the RTSP session on its way out. TCP needs nothing beyond the connection already open. UDP needs a port pair to get out too. A home router normally allows that, a corporate network often does not.",
            DocRtsp),

        ["publish.uplink_mbps"] = new(
            "Upload speed",
            "What the connection actually uploads, not what the plan says. The prediction is weighed against it, and the Balanced preset prices its bitrate from a measured figure. The first start measures it once in the background, uploading 20 MB to a public test endpoint (speed.cloudflare.com). Measure again after switching networks."),

        ["viewer.srt_watch_latency_ms"] = new(
            "Retransmit window, watching",
            "How long this machine waits for a lost packet to arrive again on the way back from the relay. Most loss happens on this leg. It is delay you see, so raise it on a distant or lossy connection and lower it on a local network. The relay enforces at least 120 ms, so anything lower is raised to that.",
            DocSrt),

        ["viewer.rtsp_watch_protocol"] = new(
            "RTSP transport, watching",
            "How the media travels inside the RTSP session on the way back. TCP carries both tracks on the connection the player made, which restrictive networks usually allow. UDP is lower delay and needs the viewer's router to let it through.",
            DocRtsp),

        ["viewer.tile_watch_transport"] = new(
            "How tiles watch",
            "The protocol a tile in this window receives on. A tile can take WebRTC, which no external player opens. A tile on HLS plays the picture without sound, because the relay serves the two separately. A stream the protocol cannot carry is refused when you open it, naming the ones that can."),

        ["viewer.rtsp_watch_latency_ms"] = new(
            "Reorder window, tiles",
            "How long a tile holds RTSP packets for late or out-of-order ones before playing what it has. The delay is the tile's alone. An external player reorders by count rather than time.",
            DocRtsp),

        ["viewer.render_chain"] = new(
            "How frames are converted",
            "What turns decoded frames into the picture a tile draws. The graphics-card routes convert for free on the card. The system-memory route pulls every frame across and converts on the CPU. Some routes state their color exactly, some leave it to the driver. A route this computer has no elements for is grayed with the missing one named."),

        ["relay.srt_port"] = new("SRT port", "The relay's SRT port. The default is 8890."),
        ["relay.rtsp_port"] = new("RTSP port", "The relay's RTSP port. On an encrypted relay the session rides inside TLS, so the default is 8322 rather than the cleartext port."),
        ["relay.webrtc_port"] = new("WebRTC port", "The relay's WebRTC port, which serves both sending and watching. The default is 8889."),
        ["relay.rtmp_port"] = new("RTMP port", "The relay's RTMP port. On an encrypted relay the stream rides inside TLS, so the default is 1936 rather than the cleartext port."),
        ["relay.hls_port"] = new("HLS port", "The relay's HLS port. The default is 8888. A watching port only."),
        ["relay.moq_port"] = new("MoQ port", "The relay's Media-over-QUIC port, TCP and UDP on the same number. The default is 8892. A watching port only, and only a browser reaches it. It stays part of the address on an encrypted relay, because this leg cannot pass the proxy and the relay answers it directly."),
    };

    private static readonly Dictionary<string, GroupEntry> Groups = new()
    {
        ["source"] = new(
            "Capture",
            "Which screen is shared and how the frames reach the encoder. The capture method fixes which encoder software runs, so the rest of the form follows from it."),

        ["quality"] = new(
            "Encode",
            "What compresses the picture and how it spends bandwidth. Everything here follows the capture method chosen above. An entry grayed with an encoder's name asks for a different capture, not a different encoder."),

        ["audio"] = new(
            "Audio",
            "The sound track: where it comes from and what compresses it. Every source listed is mixed into the one track viewers hear. Which sources exist depends on the operating system. Which formats reach the relay depends on the sending protocol."),

        ["transport"] = new(
            "Sending",
            "How the stream travels from this computer to the relay, and what that protocol leaves to tune. Viewers pick their own protocol regardless."),

        ["watch"] = new(
            "Watching",
            "How a stream comes back from the relay and how it is decoded. It covers every stream this window watches, anyone's as well as this computer's. External players and browser pages are not set here. Right-click a stream in the list to pick their protocol. A tile already on screen keeps its settings, so a change here reaches the next one."),

        ["relay"] = new(
            "Relay",
            "Which computer carries the stream, and which port each of its listeners uses. The ports default to the relay's own, so they only matter for a relay set up differently."),
    };

    /// <summary>
    /// Copy for one field key, falling back to the key itself,
    /// which draws a control the reader can at least identify and report.
    /// </summary>
    public static Entry Of(string key) =>
        key.Length > 0 && Entries.TryGetValue(Template(key), out var entry) ? entry : new Entry(key, "");

    /// <summary>
    /// Control a key names, with a list index taken out:
    /// <c>publish.audio_sources[2].gain</c> is a value of <c>publish.audio_sources[].gain</c>.
    /// Copy is written for the control,
    /// matching the normalization the backend does in its own tables (backend/internal/form/keys.go).
    /// </summary>
    public static string Template(string key)
    {
        var open = key.IndexOf('[');
        var close = key.IndexOf(']');
        return open < 0 || close < open ? key : key[..(open + 1)] + key[close..];
    }

    /// <summary>Copy for one group key, falling back to the key.</summary>
    public static GroupEntry Group(string key) =>
        key.Length > 0 && Groups.TryGetValue(key, out var entry) ? entry : new GroupEntry(key, "");

    /// <summary>
    /// How a unit is written after its figure.
    /// Empty where the unit is unspecified, the case for a figure carrying none.
    /// </summary>
    public static string Unit(Api.V1.Unit unit) => unit switch
    {
        Api.V1.Unit.MegabitsPerSecond => "Mbit/s",
        Api.V1.Unit.Milliseconds => "ms",
        Api.V1.Unit.FramesPerSecond => "fps",
        Api.V1.Unit.Frames => "frames",
        Api.V1.Unit.Percent => "%",
        _ => "",
    };
}
