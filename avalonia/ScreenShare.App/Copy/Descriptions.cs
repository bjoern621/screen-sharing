namespace ScreenShare.App.Copy;

/// <summary>
/// Paragraph behind one choice: what it is, and when to pick it.
/// Shown under a radio card's title, and in a dropdown entry where there is room (<c>docs/tooltips.md</c>).
///
/// Say the observable effect and not the definition:
/// "keeps a quarter of the color" is the definition,
/// "colored text and edges smear" is what the reader sees, and the effect leads.
/// Name what a choice needs before it is picked, the backend graying only what it can prove.
/// An empty answer is allowed for a value whose name says everything, and the row then draws its name alone.
/// </summary>
public static class Descriptions
{
    private static readonly Dictionary<string, string> Captures = new()
    {
        ["ddagrab"] = "Windows' own screen capture, read on the GPU, one screen at a time. Capturing costs almost no CPU. Pick it on Windows.",
        ["gdigrab"] = "Copies the desktop with the CPU, every screen as one wide frame. On a desktop with more than one screen the frame can exceed what NVIDIA's encoder accepts. Use it only where the other Windows methods do not start.",
        ["d3d11screencapturesrc"] = "The same Windows capture surface, read by GStreamer instead of ffmpeg. Pick it to reach GStreamer's encoders and its WebRTC output on Windows.",
        ["x11grab"] = "Reads the X11 screen through shared memory. The default on Linux. It sees XWayland windows but not a native Wayland desktop.",
        ["ximagesrc"] = "The same X11 screen, read by GStreamer instead of ffmpeg. Pick it to reach GStreamer's encoders on an X11 session.",
        ["kmsgrab"] = "Reads the frames the GPU scans out to the screen, below the compositor. The cheapest capture, and the only one that needs elevated privileges. Without them the capture stops at launch, and nothing can tell in advance.",
        ["portal"] = "Opens the desktop's own picker, where the screen or window is chosen. Works on Wayland, and on X11 too. Offers fewer pixel formats and rate-control settings than the ffmpeg methods. The fields say so where it matters.",
        ["avfoundation"] = "macOS screen capture, by screen index. macOS offers no system-audio input device, so desktop audio cannot be recorded.",
        ["avfvideosrc"] = "The same macOS screen, read by GStreamer instead of ffmpeg. It picks the screen itself, so the screen setting does not reach it.",
    };

    private static readonly Dictionary<string, string> Memories = new()
    {
        ["auto"] = "Keeps the frames on the GPU where the capture and the encoder can share them, and copies through RAM where they cannot. The one setting that works with every combination.",
        ["gpu"] = "Hands the encoder the frames the capture produced: no copy out, no CPU conversion, no copy back. It also demands the color chosen here. It is refused where the pair shares no path, or where that path cannot convert.",
        ["gpu-encoder-color"] = "Keeps the frames on the GPU even where nothing on the way converts them. The encoder reads the captured surface as it is and picks the color itself. The pixel format and color range fields gray out and say what the encoder sends instead.",
        ["system"] = "Copies every frame to RAM, converts it on the CPU, and hands it back. Works with every combination. The right choice when capture and encoding run on different GPUs.",
    };

    private static readonly Dictionary<string, string> DrmMaps = new()
    {
        ["auto"] = "Picks the mapping device from the capture GPU's driver: VAAPI on Intel and AMD, Vulkan elsewhere.",
        ["vaapi"] = "Maps through VAAPI. Works on Intel and AMD, whose drivers expose a VAAPI device.",
        ["vulkan"] = "Maps through Vulkan, which every vendor implements. Pick it on NVIDIA, or where VAAPI is missing.",
        ["none"] = "Copies the frame with no mapping. Only correct for a plain, untiled framebuffer. Any other layout stops the capture immediately.",
    };

    private static readonly Dictionary<string, string> Formats = new()
    {
        ["h264"] = "Plays everywhere: every browser, every phone, every player. The least efficient of the modern formats, so it needs the most bits for the same picture.",
        ["hevc"] = "About 40% smaller than H.264 at the same quality. Browser playback needs Safari or an OS extension, so pick it where the viewers' players are known.",
        ["av1"] = "The most efficient per bit. Hardware encoding needs NVIDIA RTX 40, Intel Arc, or AMD RDNA3 and up.",
        ["vp9"] = "Royalty-free and plays in every browser except Safari. Efficiency sits between H.264 and HEVC.",
        ["vp8"] = "Royalty-free and carried everywhere WebRTC is. Roughly as efficient as H.264.",
    };

