namespace ScreenShare.App.Copy;

/// <summary>
/// The paragraph behind one choice: what it is, what it costs, and when to pick it.
/// Shown under a radio card's title, and in a dropdown entry where there is room (docs/tooltips.md).
///
/// Three rules hold every entry.
/// Say the trade and not the definition: "4:2:0 keeps a quarter of the colour" is the definition, "coloured
/// text and edges smear" is what the reader will see, and both belong in that order.
/// Name what a choice needs before it is picked, since the backend greys only what it can prove.
/// An empty answer is allowed for a value whose name already says everything, and the row then draws one line
/// instead of two.
/// </summary>
public static class Descriptions
{
    private static readonly Dictionary<string, string> Captures = new()
    {
        ["ddagrab"] = "Windows' own screen-capture path, taken on the GPU, one monitor at a time. The best choice on Windows: it costs almost nothing and never touches the CPU to grab a frame.",
        ["gdigrab"] = "Copies the desktop with the CPU, and copies all monitors as one wide frame. Slower than Desktop Duplication and wide enough on a multi-monitor desktop to exceed what NVIDIA's encoder accepts. Use it only where the other Windows backends will not start.",
        ["d3d11screencapturesrc"] = "The same Windows capture surface, read by GStreamer instead of ffmpeg. Pick it to reach GStreamer's encoders and its WebRTC output on Windows.",
        ["x11grab"] = "Reads the X11 screen through shared memory. The default on Linux, and it sees XWayland windows but not a native Wayland desktop.",
        ["ximagesrc"] = "The same X11 screen, read by GStreamer instead of ffmpeg. Pick it to reach GStreamer's encoders on an X11 session.",
        ["kmsgrab"] = "Grabs the frames the GPU is already scanning out to the monitor, below the compositor. The cheapest capture there is, and the only one that needs elevated privileges: the app either holds that permission or the capture stops at launch, and nothing can tell which in advance.",
        ["portal"] = "Asks the desktop to show its own picker, so you choose the monitor or window and the desktop consents. The one that works on Wayland, and it works on X11 too. Fewer pixel formats and rate-control knobs than the ffmpeg backends - the fields say so where it matters.",
        ["avfoundation"] = "macOS screen capture, by screen index. Desktop audio is out of reach here: macOS offers no system-audio input device.",
        ["avfvideosrc"] = "The same macOS screen, read by GStreamer instead of ffmpeg. The element picks the screen itself, so the monitor setting does not reach it.",
    };

    private static readonly Dictionary<string, string> Memories = new()
    {
        ["auto"] = "Keep the frames on the GPU where the capture and the encoder can share them, and copy through RAM where they cannot. The one setting that works with every combination.",
        ["gpu"] = "Hand the encoder the frames the capture already produced: no copy out, no CPU conversion, no copy back. It also demands the colour you chose, so it is refused where the pair has no shared path and where their shared path cannot convert.",
        ["gpu-encoder-color"] = "Keep the frames on the GPU even where nothing on the way can convert them: the encoder reads the captured surface as it is and picks the colour itself. The pixel format and colour range you chose stop applying - both fields grey and say what the encoder will send instead.",
        ["system"] = "Copy every frame out to RAM, convert it on the CPU, and hand it back. The path every combination has, and the right one when capture and encoding run on different GPUs.",
    };

    private static readonly Dictionary<string, string> DrmMaps = new()
    {
        ["auto"] = "Pick the mapping device from the capture GPU's driver: VAAPI on Intel and AMD, Vulkan elsewhere.",
        ["vaapi"] = "Map through VAAPI. Works where the driver exposes a VAAPI device, which is Intel and AMD.",
        ["vulkan"] = "Map through Vulkan, which every vendor implements. Use on NVIDIA, or where VAAPI is missing.",
        ["none"] = "Copy the frame with no mapping at all. Only correct for a plain, untiled framebuffer; anything else fails immediately.",
    };

    private static readonly Dictionary<string, string> Formats = new()
    {
        ["h264"] = "Plays everywhere - every browser, every phone, every player made this century. The least efficient of the modern formats, so it costs the most bits for the same picture.",
        ["hevc"] = "About 40% smaller than H.264 at the same quality. Browser playback is limited to Safari or an OS extension, so it is the better choice when you know what your viewers are using.",
        ["av1"] = "The most efficient per bit. Hardware encoders are recent: NVIDIA RTX 40 and up, Intel Arc, AMD RDNA3 and up.",
        ["vp9"] = "Royalty-free and plays in every browser except Safari. Efficiency sits between H.264 and HEVC.",
        ["vp8"] = "The older royalty-free format, carried everywhere WebRTC is. Roughly as efficient as H.264.",
    };

