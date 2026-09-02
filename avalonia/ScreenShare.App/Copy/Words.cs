namespace ScreenShare.App.Copy;

/// <summary>
/// What every identifier the backend uses is called on screen.
///
/// The backend spells a codec <c>hevc_nvenc</c>, an engine <c>gstreamer</c> and a pixel format <c>yuv420p</c>,
/// those being the encoder's, the element registry's and libavutil's own names.
/// Here each becomes something a reader can use: <c>HEVC</c>, <c>GStreamer</c>, <c>4:2:0</c>.
/// Nothing here decides what is legal or what exists,
/// and a value the backend stops sending stops being looked up (<c>docs/ipc-api.md</c>).
///
/// <b>An unknown identifier answers with itself.</b>
/// A backend newer than this build names codecs this table has not heard of,
/// and <c>av1_qsv</c> on screen is one a reader can still pick, search for and report.
/// A missing word is a defect this table can fix, so it renders rather than asserting.
///
/// <b>Short here, long in <see cref="Descriptions"/>.</b>
/// Every entry is a name at the width of a dropdown row, a chip or a step strip.
///
/// <b>The identifier is kept wherever the reader meets it again.</b>
/// A pixel format reads <c>yuv420p · 4:2:0</c>,
/// that string being what the command preview, the run log and every ffmpeg answer elsewhere call it.
/// </summary>
public static class Words
{
    /// <summary>
    /// Publish engines.
    /// Both spellings, casing included, are the projects' own.
    /// </summary>
    private static readonly Dictionary<string, string> Engines = new()
    {
        ["ffmpeg"] = "ffmpeg",
        ["gstreamer"] = "GStreamer",
    };

    /// <summary>
    /// Video coding formats, named the way a viewer's decoder settings name them.
    /// Both halves of an H.26x name are kept, the ITU spelling and the ISO one,
    /// a reader having met either.
    /// </summary>
    private static readonly Dictionary<string, string> Formats = new()
    {
        ["h264"] = "H.264 / AVC",
        ["hevc"] = "H.265 / HEVC",
        ["av1"] = "AV1",
        ["vp9"] = "VP9",
        ["vp8"] = "VP8",
    };

    /// <summary>
    /// Encoder families, named by the hardware or the library a reader would go looking for.
    /// "vaapi" names a driver, where "Intel / AMD GPU" names the thing they either have or do not.
    /// </summary>
    private static readonly Dictionary<string, string> Families = new()
    {
        ["software"] = "CPU",
        ["nvenc"] = "NVIDIA GPU",
        ["vaapi"] = "Intel / AMD GPU",
        ["qsv"] = "Intel Quick Sync",
        ["amf"] = "AMD GPU",
        ["v4l2"] = "ARM SoC",
        ["rkmpp"] = "Rockchip SoC",
        ["vulkan"] = "Vulkan Video",
        ["videotoolbox"] = "Mac hardware",
    };

    /// <summary>
    /// The CPU encoders, each named for the project a reader meets in a release note or a bug report.
    /// Only these: a hardware family is one encoder and answers under its family's name,
    /// so a row here for one would be that name written twice.
    /// </summary>
    private static readonly Dictionary<string, string> Encoders = new()
    {
        ["x264"] = "CPU · x264",
        ["x265"] = "CPU · x265",
        ["libvpx"] = "CPU · libvpx",
        ["libaom"] = "CPU · libaom",
        ["svt-av1"] = "CPU · SVT-AV1",
        ["rav1e"] = "CPU · rav1e",
    };

    /// <summary>
    /// Pixel formats.
    /// The subsampling ratio carries the trade, so it leads.
    /// Bit depth is named only where it is not eight.
    /// </summary>
    private static readonly Dictionary<string, string> Chromas = new()
    {
        ["gbrp"] = "RGB, no subsampling",
        ["yuv444p"] = "4:4:4 full color",
        ["yuv422p"] = "4:2:2 half color",
        ["yuv420p"] = "4:2:0 quarter color",
        ["p010le"] = "4:2:0, 10-bit",
    };

    /// <summary>
    /// Rate-control modes, named for what the encoder holds steady.
    /// The whole difference between them, and the acronym says none of it.
    /// </summary>
    private static readonly Dictionary<string, string> Modes = new()
    {
        ["cbr"] = "Constant bitrate",
        ["vbr"] = "Variable bitrate, capped",
        ["abr"] = "Average bitrate",
        ["crf"] = "Constant quality",
        ["lossless"] = "Lossless",
    };

