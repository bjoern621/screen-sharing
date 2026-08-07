namespace ScreenShare.App.Copy;

/// <summary>
/// What every identifier the backend uses is called on screen.
///
/// The backend names a codec <c>hevc_nvenc</c>, an engine <c>gstreamer</c> and a pixel
/// format <c>yuv420p</c>, because those are what the encoder, the element registry and
/// libavutil call them. This file is where each of those becomes something a reader can
/// use: <c>HEVC</c>, <c>GStreamer</c>, <c>4:2:0</c>. Nothing here decides what is legal or
/// what exists - that is the backend's, and a value it stops sending simply stops being
/// looked up (docs/ipc-api.md).
///
/// Three rules hold across the whole file.
///
/// <b>An unknown identifier answers with itself.</b> A backend newer than this build can
/// name a codec this table has never heard of, and showing <c>av1_qsv</c> is a screen a
/// reader can still act on where a blank is one they cannot. It is a defect and not a
/// failure, which is why it renders rather than asserting.
///
/// <b>Short here, long in <see cref="Descriptions"/>.</b> Everything in this file has to
/// fit a dropdown row, a chip and a step strip, so it is a name and never a sentence. The
/// paragraph explaining the choice lives beside it in the other file and is shown where
/// there is room.
///
/// <b>The identifier is kept wherever the reader will meet it again.</b> A pixel format
/// reads <c>yuv420p · 4:2:0</c> and not <c>4:2:0</c> alone, because that string is what
/// the command preview, the run log and every ffmpeg answer on the internet will call it.
/// Hiding it would make this app the only place the setting has that name.
/// </summary>
public static class Words
{
    /// <summary>The publish engines. Both spellings are the projects' own.</summary>
    private static readonly Dictionary<string, string> Engines = new()
    {
        ["ffmpeg"] = "ffmpeg",
        ["gstreamer"] = "GStreamer",
    };