    private static readonly Dictionary<string, string> Families = new()
    {
        ["software"] = "Encodes on the CPU. Needs no particular graphics card and works everywhere - and at a full desktop resolution and a high frame rate it will use a large part of the machine, more so for the newer formats.",
        ["nvenc"] = "NVIDIA's dedicated encoder chip, which sits beside the graphics cores rather than using them: a stream costs almost nothing in game performance. Needs an NVIDIA card and its driver.",
        ["vaapi"] = "The shared Intel and AMD hardware encoder on Linux, and the GPU option on a machine without NVIDIA. Which formats a card encodes depends on its generation; all of them are 4:2:0 and none is lossless.",
        ["qsv"] = "Intel's own runtime for the same silicon VAAPI reaches. Intel graphics only, and which formats it encodes moves with the generation: VP9 from Ice Lake, AV1 from Arc.",
        ["amf"] = "AMD's own encoder runtime, driving the same chip VAAPI reaches on an AMD card. VAAPI covers more formats; AMF brings AMD's rate control, whose capped mode gives the burst ceiling a bitrate mode can aim at. x86-64 and ffmpeg only.",
        ["v4l2"] = "The kernel's encoders on ARM boards - Raspberry Pi and similar.",
        ["rkmpp"] = "Rockchip's media encoders, on RK35xx-class boards.",
        ["vulkan"] = "Encoding through the Vulkan driver, which every vendor implements: one path to NVIDIA, AMD and Intel silicon. Which formats it reaches depends on the driver; all of them are 4:2:0 and none is lossless. ffmpeg only.",
    };

    /// <summary>
    /// What one encoder adds beyond its format, for the formats more than one encoder here produces.
    /// </summary>
    private static readonly Dictionary<string, string> Encoders = new()
    {
        ["libaom-av1"] = "The AV1 reference encoder: the only software AV1 here that codes full colour and RGB, and the slowest of the three even in its realtime mode.",
        ["libsvtav1"] = "The fastest realtime AV1, which is what makes the format usable at a desktop resolution at all. 4:2:0 and 10-bit only.",
        ["librav1e"] = "Between the other two in speed, and reaches full colour and 10-bit. It takes one bitrate target with no ceiling and no buffer, and its quality scale counts to 255 rather than 51.",
    };

    private static readonly Dictionary<string, string> Chromas = new()
    {
        ["gbrp"] = "The desktop's own pixels, sent with no colour conversion at all. Text and thin lines arrive exactly as they look here. The heaviest option by a wide margin, and few GPUs decode it.",
        ["yuv444p"] = "Keeps the colour at full resolution. All but indistinguishable from RGB once converted, and slightly cheaper. The right choice for sharing code, spreadsheets or anything with coloured text.",
        ["yuv422p"] = "Keeps colour at half width and full height. Holds the vertical colour detail 4:2:0 throws away, for two thirds of the colour samples of 4:4:4 - the middle ground.",
        ["yuv420p"] = "Keeps a quarter of the colour. Smallest, and the one thing every device decodes. Coloured text and edges smear: the washed-out video-call look.",
        ["p010le"] = "Ten bits per component, still at quarter colour. More tonal steps and less banding in gradients, and only worth it if the source really is more than 8-bit.",
    };

    private static readonly Dictionary<string, string> Modes = new()
    {
        ["cbr"] = "The encoder holds the target every second, so the bandwidth is fixed and the quality moves with the scene. The right choice when the connection has a hard cap you cannot exceed.",
        ["vbr"] = "Aims at the target and is allowed to burst up to the ceiling when the picture moves, holding quality where constant bitrate would soften. Needs headroom above the average.",
        ["abr"] = "Aims at the target on average with no ceiling, so hard frames spend freely and quality holds. The simplest bitrate mode, and a good fit on a LAN where a burst costs nothing.",
        ["crf"] = "The encoder spends whatever it takes to hold the quality you set. The bitrate rises when the screen moves and falls to almost nothing when it is still. Quality first, with no bandwidth bound at all.",
        ["lossless"] = "Nothing is thrown away: what the viewer decodes is pixel-for-pixel what was captured. There is no rate control, so a moving screen can burst to hundreds of Mbit/s. LAN only.",
    };

