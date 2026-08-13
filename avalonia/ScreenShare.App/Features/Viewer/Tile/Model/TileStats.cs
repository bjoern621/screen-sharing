using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.Tile.Model;

/// <summary>
/// One row of the stats panel: what the figure is called, what it reads, and what it means.
///
/// The name and the meaning are fixed and the reading moves, which is the whole reason this is a live object
/// rather than a record replaced per sample.
/// A row replaced once a second is a row whose tooltip closes once a second, and the tooltip is what the
/// panel is for.
/// </summary>
public sealed class StatLine : Observable
{
    private string _value;

    public StatLine(string label, string value, string tip)
    {
        Assert.That(label.Length > 0, "a row names the figure it prints");

        Label = label;
        Tip = tip;
        _value = value;
    }

    /// <summary>The name of the figure.</summary>
    public string Label { get; }

    /// <summary>What a reading of it is evidence of, shown on the row's tooltip.</summary>
    public string Tip { get; }

    /// <summary>
    /// What it reads now, or <see cref="Figure.NoValue"/> where nothing has measured it.
    /// Never a zero standing in for an absence.
    /// </summary>
    public string Value { get => _value; set => Set(ref _value, value); }
}

/// <summary>
/// One block of the stats panel: a stage of the pipeline and the figures read off it.
///
/// The rows are a collection that is converged rather than replaced, for the reason a row's reading is
/// written rather than rebuilt.
/// Which rows a block holds changes only when the pipeline negotiates something new.
/// </summary>
public sealed class StatSection
{
    public StatSection(string heading, string tip, IEnumerable<StatLine> lines)
    {
        Assert.That(heading.Length > 0, "a block names the stage it describes");

        Heading = heading;
        Tip = tip;
        Lines = [.. lines];
    }

    /// <summary>What the block is called.</summary>
    public string Heading { get; }

    /// <summary>What this stage of the pipeline does, shown on the heading.</summary>
    public string Tip { get; }

    /// <summary>The figures, in the order they are read.</summary>
    public ObservableCollection<StatLine> Lines { get; }
}

/// <summary>
/// The stats panel, composed from one sample of a decode and one report from the window drawing it.
///
/// <b>It is a function and holds nothing.</b> A panel is the whole of what two readings say, rebuilt on every
/// pass, so there is no accumulated figure here to disagree with the backend and nothing to reset when a
/// decode is rebuilt under the same tile.
///
/// <b>The blocks follow the frames.</b> Arriving, picture, decode, render, timing, then what this window did
/// with the result - which is the order a reader walks when a tile looks wrong, because the first stage that
/// reads badly is the one to act on and everything after it is downstream of that.
///
/// <b>Nothing is computed here that the backend measured.</b> The rates are the sample's, taken against the
/// backend's own interval, so two windows on one decode read the same figure
/// (<c>api/proto/screenshare/v1/events.proto</c>, ReceiveStreamStats).
/// What this adds is the window's own block, which is the one thing a backend cannot see.
/// </summary>
public static class TileStats
{
    /// <summary>
    /// The panel for one tile.
    ///
    /// <paramref name="sample"/> is null on a tile whose decode has not been sampled yet and on one drawing
    /// something that is not a relay decode at all.
    /// Both leave the panel with the window's own block alone, which is honest: this window's counters are
    /// true whatever is producing the frames.
    /// </summary>
    public static IReadOnlyList<StatSection> Of(ReceiveStreamStats? sample, TileReport report)
    {
        var sections = new List<StatSection>(8);

        if (sample is not { } s)
        {
            Add(sections, "section.window", Window(report));
            return sections;
        }

        Add(sections, "section.stream", Arriving(s));
        Add(sections, "section.picture", Picture(s));
        Add(sections, "section.decode", Decode(s));
        Add(sections, "section.render", Render(s));
        Add(sections, "section.timing", Timing(s));
        Add(sections, "section.audio", Audio(s));
        Add(sections, "section.window", Window(report));

        // Last, because they are the deepest thing on the panel and the only part of it that depends on which
        // leg the decode was opened on.
        foreach (var group in s.Groups)
        {
            sections.Add(Transport(group));
        }

        Assert.That(sections.Count > 0, "a panel prints at least what this window knows");
        return sections;
    }