    /// <summary>Transports, under the acronym every other tool spells them with.</summary>
    private static readonly Dictionary<string, string> Transports = new()
    {
        ["srt"] = "SRT",
        ["rtsp"] = "RTSP",
        ["webrtc"] = "WebRTC",
        ["rtmp"] = "RTMP",
        ["hls"] = "HLS",
        ["moq"] = "MoQ",
    };

    /// <summary>
    /// Legs of a relay check no transport carries.
    /// Every other leg is a transport and is named by <see cref="Transports"/>.
    /// </summary>
    private static readonly Dictionary<string, string> RelayLegs = new()
    {
        ["groups"] = "Group service",
        ["api"] = "Relay API",
    };

    /// <summary>
    /// Capture backends, named by what they read rather than by the element that reads it.
    /// A source read by both engines repeats this half of the name, so it does not identify on its own.
    /// The engine that completes it comes from the catalog row (<see cref="Vocabulary"/>).
    /// </summary>
    private static readonly Dictionary<string, string> Captures = new()
    {
        ["ddagrab"] = "Desktop Duplication",
        ["gdigrab"] = "Whole desktop, CPU copy",
        ["d3d11screencapturesrc"] = "Desktop Duplication",
        ["x11grab"] = "X11 screen",
        ["ximagesrc"] = "X11 screen",
        ["kmsgrab"] = "Scanout buffers (DRM/KMS)",
        ["portal"] = "Screen picker (portal)",
        ["avfoundation"] = "macOS screen",
        ["avfvideosrc"] = "macOS screen",
    };

    private static readonly Dictionary<string, string> Memories = new()
    {
        ["auto"] = "Automatic",
        ["gpu"] = "Stay on the GPU",
        ["gpu-encoder-color"] = "Stay on the GPU, encoder's color",
        ["system"] = "Copy through RAM",
    };

    /// <summary>
    /// Where a pipeline's frames are held, in the spelling the memory feature on its caps uses.
    /// Not <see cref="Memories"/> above, which names the setting that asks for one.
    /// The two sets do not correspond: a chain asked to stay on the GPU reports the API it used,
    /// and one asked for nothing in particular reports whatever it negotiated.
    /// </summary>
    private static readonly Dictionary<string, string> FrameMemories = new()
    {
        ["memory:SystemMemory"] = "system memory",
        ["memory:D3D11Memory"] = "the GPU, Direct3D 11",
        ["memory:D3D12Memory"] = "the GPU, Direct3D 12",
        ["memory:GLMemory"] = "the GPU, OpenGL",
        ["memory:DMABuf"] = "the GPU, dmabuf",
    };

    private static readonly Dictionary<string, string> Cursors = new()
    {
        ["embedded"] = "Drawn into the picture",
        ["hidden"] = "Not shared",
        ["metadata"] = "Sent beside the picture",
    };

    /// <summary>kmsgrab download strategies, named by the device that maps.</summary>
    private static readonly Dictionary<string, string> DrmMaps = new()
    {
        ["auto"] = "Match the GPU",
        ["vaapi"] = "Through VAAPI",
        ["vulkan"] = "Through Vulkan",
        ["none"] = "No mapping",
    };

    private static readonly Dictionary<string, string> AudioSources = new()
    {
        ["none"] = "No audio",
        ["desktop"] = "What this computer plays",
        ["application"] = "One application",
    };

    private static readonly Dictionary<string, string> AudioCodecs = new()
    {
        ["opus"] = "Opus",
        ["aac"] = "AAC",
    };

    /// <summary>
    /// Quantization ranges, named by the code values they use.
    /// The numbers are the whole difference, and what a mismatch shows up as.
    /// </summary>
    private static readonly Dictionary<string, string> ColorRanges = new()
    {
        ["pc"] = "Full range (0-255)",
        ["tv"] = "Limited range (16-235)",
    };

    /// <summary>
    /// Effort ladders, every encoder's steps in one table.
    /// A step is the encoder's own identifier, and the backend offers whichever ladder the selected codec declares,
    /// so the named ladders sit together: no codec offers both, and a step of one is never a step of the other.
    /// Only the ends and the defaults carry a word, naming each step between them implying a difference in kind.
    /// The numeric ladders are absent: their steps are numbers on the encoder's own scale,
    /// 0 to 13 on SVT-AV1 and 0 to 8 on libaom, so the number is the name and the lookup falls through.
    /// </summary>
    private static readonly Dictionary<string, string> Efforts = new()
    {
        // NVIDIA's ladder, slowest step first, as the backend orders every ladder.
        ["p7"] = "p7 · smallest",
        ["p6"] = "p6",
        ["p5"] = "p5",
        ["p4"] = "p4 · default",
        ["p3"] = "p3",
        ["p2"] = "p2",
        ["p1"] = "p1 · fastest",

        // x264's ladder, taken by libx265 as well.
        ["placebo"] = "Placebo · slowest",
        ["veryslow"] = "Very slow",
        ["slower"] = "Slower",
        ["slow"] = "Slow",
        ["medium"] = "Medium · default",
        ["fast"] = "Fast",
        ["faster"] = "Faster",
        ["veryfast"] = "Very fast",
        ["superfast"] = "Superfast",
        ["ultrafast"] = "Ultrafast · fastest",

        // AMD's quality scale, spelled alike by all three of its encoders.
        // Three steps and no ladder between them, so each carries a word.
        ["quality"] = "Quality",
        ["balanced"] = "Balanced",
        ["speed"] = "Speed",
    };