    private static readonly Dictionary<string, string> AudioSources = new()
    {
        ["none"] = "",
        ["desktop"] = "Everything the machine plays, mixed into the stream as a second track. Viewers hear it without doing anything.",
        ["mic"] = "What a microphone hears, so you can talk over what you are showing. Pick the device on the row beside it where your machine has more than one.",
        ["application"] = "One running program's own sound and nothing else, which is what a stream carrying a game and not the call about it needs. No platform here can open it yet.",
    };

    private static readonly Dictionary<string, string> AudioCodecs = new()
    {
        ["opus"] = "Low delay, royalty-free, and the only audio codec WebRTC will negotiate. The default choice.",
        ["aac"] = "What players expect behind an RTMP or HLS address. Older and higher-delay than Opus, and WebRTC will not take it.",
    };

    private static readonly Dictionary<string, string> ColorRanges = new()
    {
        ["pc"] = "Every code value carries picture. Correct for a computer screen, which is what this is - the desktop is full range to begin with, so this is what reaches a viewer unchanged.",
        ["tv"] = "Squeezes the picture into the 16-235 broadcast range on the way in and expands it again on the way out. Pick it only when something downstream demands it: it loses code values, and players disagree slightly about how to expand them.",
    };

    private static readonly Dictionary<string, string> Efforts = new()
    {
        ["p1"] = "Least analysis, largest files for a given quality.",
        ["p2"] = "",
        ["p3"] = "",
        ["p4"] = "NVIDIA's own balance of speed and file size.",
        ["p5"] = "Used by constant bitrate, which pins the preset here for its low-delay tuning.",
        ["p6"] = "",
        ["p7"] = "Most analysis, smallest files. On the dedicated encoder chip even this barely touches the graphics cores.",

        // The software ladder.
        // Only a step carrying a fact beyond its position says anything; the rest are rungs.
        ["placebo"] = "Hours of analysis for a fraction of a percent smaller. Named as a joke by x264's own authors, and offered because the encoder offers it.",
        ["medium"] = "x264's own balance of speed and file size.",
        ["veryfast"] = "Where the live modes start: fast enough to keep up with a screen without taking every core.",
        ["ultrafast"] = "Almost no analysis. The step that keeps up with a large screen on a slow machine, at a much larger stream.",
    };

    /// <summary>
    /// What each tune aims at, and never whether it is good: which one is right follows from the content and
    /// the delay budget, both of which are the reader's.
    /// </summary>
    private static readonly Dictionary<string, string> Tunes = new()
    {
        ["none"] = "Encode the picture as it comes, with no assumption about what it contains.",
        ["film"] = "Assumes camera footage: keeps the fine texture that would otherwise be smoothed away as noise.",
        ["animation"] = "Assumes flat colour and hard edges, where detail is scarce and blocking shows immediately.",
        ["grain"] = "Preserves film grain instead of spending the bitrate erasing it. Expensive, and unmistakable when it is missing.",
        ["stillimage"] = "For a picture that barely moves, such as a slide left on screen.",
        ["psnr"] = "Optimises the arithmetic error rather than what the eye sees. For measurement, not for watching.",
        ["ssim"] = "Optimises a structural similarity score. Like PSNR, a metric target rather than a viewing one.",
        ["fastdecode"] = "Drops the coding tools that cost the most to decode, so a weak viewer can keep up.",
        ["zerolatency"] = "Drops the lookahead and the reordering, so a frame leaves the encoder as soon as it arrives. This is what keeps a live picture close to the moment it happened.",
        ["hq"] = "Spends the encoder's effort on the picture, at the delay its analysis needs.",
        ["ll"] = "Holds the delay down, which is what lets the encoder keep a constant bitrate.",
        ["ull"] = "Holds it down further still, giving up more quality for it.",
        ["lossless"] = "Codes the frame exactly, with nothing thrown away and no rate control at all.",
    };

    /// <summary>
    /// What each built-in preset delivers.
    /// The line holds for every configuration the preset accepts and no other, so it names no encoder, pixel
    /// format or capture backend: those are this machine's answer to the promise and change with the machine
    /// (<c>docs/presets.md</c>).
    /// </summary>
    private static readonly Dictionary<string, string> Presets = new()
    {
        ["lossless"] = "Bit-exact pixels: the encoder quantizes nothing, no colour detail is thrown away, and the desktop's own code values reach the viewer untouched. Bursts to hundreds of Mbit/s on motion, so LAN and localhost only.",
        ["gaming"] = "Motion first: 60 frames a second, no reorder delay, a short retransmit window, and a bitrate held constant so a busy scene costs no extra delay.",
        ["readability"] = "Text first: constant quality at a screen-share frame rate, so a still page of text gets the bits that motion would otherwise take.",
    };