    /// <summary>
    /// Converges the panel a tile is holding onto the one its newest readings describe.
    ///
    /// <b>It writes readings into the rows that are there rather than replacing them.</b> A row is a name, a
    /// meaning and a figure, and only the figure moves between two samples of one pipeline; a panel rebuilt
    /// every second would take the row out from under a pointer that is resting on it, which closes the
    /// tooltip that row exists to show.
    ///
    /// The shape is what decides between converging and replacing, and the shape is the headings and the
    /// labels.
    /// It changes when a pipeline negotiates something it had not - an audio branch coming up, a transport
    /// element starting to keep counters - and then the block or the panel is rebuilt once and holds still
    /// again.
    /// </summary>
    public static void Merge(ObservableCollection<StatSection> held, IReadOnlyList<StatSection> built)
    {
        Assert.NotNull(held, "a merge converges the panel a tile is holding");
        Assert.NotNull(built, "a merge converges onto the panel the readings describe");

        if (!held.Select(section => section.Heading).SequenceEqual(built.Select(section => section.Heading)))
        {
            held.Clear();
            foreach (var section in built)
            {
                held.Add(section);
            }

            return;
        }

        for (var i = 0; i < held.Count; i++)
        {
            MergeLines(held[i].Lines, built[i].Lines);
        }

        Assert.That(held.Count == built.Count, "a block per block the readings describe", held.Count, built.Count);
    }

    private static void MergeLines(ObservableCollection<StatLine> held, IReadOnlyList<StatLine> built)
    {
        if (!held.Select(line => line.Label).SequenceEqual(built.Select(line => line.Label)))
        {
            held.Clear();
            foreach (var line in built)
            {
                held.Add(line);
            }

            return;
        }

        for (var i = 0; i < held.Count; i++)
        {
            held[i].Value = built[i].Value;
        }
    }

    /// <summary>
    /// Adds one block, and adds nothing where the stage it describes reported no figure at all.
    /// A heading over an empty block says a stage exists and refuses to describe it, which is worse than the
    /// block being absent: the audio block is missing because there is no track.
    /// </summary>
    private static void Add(List<StatSection> sections, string id, IReadOnlyList<StatLine> lines)
    {
        if (lines.Count == 0)
        {
            return;
        }

        var heading = Counters.Heading(id);
        sections.Add(new StatSection(heading.Label, heading.Tip, lines));
    }

    private static IReadOnlyList<StatLine> Arriving(ReceiveStreamStats s) =>
    [
        Line("codec_description", s.CodecDescription),
        Line("profile", s.Profile),
        Line("level", s.Level),
        Line("video_mbps", s.HasVideoMbps ? $"{s.VideoMbps:0.00} Mb/s" : ""),
        Line("video_fps", s.HasVideoFps ? $"{s.VideoFps:0.0} /s" : ""),
        Line("declared_fps", Declared(s.FpsNum, s.FpsDen)),
        Line("keyframes", Count(s.Keyframes)),
        Line("since_keyframe_sec", s.HasSinceKeyframeSec ? $"{s.SinceKeyframeSec:0.0} s ago" : ""),
        Line("video_bytes", Bytes(s.VideoBytes)),
    ];

    private static IReadOnlyList<StatLine> Picture(ReceiveStreamStats s) =>
    [
        Line("picture_size", Pixels(s.Width, s.Height)),
        Line("pixel_format", s.PixelFormat),
        Line("depth", s.Depth > 0 ? $"{s.Depth} bits" : ""),
        Line("subsampling", s.Subsampling),
        Line("colorimetry", s.Colorimetry),
        Line("transfer", s.Transfer.Length > 0 ? Words.Transfer(s.Transfer) : ""),
        Line("chroma_site", s.ChromaSite),
        Line("pixel_aspect", s.PixelAspect),
        Line("interlace", s.Interlace),
    ];