    private static readonly Dictionary<string, string> Families = new()
    {
        ["software"] = "Encodes on the CPU and works everywhere. A full desktop at a high frame rate uses a large part of this computer, more for the newer formats.",
        ["nvenc"] = "NVIDIA's dedicated encoder chip, separate from the graphics cores. A stream costs almost no game performance. Needs an NVIDIA card and its driver.",
        ["vaapi"] = "The Intel and AMD hardware encoder on Linux. Which formats a card encodes depends on its generation. All of them are 4:2:0, and none is lossless.",
        ["qsv"] = "Intel's own runtime for the same silicon VAAPI reaches. Intel graphics only. Which formats it encodes depends on the generation: VP9 from Ice Lake, AV1 from Arc.",
        ["amf"] = "AMD's own encoder runtime, driving the same chip VAAPI reaches. VAAPI covers more formats. AMF adds AMD's rate control, including a capped mode with a burst ceiling. x86-64 and ffmpeg only.",
        ["v4l2"] = "The kernel's encoders on ARM boards: Raspberry Pi and similar.",
        ["rkmpp"] = "Rockchip's media encoders, on RK35xx-class boards.",
        ["vulkan"] = "Encoding through the Vulkan driver, which every vendor implements. Which formats it reaches depends on the driver. All of them are 4:2:0, and none is lossless. ffmpeg only.",
        ["videotoolbox"] = "The media chip built into every Mac, separate from the graphics cores. It encodes at an average bitrate only, and produces only 4:2:0.",
    };

    /// <summary>
    /// The CPU encoders, each stating what it is and what it costs against the others that code the same format.
    /// Only these: a hardware family is one encoder and answers with its family's paragraph,
    /// so a row here for one would be that paragraph written twice.
    /// </summary>
    private static readonly Dictionary<string, string> Encoders = new()
    {
        ["x264"] = "The most widely used H.264 encoder. It handles every pixel format offered here, encodes lossless, and keeps up with a desktop at a high frame rate.",
        ["x265"] = "HEVC on the CPU, around a third fewer bits than x264 for the same picture. It works much harder for them. A full desktop at a high frame rate uses a large part of this computer.",
        ["libvpx"] = "Google's VP8 and VP9 encoder. VP9 saves bits over H.264, and every browser except Safari decodes it. VP8 is for players that take nothing else.",
        ["libaom"] = "The AV1 reference encoder. The only CPU AV1 encoder here that produces full color and RGB. The slowest of the three, even in realtime mode.",
        ["svt-av1"] = "The fastest realtime AV1 encoder. The one that keeps up at desktop resolution. 4:2:0 and 10-bit only.",
        ["rav1e"] = "Between the other two in speed. Produces full color and 10-bit. It takes one bitrate target with no ceiling, and its quality scale ends at 255 rather than 51.",
    };

    private static readonly Dictionary<string, string> Chromas = new()
    {
        ["gbrp"] = "The desktop's own pixels, with no color conversion. Text and thin lines arrive exactly as they look here. The heaviest option, and few GPUs decode it.",
        ["yuv444p"] = "Keeps the color at full resolution, nearly indistinguishable from RGB. The right choice for code, spreadsheets, or anything with colored text.",
        ["yuv422p"] = "Keeps color at half width and full height. The middle ground between 4:2:0 and 4:4:4.",
        ["yuv420p"] = "Keeps a quarter of the color. The smallest, and the one every device decodes. Colored text and edges smear: the washed-out video-call look.",
        ["p010le"] = "Ten bits per component, still at quarter color. Less banding in gradients. Only worth it when the source has more than 8 bits.",
    };

    private static readonly Dictionary<string, string> Modes = new()
    {
        ["cbr"] = "Fixed bandwidth, moving quality. The encoder holds the target every second. The right choice when the connection has a hard cap.",
        ["vbr"] = "Aims at the target and bursts to the ceiling on motion. Holds quality where constant bitrate would soften. Needs headroom above the average.",
        ["abr"] = "Aims at the target on average, with no ceiling. Quality holds through hard frames. A good fit on a LAN, where a burst costs nothing.",
        ["crf"] = "Constant quality, variable bandwidth. The rate rises with motion and falls to almost nothing on a still screen. Set a ceiling to keep busy moments within the connection.",
        ["lossless"] = "The viewer decodes pixel for pixel what was captured. Nothing bounds the rate, so a moving screen can burst to hundreds of Mbit/s. LAN only.",
    };

