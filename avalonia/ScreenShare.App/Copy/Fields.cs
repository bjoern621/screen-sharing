namespace ScreenShare.App.Copy;

/// <summary>
/// What each control is called, what it teaches, and where to read more.
/// The backend sends a field as a key, <c>bitrate_mbps</c>, <c>capture_memory</c>, and this turns that key into
/// a heading and a paragraph.
/// Which controls exist, in what order and which are reachable is the backend's answer and is not restated here
/// (docs/ipc-api.md).
///
/// Help answers three things in this order: what the control does, what moving it costs, what to do about it.
/// A control whose help only expands its own label has been left without help.
///
/// A key with no entry renders with the key as its heading and no paragraph, a defect left visible rather than
/// swallowed.
/// </summary>
public static class Fields
{
    /// <summary>
    /// What a control the backend marked live costs to change on a running stream.
    /// Sits beside the label in a chip's width, so it stays short.
    /// Which controls carry it is the backend's answer; the wording is this side's.
    /// </summary>
    public const string LiveNotice = "applies without reconnecting";

    /// <summary>
    /// Names where a choice control's ruled-out entries are rather than what pressing the disclosure does, like
    /// every other row carrying a state (<c>docs/design-language.md</c>, "Menus").
    /// </summary>
    public const string RefusedTitle = "Unavailable options";

    /// <summary>Entries held back, beside the disclosure: the figure says whether opening it is worth the trip.</summary>
    public static string RefusedCount(int count) => count == 1 ? "1 option" : $"{count} options";

    /// <summary>One control's copy. <c>Doc</c> is the article for the concept, where one exists.</summary>
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
        ["publish.name"] = new(
            "Stream name",
            "The name viewers open. It becomes the last part of the address they are sent, so keep it short and free of spaces."),

        ["relay.tls"] = new(
            "Relay uses TLS",
            "Whether the relay answers everything on one address behind a certificate, or directly on the ports below. It follows the relay address rather than being set here: a relay on this machine or the local network is reached directly, and anything further away is encrypted with no way to turn that off."),

        ["relay.group_key"] = new(
            "Group key",
            "The secret that decides who can watch. Everyone holding it sees this machine's streams and nobody else does, so send it the way a meeting link is sent. Change it when somebody should stop seeing them. Left empty, these streams go out where anyone who knows the relay address can watch them."),

        ["relay.display_name"] = new(
            "Name in the group",
            "What this machine goes by to everyone in the group: the row that is this machine in the member list, and the name beside each stream it publishes. A name is claimed per group and the first claim holds it, so one another member already goes by is refused and this machine keeps the identity it had either way. Joining a group takes one."),

        ["relay.srt_passphrase"] = new(
            "SRT passphrase",
            "What scrambles the packets on the SRT leg. It is the one way out that cannot be wrapped in the usual encryption, so this is what protects it, and the relay operator sets the same value at their end. Leave it empty if the relay asks for none."),

        ["relay.host"] = new(
            "Relay address",
            "The machine running the relay. This machine pushes to it and everyone watching pulls from it, so it needs to be reachable from both sides: a machine on the same network for a LAN stream, a server with a public address for anyone further away."),

        ["publish.capture"] = new(
            "How to capture",
            "How frames leave the desktop. This is the first choice to get right: it also fixes which encoder software runs, so almost everything below follows from it. Prefer the one this system is built around: Desktop Duplication on Windows, the screen picker on a Wayland desktop."),

        ["publish.monitor"] = new(
            "Which screen",
            "The monitor to share. Only what this screen shows is sent; windows on the other screens stay private."),

        ["publish.output_resolution"] = new(
            "Size sent",
            "The size the encoder is fed. Leave it at the source to send exactly what the screen shows. Sending smaller costs sharpness and saves everything at once: fewer bits to encode, to upload, and for viewers to decode. The single most effective knob when the connection is the problem."),

        ["publish.fps"] = new(
            "Frame rate",
            "How many pictures a second. Higher is smoother and costs proportionally more upload and more encoding. Above the monitor's own refresh rate the extra frames are duplicates: they cost bandwidth and buy no smoothness.",
            ""),