    private static IReadOnlyList<StatLine> Decode(ReceiveStreamStats s) =>
    [
        Line("decoder", s.Decoder.Length > 0
            ? $"{s.Decoder}, {(s.Hardware ? "on the GPU" : "on the CPU")}"
            : ""),
        Line("decode_memory", s.DecodeMemory.Length > 0 ? Words.FrameMemory(s.DecodeMemory) : ""),

        // Stated in both directions rather than only when it is on.
        // A reader comparing two tiles of one HDR stream is comparing exactly this, and a row that
        // disappeared when the answer was "no" would leave them unable to tell which tile they are looking
        // at.
        Line("tone_map", s.ToneMap ? "on" : "off"),
    ];

    private static IReadOnlyList<StatLine> Render(ReceiveStreamStats s) =>
    [
        Line("chain", s.Chain.Length > 0 ? Words.RenderChain(s.Chain) : ""),
        Line("render_memory", s.RenderMemory.Length > 0 ? Words.FrameMemory(s.RenderMemory) : ""),
        Line("render_format", Joined(s.RenderFormat, s.RenderColorimetry)),
        Line("render_size", Pixels(s.RenderWidth, s.RenderHeight)),
        Line("render_fps", s.HasRenderFps ? $"{s.RenderFps:0.0} /s" : ""),
        Line("rendered", Count(s.Rendered)),
        Line("sink_dropped", Count(s.Dropped)),
    ];

    private static IReadOnlyList<StatLine> Timing(ReceiveStreamStats s) =>
    [
        Line("live", s.Live ? "yes" : "no"),
        Line("latency", Latency(s)),
        Line("position_sec", s.HasPositionSec ? Clock(s.PositionSec) : ""),
        Line("uptime_sec", Clock(s.UptimeSec)),
    ];

    /// <summary>
    /// The sound track, and nothing at all where the pipeline built no audio branch.
    ///
    /// Emptiness is the test rather than a flag, because it is the same fact: the branch is what fills every
    /// field here, so a stream published without a track and one whose branch has not come up yet both read
    /// as no block.
    /// Which of the two it is belongs to the tile, which draws a meter for the first and none for the second.
    /// </summary>
    private static IReadOnlyList<StatLine> Audio(ReceiveStreamStats s)
    {
        if (s.AudioCodecDescription.Length == 0 && s.AudioRate == 0)
        {
            return [];
        }

        return
        [
            Line("audio_codec_description", s.AudioCodecDescription),
            Line("audio_decoder", s.AudioDecoder),
            Line("audio_format", s.AudioFormat),
            Line("audio_rate", s.AudioRate > 0 ? $"{s.AudioRate} Hz" : ""),
            Line("audio_channels", s.AudioChannels > 0 ? $"{s.AudioChannels}" : ""),
            Line("audio_kbps", s.HasAudioKbps ? $"{s.AudioKbps:0} kb/s" : ""),
            Line("audio_bytes", Bytes(s.AudioBytes)),
        ];
    }

    /// <summary>
    /// What this window got and drew.
    /// It is always drawn, on every kind of tile, because it is true of every kind: these are the frame
    /// channel's counters and they do not depend on what is producing the pictures.
    /// </summary>
    private static IReadOnlyList<StatLine> Window(TileReport report) =>
    [
        Line("window.size", Pixels(report.Width, report.Height)),
        Line("window.frames", Count(report.Frames)),
        Line("window.dropped", Count(report.Dropped)),
    ];

    /// <summary>One element of the transport and the counters it keeps, in the element's order.</summary>
    private static StatSection Transport(ReceiveStatGroup group)
    {
        Assert.That(group.Values.Count > 0, "a reported element carries at least one counter", group.Element);

        var element = Counters.Element(group.Factory);

        // The element's own name, beside what it is.
        // It is what tells two jitterbuffers of one muxed stream apart, and it is the name to look for in a
        // pipeline dump.
        return new StatSection(
            $"{element.Label} · {group.Element}",
            element.Tip,
            group.Values.Select(Line));
    }