    /// <summary>
    /// The video coding formats, named the way a viewer's decoder settings name them. Both
    /// halves of the H.26x names are kept: one is the ITU spelling and the other the ISO
    /// one, and which of the two a reader has met before is not knowable from here.
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
    /// The encoder families, named by the hardware or the library a reader would go looking
    /// for. "vaapi" names a driver; "Intel / AMD GPU" names the thing they either have or
    /// do not.
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
    };

    /// <summary>
    /// The pixel formats. The subsampling ratio is the part that carries the trade, so it
    /// leads; the bit depth is called out only where it is not eight.
    /// </summary>
    private static readonly Dictionary<string, string> Chromas = new()
    {
        ["gbrp"] = "RGB, no subsampling",
        ["yuv444p"] = "4:4:4 full colour",
        ["yuv422p"] = "4:2:2 half colour",
        ["yuv420p"] = "4:2:0 quarter colour",
        ["p010le"] = "4:2:0, 10-bit",
    };

    /// <summary>
    /// The rate-control modes, named for what the encoder holds steady. That is the whole
    /// distinction between them, and the acronym alone says none of it.
    /// </summary>
    private static readonly Dictionary<string, string> Modes = new()
    {
        ["cbr"] = "Constant bitrate",
        ["vbr"] = "Variable bitrate, capped",
        ["abr"] = "Average bitrate",
        ["crf"] = "Constant quality",
        ["lossless"] = "Lossless",
    };

    /// <summary>The transports, named by the acronym everything else in the world uses.</summary>
    private static readonly Dictionary<string, string> Transports = new()
    {
        ["srt"] = "SRT",
        ["rtsp"] = "RTSP",
        ["webrtc"] = "WebRTC",
        ["rtmp"] = "RTMP",
        ["hls"] = "HLS",
    };

    /// <summary>
    /// The capture backends, named by what they read rather than by the element that reads
    /// it. Two of them are the same source on two engines, and the name says so: which
    /// engine that is follows from the entry's own note.
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

    /// <summary>Where the frames reach the encoder.</summary>
    private static readonly Dictionary<string, string> Memories = new()
    {
        ["auto"] = "Automatic",
        ["gpu"] = "Stay on the GPU",
        ["gpu-encoder-color"] = "Stay on the GPU, encoder's colour",
        ["system"] = "Copy through RAM",
    };

    /// <summary>The kmsgrab download strategies, named by the device that does the mapping.</summary>
    private static readonly Dictionary<string, string> DrmMaps = new()
    {
        ["auto"] = "Match the GPU",
        ["vaapi"] = "Through VAAPI",
        ["vulkan"] = "Through Vulkan",
        ["none"] = "No mapping",
    };

    /// <summary>The second-track sources.</summary>
    private static readonly Dictionary<string, string> AudioSources = new()
    {
        ["none"] = "No audio",
        ["desktop"] = "What this machine plays",
    };

    private static readonly Dictionary<string, string> AudioCodecs = new()
    {
        ["opus"] = "Opus",
        ["aac"] = "AAC",
    };

    /// <summary>
    /// The quantization ranges, named by the code values they use. The numbers are the
    /// whole of the difference and are what a mismatch shows up as.
    /// </summary>
    private static readonly Dictionary<string, string> ColorRanges = new()
    {
        ["pc"] = "Full range (0-255)",
        ["tv"] = "Limited range (16-235)",
    };

    /// <summary>
    /// The NVENC preset ladder. Only the ends and the default carry a word: the steps
    /// between them are a ladder, and naming each one would imply a difference in kind.
    /// </summary>
    private static readonly Dictionary<string, string> EncPresets = new()
    {
        ["p1"] = "p1 · fastest",
        ["p2"] = "p2",
        ["p3"] = "p3",
        ["p4"] = "p4 · default",
        ["p5"] = "p5",
        ["p6"] = "p6",
        ["p7"] = "p7 · smallest",
    };

    /// <summary>The RTP lower transports an RTSP session runs over.</summary>
    private static readonly Dictionary<string, string> RtspProtocols = new()
    {
        ["tcp"] = "TCP, on the RTSP connection",
        ["udp"] = "UDP, its own port pair",
    };

    /// <summary>
    /// The decode paths, named by the hardware a viewer would have rather than by the
    /// plugin the element comes from. A reader deciding what to publish is deciding whose
    /// machine will cope, and "vaapi" is not the name of a machine.
    /// </summary>
    private static readonly Dictionary<string, string> DecodeFamilies = new()
    {
        ["software"] = "any CPU",
        ["va"] = "AMD and Intel GPUs on Linux",
        ["nvcodec"] = "NVIDIA GPUs",
        ["qsv"] = "Intel GPUs",
        ["dxva"] = "any GPU on Windows",
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

    public static string Format(string id) => Look(Formats, id);

    public static string Family(string id) => Look(Families, id);

    public static string Chroma(string id) => Look(Chromas, id);

    public static string Mode(string id) => Look(Modes, id);

    public static string Transport(string id) => Look(Transports, id);

    public static string Capture(string id) => Look(Captures, id);

    public static string Memory(string id) => Look(Memories, id);

    public static string DrmMap(string id) => Look(DrmMaps, id);

    public static string AudioSource(string id) => Look(AudioSources, id);

    public static string AudioCodec(string id) => Look(AudioCodecs, id);

    public static string ColorRange(string id) => Look(ColorRanges, id);

    public static string EncPreset(string id) => Look(EncPresets, id);

    public static string RtspProtocol(string id) => Look(RtspProtocols, id);

    public static string DecodeFamily(string id) => Look(DecodeFamilies, id);

    public static string OperatingSystem(string id) => Look(OperatingSystems, id);

    public static string DisplayServer(string id) => Look(DisplayServers, id);

    /// <summary>
    /// A list of names as a sentence reads it: "a", "a and b", "a, b or c". The joining word
    /// is the caller's, because a list of things that all hold and a list of things to
    /// choose between are the same list read two ways.
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
    /// The name for an identifier, falling back to the identifier itself.
    ///
    /// The fallback is the honest answer rather than a guard: a value this build has no
    /// word for is still a value the backend will accept, and a reader shown the raw
    /// identifier can pick it, search for it and report it.
    /// </summary>
    private static string Look(Dictionary<string, string> words, string id) =>
        id.Length > 0 && words.TryGetValue(id, out var word) ? word : id;
}