    private static readonly Dictionary<string, string> AudioSources = new()
    {
        ["none"] = "",
        ["desktop"] = "Everything this computer plays, mixed into the stream. Pick the output on the row beside it where more than one exists.",
        ["application"] = "One program's own sound and nothing else, so a game goes out without the call about it. Pick the program on the row beside it.",
    };

    private static readonly Dictionary<string, string> AudioCodecs = new()
    {
        ["opus"] = "Low delay, royalty-free, and the only audio codec WebRTC negotiates. The default choice.",
        ["aac"] = "What players expect behind an RTMP or HLS address. Higher delay than Opus, and WebRTC does not take it.",
    };

    private static readonly Dictionary<string, string> ColorRanges = new()
    {
        ["pc"] = "Every code value carries picture. The desktop is full range, so this reaches viewers unchanged. Correct for a screen share.",
        ["tv"] = "Squeezes the picture into the 16-235 broadcast range and expands it again at the viewer. Pick it only when something downstream demands it. It loses code values, and players expand them slightly differently.",
    };

    private static readonly Dictionary<string, string> Efforts = new()
    {
        ["p1"] = "Least analysis, largest files for a given quality.",
        ["p2"] = "",
        ["p3"] = "",
        ["p4"] = "NVIDIA's default balance of speed and file size.",
        ["p5"] = "Constant bitrate pins the effort here for its low-delay tuning.",
        ["p6"] = "",
        ["p7"] = "Most analysis, smallest files. On the dedicated encoder chip even this barely touches the graphics cores.",

        // Software ladder.
        // Only a step carrying a fact beyond its position says anything, the rest being rungs.
        ["placebo"] = "Hours of analysis for files under a percent smaller. It cannot keep up with a live stream.",
        ["medium"] = "x264's default balance of speed and file size.",
        ["veryfast"] = "Fast enough to keep up with a screen without taking every core.",
        ["ultrafast"] = "Almost no analysis. Keeps up with a large screen on a slow computer, at a much larger stream.",
    };

    /// <summary>
    /// What each tune aims at.
    /// Which one is right follows from the content and the delay budget, both the reader's.
    /// </summary>
    private static readonly Dictionary<string, string> Tunes = new()
    {
        ["none"] = "Encodes the picture as it comes, with no assumption about its content.",
        ["film"] = "Assumes camera footage: keeps the fine texture that would otherwise be smoothed away as noise.",
        ["animation"] = "Assumes flat color and hard edges, where detail is scarce and blocking shows immediately.",
        ["grain"] = "Keeps film grain instead of spending bitrate erasing it. Expensive.",
        ["stillimage"] = "For a picture that barely moves, such as a slide left on screen.",
        ["psnr"] = "Optimizes for arithmetic error rather than what the eye sees. A metric target, for measurement.",
        ["ssim"] = "Optimizes for a structural similarity score. A metric target, like PSNR.",
        ["ms-ssim"] = "The same structural score read at several scales at once. A metric target, like the other two.",
        ["vq"] = "Weighs what a viewer would notice rather than a score. The right choice for a watched picture.",
        ["iq"] = "Aims at how a still frame reads. Controls sharpening, quantization tables, and the screen-content tools to get there.",
        ["psychovisual"] = "Weighs what the eye notices instead of a score, keeping detail a metric target would smooth away.",
        ["fastdecode"] = "Drops the coding tools that cost the most to decode, so a weak viewer can keep up.",
        ["zerolatency"] = "Drops lookahead and reordering, so a frame leaves the encoder as soon as it arrives. Keeps a live picture close to the moment it happened.",
        ["hq"] = "Spends the encoder's effort on the picture, at the delay its analysis needs.",
        ["ll"] = "Holds the delay down, which lets the encoder keep a constant bitrate.",
        ["ull"] = "Holds the delay down further, giving up more quality for it.",
        ["lossless"] = "Encodes the frame exactly, with no rate control.",
        ["displayremoting"] = "Tells the encoder the content is a desktop watched somewhere else. The right choice for a screen share.",
        ["videoconference"] = "For faces over a static background, where the picture holds still and one region moves.",
        ["archive"] = "For an encode nobody waits on. Trades delay for the smallest file.",
        ["livestreaming"] = "For a stream to many viewers. Holds a steady rate over a long run.",
        ["cameracapture"] = "For a camera feed, where the picture carries sensor noise and constant small motion.",
        ["videosurveillance"] = "For a fixed camera watching a scene that rarely changes.",
        ["gamestreaming"] = "For rendered motion, where whole scenes change at once and delay matters.",
        ["remotegaming"] = "For rendered motion someone plays through. Delay above everything else.",
    };