    private static readonly Dictionary<string, string> Transports = new()
    {
        ["srt"] = "UDP that asks for lost packets again, within a delay window you set. One connection carries everything in both directions, so it asks no more of a home router than any other outgoing connection.",
        ["rtsp"] = "A session that carries each track as its own stream. Each leg picks what those run over: interleaved on the session's own connection, or a UDP port pair per track.",
        ["webrtc"] = "What a browser speaks. It tests the path before sending anything, which is what opens the way through a home router - the one protocol here that establishes its route rather than assuming it. No external player opens it by address.",
        ["rtmp"] = "One TCP connection, the protocol broadcast tools speak. Nothing about it is tunable: no retransmit window, no buffer, and the delay is whatever TCP and the relay make it.",
        ["hls"] = "Files and a playlist over ordinary HTTP, which gets through proxies and firewalls that block everything else. A viewer cannot start until a segment exists, so it is by far the slowest way to watch.",
    };

    private static readonly Dictionary<string, string> RtspProtocols = new()
    {
        ["tcp"] = "Everything rides the connection the session already opened. Nothing is lost and no second port has to reach the far end, so nothing extra has to cross a router or a firewall. The cost is that one late packet holds up everything queued behind it.",
        ["udp"] = "Each track gets its own port pair, which drops the delay that in-order delivery adds. Lost packets are never resent, so loss shows up as artefacts rather than as a stall. A network that blocks outgoing UDP kills it silently: the session sets up and no picture follows.",
    };

    /// <summary>
    /// What each render chain does and what it says about colour.
    /// Where the frames are converted decides the cost, and whether the route states its colour decides
    /// whether dark content survives it.
    /// </summary>
    private static readonly Dictionary<string, string> RenderChains = new()
    {
        ["gl"] = "Converts on the graphics card and hands the picture over as a texture, so no frame crosses the bus. It states the colour it produces exactly, and it was measured indistinguishable from the system-memory route on everything but a saturated colour bar.",
        ["cpu"] = "Pulls every frame into main memory and converts it there. It states the colour it produces exactly, and it is the route that always works - at the cost of the download, which at high resolutions and frame rates is gigabytes a second.",
        ["d3d11"] = "Converts on the graphics card with Direct3D 11 and brings the result back. The driver may do the conversion in its video processor, which is configured through an interface that cannot say what it did, so the colour is labelled rather than guaranteed.",
        ["d3d12"] = "The same as Direct3D 11 on the newer interface, which reaches a decoder that already produces frames in the form it wants. The same reservation about colour applies.",
        ["raw"] = "Hands the decoded frames over untouched. Nothing states a colour, so the window interprets them itself and reads an unstated one as broadcast video - which washes out dark content on a desktop. It also draws at the size the sender chose, since nothing in it scales.",
    };

    public static string Capture(string id) => Look(Captures, id);

    public static string Memory(string id) => Look(Memories, id);

    public static string DrmMap(string id) => Look(DrmMaps, id);

    public static string Format(string id) => Look(Formats, id);

    public static string Family(string id) => Look(Families, id);

    public static string Encoder(string id) => Look(Encoders, id);

    public static string Chroma(string id) => Look(Chromas, id);

    public static string Mode(string id) => Look(Modes, id);

    public static string AudioSource(string id) => Look(AudioSources, id);

    public static string AudioCodec(string id) => Look(AudioCodecs, id);

    public static string ColorRange(string id) => Look(ColorRanges, id);

    public static string Effort(string id) => Look(Efforts, id);

    public static string Tune(string id) => Look(Tunes, id);

    public static string Preset(string id) => Look(Presets, id);

    public static string Transport(string id) => Look(Transports, id);

    public static string RtspProtocol(string id) => Look(RtspProtocols, id);

    public static string RenderChain(string id) => Look(RenderChains, id);

    /// <summary>
    /// The paragraph for an identifier, and nothing where this build has none.
    /// Absence is the answer both for a value whose name says everything and for one a newer backend
    /// invented.
    /// </summary>
    private static string Look(Dictionary<string, string> text, string id) =>
        id.Length > 0 && text.TryGetValue(id, out var body) ? body : "";
}