    /// <summary>
    /// One row.
    /// An empty value is what a figure nothing has measured looks like on the way in, and it prints as absent
    /// rather than as a zero: the pads negotiate after the pipeline is built, so every field here is empty
    /// for a moment on every decode.
    /// </summary>
    private static StatLine Line(string key, string value)
    {
        var counter = Counters.Field(key);
        return new StatLine(counter.Label, value.Length > 0 ? value : Figure.NoValue, counter.Tip);
    }

    /// <summary>One transport counter, whose key is the element's own name for it.</summary>
    private static StatLine Line(ReceiveStatValue value) => Line(value.Key, Counter(value.Key, value.Value));

    /// <summary>
    /// One of an element's counters.
    /// The unit follows from the key, because that is where the unit is stated in the first place: an element
    /// that reports a figure in milliseconds names the field for it.
    /// </summary>
    private static string Counter(string key, double value)
    {
        if (key.EndsWith("-ms", StringComparison.Ordinal))
        {
            return $"{value:0.0} ms";
        }

        if (key.EndsWith("-mbps", StringComparison.Ordinal))
        {
            return $"{value:0.00} Mb/s";
        }

        return value.ToString("N0");
    }

    /// <summary>A picture size, and nothing before the pads have negotiated one.</summary>
    private static string Pixels(int width, int height) =>
        width > 0 && height > 0 ? $"{width}×{height}" : "";

    /// <summary>
    /// The frame rate the caps declare.
    /// A denominator of zero is a stream that declares a variable rate, which is a different fact from
    /// declaring none and is worth saying.
    /// </summary>
    private static string Declared(int num, int den)
    {
        if (num <= 0)
        {
            return "";
        }

        return den > 0 ? $"{(double)num / den:0.##} /s" : "variable";
    }

    /// <summary>
    /// The latency window as one reading, since the two ends of it are only meaningful together.
    /// A window with no upper end is a pipeline that will hold a frame for as long as it takes, which is
    /// worth its own word rather than a very large number.
    /// </summary>
    private static string Latency(ReceiveStreamStats s)
    {
        if (!s.HasLatencyMinMs)
        {
            return "";
        }

        return s.HasLatencyMaxMs && s.LatencyMaxMs > s.LatencyMinMs
            ? $"{s.LatencyMinMs:0} to {s.LatencyMaxMs:0} ms"
            : $"{s.LatencyMinMs:0} ms";
    }

    /// <summary>Two caps fields that read as one answer, with either half possibly absent.</summary>
    private static string Joined(string first, string second) =>
        string.Join(" ", new[] { first, second }.Where(part => part.Length > 0));

    /// <summary>A count, grouped, because these reach seven digits within the hour.</summary>
    private static string Count(ulong value) => value.ToString("N0");

    /// <summary>
    /// A byte total at the scale it is worth reading at.
    /// Decimal rather than binary multiples, because the figure beside it is a bitrate and the two are
    /// compared.
    /// </summary>
    private static string Bytes(ulong value) => value switch
    {
        0 => "",
        >= 1_000_000_000 => $"{value / 1_000_000_000d:0.00} GB",
        >= 1_000_000 => $"{value / 1_000_000d:0.0} MB",
        >= 1_000 => $"{value / 1_000d:0.0} kB",
        _ => $"{value} B",
    };

    /// <summary>
    /// A running time, as a clock reads it. Two of these sit beside each other and are compared
    /// - a position that has stopped against an uptime that has not - so both are spelled the
    /// same way rather than one in seconds and one in hours.
    /// </summary>
    private static string Clock(double seconds)
    {
        var span = TimeSpan.FromSeconds(seconds);
        return span.TotalHours >= 1
            ? $"{(int)span.TotalHours}:{span.Minutes:00}:{span.Seconds:00}"
            : $"{span.Minutes}:{span.Seconds:00}";
    }
}