    private static readonly Dictionary<string, string> Transports = new()
    {
        ["srt"] = "UDP that requests lost packets again, within a delay window set here. One outgoing connection carries everything, so a home router needs no setup.",
        ["rtsp"] = "A session that carries each track as its own stream. Each leg picks what those run over: interleaved on the session's own connection, or a UDP port pair per track. It is the only protocol here that carries every format, and it adds the least delay.",
        ["webrtc"] = "What a browser speaks. It tests the path before sending, which opens the way through a home router. No external player opens it by address.",
        ["rtmp"] = "One TCP connection, the protocol broadcast tools speak. Nothing is tunable: the delay is whatever TCP and the relay make it.",
        ["hls"] = "Files and a playlist over ordinary HTTP, which gets through proxies and firewalls that block everything else. A viewer cannot start until a segment exists, so it is by far the slowest way to watch.",
        ["moq"] = "Tracks pushed over QUIC, which a browser subscribes to and decodes itself. The only watch protocol that carries every format offered here, and only a browser opens it. It needs both the TCP and UDP side of its port to reach the relay.",
    };

    private static readonly Dictionary<string, string> RtspProtocols = new()
    {
        ["tcp"] = "Everything rides the connection the session opened. Nothing is lost, and no extra port has to cross a router or firewall. One late packet holds up everything behind it.",
        ["udp"] = "Each track gets its own port pair, which drops the delay of in-order delivery. Lost packets are not resent, so loss shows as artifacts rather than a stall. A network that blocks outgoing UDP breaks it silently. The session sets up and no picture follows.",
    };

    /// <summary>
    /// What each render chain does and what it says about color.
    /// Where the frames are converted decides the cost,
    /// and whether the route states its color decides whether dark content survives it.
    /// </summary>
    private static readonly Dictionary<string, string> RenderChains = new()
    {
        ["gl"] = "Converts on the graphics card and hands the picture over as a texture. No frame crosses the bus, and the color is stated exactly.",
        ["cpu"] = "Pulls every frame into main memory and converts it there. States the color exactly, and always works. The download costs gigabytes a second at high resolutions.",
        ["d3d11"] = "Converts on the graphics card with Direct3D 11 and brings the result back. The driver may convert in its video processor, which cannot report what it did. The color is labeled rather than guaranteed.",
        ["d3d12"] = "The same on Direct3D 12, which reaches a decoder producing frames in the form it wants. The same reservation about color applies.",
        ["raw"] = "Hands the decoded frames over untouched. No color is stated, so the window guesses and reads them as broadcast video, which washes out a desktop. It draws at the size the sender chose, since nothing in it scales.",
    };

    public static string Capture(string id) => Look(Captures, id);

    public static string Memory(string id) => Look(Memories, id);

    public static string DrmMap(string id) => Look(DrmMaps, id);

    public static string Format(string id) => Look(Formats, id);

    public static string Family(string id) => Look(Families, id);

    /// <summary>What produces a bitstream: the library where the CPU is what runs it, the family otherwise.</summary>
    public static string Encoder(string id) =>
        Encoders.TryGetValue(id, out var body) ? body : Family(id);

    public static string Chroma(string id) => Look(Chromas, id);

    public static string Mode(string id) => Look(Modes, id);

    public static string AudioSource(string id) => Look(AudioSources, id);

    public static string AudioCodec(string id) => Look(AudioCodecs, id);

    public static string ColorRange(string id) => Look(ColorRanges, id);

    public static string Effort(string id) => Look(Efforts, id);

    public static string Tune(string id) => Look(Tunes, id);

    public static string Transport(string id) => Look(Transports, id);

    public static string RtspProtocol(string id) => Look(RtspProtocols, id);

    public static string RenderChain(string id) => Look(RenderChains, id);

    /// <summary>
    /// Paragraph for an identifier, and nothing where this build has none.
    /// Absence is the answer both for a value whose name says everything and for one a newer backend invented.
    /// </summary>
    private static string Look(Dictionary<string, string> text, string id) =>
        id.Length > 0 && text.TryGetValue(id, out var body) ? body : "";
}