    /// <summary>
    /// Tune ladders: what an encoder aims at while it spends its effort.
    /// Several vocabularies meet here, as above.
    /// x264 and x265 name what the picture is or what the decoder needs.
    /// The NVIDIA encoders name the delay they hold, in the SDK's abbreviations,
    /// which is what the backend carries and what a log shows.
    /// The AV1 and VP encoders name a score to maximise, or the judgement that weighs what the eye sees.
    /// Quick Sync names the session the encode is for.
    /// </summary>
    private static readonly Dictionary<string, string> Tunes = new()
    {
        ["none"] = "No tuning",
        ["film"] = "Film",
        ["animation"] = "Animation",
        ["grain"] = "Film grain",
        ["stillimage"] = "Still image",
        ["psnr"] = "PSNR",
        ["ssim"] = "SSIM",
        ["ms-ssim"] = "Multi-scale SSIM",
        ["vq"] = "Visual quality",
        ["iq"] = "Image quality",
        ["psychovisual"] = "What the eye sees",
        ["fastdecode"] = "Easy to decode",
        ["zerolatency"] = "Zero latency",
        ["hq"] = "Quality",
        ["ll"] = "Low latency",
        ["ull"] = "Ultra low latency",
        ["lossless"] = "Lossless",
        ["displayremoting"] = "Screen sharing",
        ["videoconference"] = "Video call",
        ["archive"] = "Archive",
        ["livestreaming"] = "Live stream",
        ["cameracapture"] = "Camera",
        ["videosurveillance"] = "Surveillance",
        ["gamestreaming"] = "Game stream",
        ["remotegaming"] = "Remote play",
    };

    /// <summary>
    /// Built-in presets, named for what each puts first.
    /// A preset is a promise about the picture rather than a set of values,
    /// so a name says what is asked for.
    /// The encoder answering is this machine's and differs on the next (<c>docs/presets.md</c>).
    /// </summary>
    private static readonly Dictionary<string, string> Presets = new()
    {
        ["lossless"] = "Lossless",
        ["gaming"] = "Gaming",
        ["readability"] = "Text and detail",
    };

    /// <summary>Lower transport RTP takes inside an RTSP session, not the session's own.</summary>
    private static readonly Dictionary<string, string> RtspProtocols = new()
    {
        ["tcp"] = "TCP, on the RTSP connection",
        ["udp"] = "UDP, its own port pair",
    };

    /// <summary>
    /// Render chains, named by where the frames are converted and what that says about their color,
    /// the two halves of the choice.
    /// The element names the backend builds each from stay behind the boundary,
    /// so a reader picks a place and a promise.
    /// </summary>
    private static readonly Dictionary<string, string> RenderChains = new()
    {
        ["gl"] = "Graphics card, OpenGL · exact color",
        ["cpu"] = "System memory · exact color",
        ["d3d11"] = "Graphics card, Direct3D 11 · driver's color",
        ["d3d12"] = "Graphics card, Direct3D 12 · driver's color",
        ["raw"] = "No conversion",
    };

    /// <summary>
    /// Decode paths, named by the hardware a viewer would have rather than by the plugin the element comes from.
    /// A reader deciding what to publish is deciding whose machine copes, and "vaapi" is not a machine's name.
    /// The keys are the decoders' own and not <see cref="Families"/>: <c>va</c> here, <c>vaapi</c> there.
    /// </summary>
    private static readonly Dictionary<string, string> DecodeFamilies = new()
    {
        ["software"] = "any CPU",
        ["va"] = "AMD and Intel GPUs on Linux",
        ["nvcodec"] = "NVIDIA GPUs",
        ["qsv"] = "Intel GPUs",
        ["dxva"] = "any GPU on Windows",
        ["videotoolbox"] = "any Mac",
    };