        ["publish.capture_memory"] = new(
            "Where frames travel",
            "Whether the frames stay on the graphics card on their way to the encoder or take a trip through main memory. Staying on the card is free but only possible when the capture and the encoder can share it, so this follows both choices above. Automatic is right unless the last of the CPU is worth chasing.",
            DocDrmPrime),

        ["publish.cursor"] = new(
            "Mouse pointer",
            "Whether the pointer appears in what viewers see. Drawn into the picture is what a screen share normally looks like, and it costs a little bandwidth because the encoder has to keep redrawing the area it moves through. Not shared leaves it out entirely. Sent beside the picture keeps it sharp at any size and costs the encoder nothing. The preview draws it from that position, and the picture viewers receive carries no pointer."),

        ["publish.drm_map"] = new(
            "Frame download route",
            "How scanout frames are brought into main memory. They are usually stored in a GPU-specific layout, so they have to be read back through a device that understands it. Leave it automatic unless the capture fails to start.",
            DocDrm),

        ["publish.format"] = new(
            "Video format",
            "How the picture is compressed, which is what every viewer has to decode and what the protocol carries. The newer formats send the same picture in fewer bits and are decoded by fewer devices; H.264 plays everywhere."),

        ["publish.encoder"] = new(
            "Encoded by",
            "What produces that format on this machine. A graphics card encodes on a chip of its own, so a stream costs almost nothing while a game is running. The CPU encoders squeeze harder for the same picture and charge the machine for it."),

        ["publish.chroma"] = new(
            "Colour detail",
            "How much colour information is kept. Video normally throws away three quarters of it, which nobody notices in a film and everyone notices in coloured text. Keeping more costs bits and narrows which viewers can decode it on their GPU. Every format plays everywhere, on the CPU if nothing else.",
            DocChroma),

        ["publish.color_range"] = new(
            "Colour range",
            "Which code values carry picture. A desktop is full range; broadcast video is not. Getting this wrong is what makes a stream look washed out or crushed at the other end.",
            DocYCbCr),

        ["publish.effort"] = new(
            "Encoder effort",
            "How hard the encoder looks for savings. More effort means a smaller stream at the same quality, paid for in encoding time. On a graphics card that time is nearly free, because the chip doing it is separate from the graphics cores, and on the CPU it is the cores this machine also needs for everything else. The steps are the encoder's own, so they read differently on each one.",
            DocNvenc),

        ["publish.tune"] = new(
            "What to tune for",
            "What the encoder aims at while it spends that effort. Low-latency tuning drops the tricks that need future frames, which is what keeps a live picture close to the moment it happened; the quality tunings keep them and code a smaller, better stream a fraction of a second behind. The rest describe the picture itself, so the encoder stops treating film grain or flat animation as detail worth keeping.",
            DocEncoderTuning),

        ["publish.mode"] = new(
            "What to hold steady",
            "The one decision behind everything else in this step: whether the encoder holds a bandwidth or holds a quality. Hold bandwidth when the connection has a known limit. Hold quality when it does not, and let the bitrate go where the picture takes it.",
            DocBitrate),

        ["publish.cq"] = new(
            "Quality target",
            "How much detail the encoder is allowed to discard. Lower keeps more and costs more; higher is smaller and softer. Around 20 is visually clean for a desktop and around 30 starts to show. The scale belongs to the encoder, so the same number is a different quality on a different one, and the range moves with the choice above.",
            DocQuantization),

        ["publish.bitrate_mbps"] = new(
            "Bitrate",
            "How much bandwidth the stream aims at. Set it below what the connection reliably uploads, not at it: a stream that fills the line has nothing left for the moments the picture moves."),

        ["publish.maxrate_mbps"] = new(
            "Burst ceiling",
            "The most bandwidth the stream may take when the picture moves. It rises to here on motion and falls back on a still screen, which is what keeps quality from dipping. Set it below what the line can carry, and above the target where there is one. In constant quality there is no target and zero means no ceiling at all."),

        ["publish.vbv_ms"] = new(
            "Rate buffer",
            "How many milliseconds of slack the encoder has to even bandwidth out. Smaller holds the rate tighter and adds less delay; larger keeps quality steadier across bursts. Zero leaves the encoder's own default alone.",
            DocVbv),

        ["publish.gop"] = new(
            "Keyframe interval",
            "How many frames between complete pictures. A viewer cannot start until one arrives, so a long interval saves bandwidth and makes joining slower, and makes packet loss last longer on screen. Zero means twice the frame rate, which is a good default.",
            DocGop),

        ["publish.bframes"] = new(
            "Look-ahead frames",
            "Frames that also reference the future, which saves bandwidth and adds delay in exact proportion. Useful when sending to a viewer rather than playing with one; zero is right for anything interactive.",
            DocGop),

        ["publish.audio_sources[].source"] = new(
            "Source",
            "Where one row of the mix comes from: everything this machine plays, or one program's own sound. Picking no audio on a row takes it off the list."),

        ["publish.audio_sources[].device"] = new(
            "Device",
            "Which output or which program this row records, where this machine offers more than one. The default follows whatever this system is set to, so it keeps working when a headset is plugged in."),

        ["publish.audio_sources[].gain"] = new(
            "Level",
            "How loud this source is in the mix, relative to what it produces on its own. Above 100 amplifies, which is what a quiet source needs and what makes everything else quieter by comparison. It reaches a running stream, so the mix can be balanced while people are watching."),

        ["publish.audio_sources[].mute"] = new(
            "Mute",
            "Silences this source without taking it off the list, so its device and its level are still there when it is turned back on. It reaches a running stream."),

        ["publish.audio_codec"] = new(
            "Audio format",
            "What compresses the sound. Opus unless something on the other end insists otherwise. It is lower delay and the only one browsers negotiate."),

        ["publish.publish_transport"] = new(
            "How to send",
            "The protocol carrying the stream from here to the relay. This is only the way out: viewers pick their own way back, so a stream sent over SRT can still be watched over RTSP. They differ in how they handle loss and in how much delay can be tuned away."),

        ["publish.srt_publish_latency_ms"] = new(
            "Retransmit window, sending",
            "How long the relay waits for a packet to arrive again before giving up on it. Longer survives a worse connection at the cost of delay. This window and the viewer's add up, and the relay asks for at least 120 ms, so anything lower is raised to that.",
            DocSrt),

        ["publish.rtsp_publish_protocol"] = new(
            "RTSP transport, sending",
            "How the media travels inside the RTSP session on its way out. TCP needs nothing beyond the connection already open. UDP needs a port pair to get out too: a home router normally allows that, a corporate network often does not.",
            DocRtsp),

        ["publish.uplink_mbps"] = new(
            "Upload speed",
            "What this connection actually uploads, not what the plan says. Nothing is enforced against it: it is what the prediction is weighed against, so a configuration this connection cannot carry says so here rather than at the viewers. Measure it if it is not known."),

        ["viewer.srt_watch_latency_ms"] = new(
            "Retransmit window, watching",
            "The same window on the viewer's side, where most internet loss actually happens. It is delay they see: a distant viewer on a poor connection wants more, a viewer on the local network wants less.",
            DocSrt),

        ["viewer.rtsp_watch_protocol"] = new(
            "RTSP transport, watching",
            "How the media travels inside the RTSP session on the way back. TCP carries both tracks on the connection the player already made, which is what a restrictive network is likeliest to allow. UDP is lower delay and needs the viewer's router to let it through.",
            DocRtsp),

        ["viewer.tile_watch_transport"] = new(
            "How tiles watch",
            "The protocol a tile in this window receives on. It says nothing about an external player or a browser page: those open on the protocol picked by right-clicking a stream, and they reach a different set: a tile can take WebRTC, which no player opens by address. A tile on HLS plays the picture without the sound, the relay serving the two separately and only the picture being readable here."),