    /// <summary>
    /// Transfer characteristics a decode reports, under the names a reader meets them by.
    /// The HDR curves are why the table exists: a tile says which curve it draws.
    /// PQ is absolute and mastered for a bright display, HLG relative and degrades into a standard one on its own.
    /// </summary>
    private static readonly Dictionary<string, string> Transfers = new()
    {
        ["smpte2084"] = "HDR (PQ)",
        ["arib-std-b67"] = "HDR (HLG)",
        ["bt709"] = "BT.709",
        ["bt601"] = "BT.601",
        ["bt2020-10"] = "BT.2020",
        ["bt2020-12"] = "BT.2020",
        ["srgb"] = "sRGB",
        ["smpte240m"] = "SMPTE 240M",
        ["adobergb"] = "Adobe RGB",
    };

    private static readonly Dictionary<string, string> OperatingSystems = new()
    {
        ["windows"] = "Windows",
        ["linux"] = "Linux",
        ["darwin"] = "macOS",
    };

    private static readonly Dictionary<string, string> DisplayServers = new()
    {
        ["x11"] = "X11",
        ["wayland"] = "Wayland",
    };

    public static string Engine(string id) => Look(Engines, id);

    /// <summary>
    /// Same two names for the enum a catalog row carries, where a statement carries the identifier.
    /// An unset engine answers empty, which the caller leaves out of a name.
    /// </summary>
    public static string Engine(Api.V1.Engine engine) => engine switch
    {
        Api.V1.Engine.Ffmpeg => Look(Engines, "ffmpeg"),
        Api.V1.Engine.Gstreamer => Look(Engines, "gstreamer"),
        _ => "",
    };

    public static string Format(string id) => Look(Formats, id);

    public static string Family(string id) => Look(Families, id);

    /// <summary>What produces a bitstream: the library where the CPU is what runs it, the family otherwise.</summary>
    public static string Encoder(string id) =>
        Encoders.TryGetValue(id, out var word) ? word : Family(id);

    public static string Chroma(string id) => Look(Chromas, id);

    public static string Mode(string id) => Look(Modes, id);

    public static string Transport(string id) => Look(Transports, id);

    /// <summary>
    /// One leg of a relay check: a transport, or one of the services beside them.
    /// The transports fall through to <see cref="Transport"/>,
    /// so a leg is spelled the same here as in the dropdown a reader picked it from.
    /// </summary>
    public static string RelayLeg(string id) =>
        RelayLegs.TryGetValue(id, out var word) ? word : Transport(id);

    public static string Capture(string id) => Look(Captures, id);

    public static string Memory(string id) => Look(Memories, id);

    public static string FrameMemory(string id) => Look(FrameMemories, id);

    public static string Transfer(string id) => Look(Transfers, id);

    public static string Cursor(string id) => Look(Cursors, id);

    public static string DrmMap(string id) => Look(DrmMaps, id);

    public static string AudioSource(string id) => Look(AudioSources, id);

    public static string AudioCodec(string id) => Look(AudioCodecs, id);

    public static string ColorRange(string id) => Look(ColorRanges, id);

    public static string Effort(string id) => Look(Efforts, id);

    public static string Tune(string id) => Look(Tunes, id);

    /// <summary>
    /// Name of a built-in preset.
    /// <see cref="Effort"/> holds what x264 calls a preset:
    /// a settings value on a ladder, where this is a way of publishing the form applies to the draft.
    /// </summary>
    public static string Preset(string id) => Look(Presets, id);

    public static string RtspProtocol(string id) => Look(RtspProtocols, id);

    public static string RenderChain(string id) => Look(RenderChains, id);

    public static string DecodeFamily(string id) => Look(DecodeFamilies, id);

    public static string OperatingSystem(string id) => Look(OperatingSystems, id);

    public static string DisplayServer(string id) => Look(DisplayServers, id);

    /// <summary>
    /// A list of names as a sentence reads it: "a", "a and b", "a, b or c".
    /// Joining word is the caller's: "and" where every item holds, "or" where one is picked.
    /// </summary>
    public static string List(IEnumerable<string> names, string last = "or")
    {
        var items = names.Where(name => name.Length > 0).ToList();
        return items.Count switch
        {
            0 => "",
            1 => items[0],
            2 => $"{items[0]} {last} {items[1]}",
            _ => $"{string.Join(", ", items.Take(items.Count - 1))} {last} {items[^1]}",
        };
    }

    /// <summary>
    /// Name for an identifier, falling back to the identifier itself.
    /// The fallback is an answer rather than a guard:
    /// a value this build has no word for is still one the backend accepts,
    /// and the raw identifier is something a reader can pick, search for and report.
    /// </summary>
    private static string Look(Dictionary<string, string> words, string id) =>
        id.Length > 0 && words.TryGetValue(id, out var word) ? word : id;
}