        ["viewer.rtsp_watch_latency_ms"] = new(
            "Reorder window, tiles",
            "How long a tile holds RTSP packets waiting for late or out-of-order ones before playing what it has. It is delay seen here, and it is the tile's alone: an external player reorders by count rather than by time.",
            DocRtsp),

        ["viewer.render_chain"] = new(
            "How frames are converted",
            "What turns decoded frames into the picture a tile draws. The graphics-card routes keep the frames on the card and cost nothing to convert; the system-memory route pulls every frame across and converts it on the CPU. Some of them state exactly what colour they produce and some leave it to the driver, which is what the entries say. A route this machine has no elements for is greyed with the missing one named."),

        ["relay.srt_port"] = new("SRT port", "The relay's SRT port. The default is 8890."),
        ["relay.rtsp_port"] = new("RTSP port", "The relay's RTSP port. It carries the session inside TLS, which is why the default is 8322 rather than the number a cleartext relay answers on."),
        ["relay.webrtc_port"] = new("WebRTC port", "The relay's WebRTC port, which serves both sending and watching. The default is 8889."),
        ["relay.rtmp_port"] = new("RTMP port", "The relay's RTMP port. It carries the stream inside TLS, which is why the default is 1936 rather than the number a cleartext relay answers on."),
        ["relay.hls_port"] = new("HLS port", "The relay's HLS port. The default is 8888. It is a watching port only: nothing is ever sent this way."),
        ["relay.moq_port"] = new("MoQ port", "The relay's Media-over-QUIC port, TCP and UDP on the same number. The default is 8892. It is a watching port only, and only a browser reaches it. It stays part of the address on an encrypted relay, where the other ports drop out: this leg cannot go through the proxy, so the relay answers it directly wherever it runs."),
        ["relay.api_port"] = new("Relay API port", "The relay's status port, which is where the live-now list comes from. The default is 9997."),
    };

    private static readonly Dictionary<string, GroupEntry> Groups = new()
    {
        ["stream"] = new(
            "Stream",
            "What this stream is called. It is the one setting viewers see, and it is the last part of the address they are sent."),

        ["source"] = new(
            "Capture",
            "Which screen is shared and how the frames reach the encoder. The capture method also fixes which encoder software runs, which is why the rest of the form follows from it."),

        ["quality"] = new(
            "Encode",
            "What compresses the picture and how it spends bandwidth. Everything here is offered against the capture method chosen above, so an entry greyed out with an encoder's name names the capture to change rather than the encoder."),

        ["audio"] = new(
            "Audio",
            "The sound track: where it comes from and what compresses it. Every source listed here is mixed into the one track viewers hear. Which sources exist depends on this operating system; which formats reach the relay depends on the way out."),

        ["transport"] = new(
            "Sending",
            "How the stream travels from this machine to the relay, and what that protocol leaves to tune. Nothing here limits what viewers can watch over."),

        ["watch"] = new(
            "Watching",
            "How a stream comes back from the relay and how it is decoded once it does: anyone's stream, not only this machine's. Separate from the way out. An external player and a browser page are not set here: right-click a stream in the list and the leg picked there opens it. A tile already on screen keeps the settings it was opened with, so a change here reaches the next one."),

        ["relay"] = new(
            "Relay",
            "Which machine carries the stream, and which port each of its listeners uses. The ports default to the relay's own, so they only matter against a relay someone set up differently."),
    };

    /// <summary>
    /// Copy for one field key, falling back to the key itself, which draws a control the reader can at least
    /// identify and report.
    /// </summary>
    public static Entry Of(string key) =>
        key.Length > 0 && Entries.TryGetValue(Template(key), out var entry) ? entry : new Entry(key, "");

    /// <summary>
    /// Control a key names, with a list index taken out: <c>publish.audio_sources[2].gain</c> is a value of
    /// <c>publish.audio_sources[].gain</c>.
    /// Copy is written for the control and never for one entry of a list, the normalisation the backend does in
    /// its own tables (backend/internal/form/keys.go).
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
