using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Contracts;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Broadcast.Model;
using ScreenShare.App.Mvvm;

namespace ScreenShare.App.Features.Viewer.Tile.Model;

/// <summary>
/// One row of the stats panel: the figure's name, its reading, and what it means.
/// Name and meaning are fixed and only the reading moves, so a row is a live object rather than a record replaced
/// per sample.
/// Replacing a row once a second closes its tooltip once a second, and the tooltip is why the panel exists.
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

    public string Label { get; }

    /// <summary>What a reading of it is evidence of, shown on the row's tooltip.</summary>
    public string Tip { get; }

    /// <summary>
    /// What it reads, or <see cref="Figure.NoValue"/> where nothing has measured it.
    /// A zero never stands in for an absence.
    /// </summary>
    public string Value { get => _value; set => Set(ref _value, value); }
}

/// <summary>
/// One block of the panel: one stage of the pipeline and the figures read off it.
/// Rows are converged rather than replaced, as a reading is written rather than rebuilt.
/// Which rows a block holds moves only when the pipeline negotiates something it had not.
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

    public string Heading { get; }

    /// <summary>What this stage of the pipeline does, shown on the heading.</summary>
    public string Tip { get; }

    /// <summary>Figures, in the order they are read off the pipeline.</summary>
    public ObservableCollection<StatLine> Lines { get; }
}

/// <summary>
/// The panel, composed from one sample of a decode and the report of the window drawing it.
///
/// <see cref="Of"/> holds nothing: a panel is the whole of what two readings say, built again per pass,
/// so no figure is accumulated here and none can disagree with the backend.
/// What a tile keeps between passes is <see cref="Merge"/>'s, a row's last measurement and not a figure of its own.
///
/// Blocks run in the order the frames do, the order a reader walks when a tile looks wrong:
/// the first stage that reads badly is the one to act on and everything after it is downstream.
///
/// Nothing is computed here that the backend measured.
/// The rates are the sample's, taken against the backend's own interval,
/// so two windows on one decode read one figure, and a rate is absent rather than zero on the first sample of a run
/// (<c>api/proto/screenshare/v1/events.proto</c>, ReceiveStreamStats).
/// Added here is the window's own block, the one thing a backend cannot see.
/// </summary>
public static class TileStats
{
    /// <summary>
    /// One tile's panel.
    /// <paramref name="sample"/> is null on an unsampled decode and on a tile drawing no relay decode, both
    /// leaving the window's own block alone, whose counters hold whatever produces the frames.
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
        Add(sections, "section.delay", Delay(s));
        Add(sections, "section.audio", Audio(s));
        Add(sections, "section.window", Window(report));

        // Last, being the deepest figures and the only ones depending on the leg the decode was opened on.
        foreach (var group in s.Groups)
        {
            sections.Add(Transport(group));
        }

        Assert.That(sections.Count > 0, "a panel prints at least what this window knows");
        return sections;
    }

    /// <summary>
    /// Converges the panel a tile holds onto the one its newest readings describe.
    ///
    /// Readings are written into the rows that are there rather than replacing them, only the figure moving
    /// between two samples of one pipeline.
    ///
    /// Converging or replacing is decided on the shape, the headings and the labels.
    /// The shape moves when a pipeline negotiates something it had not, an audio branch coming up or a transport
    /// element starting to keep counters, and the block or the panel is rebuilt once and holds still.
    ///
    /// A row the newest reading measured nothing for keeps what the last one measured.
    /// A second is short enough for a healthy decode to measure none of a per-interval figure, and a column
    /// alternating between a number and an ellipsis is one nobody can read.
    /// What ends a held value is the shape moving, what a decode that stopped or was rebuilt does to the panel.
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
            if (built[i].Value != Figure.NoValue)
            {
                held[i].Value = built[i].Value;
            }
        }
    }

    /// <summary>
    /// Adds one block, and nothing where the stage it describes reported no figure at all.
    /// A heading over an empty block says a stage exists and refuses to describe it: the audio block is absent
    /// where there is no track.
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

        // In the block naming the stage that does it, ahead of Render,
        // so the panel reads as the funnel it is: arriving, shed here, drawn.
        Line("discarded_fps", s.HasDiscardedFps ? $"{s.DiscardedFps:0.0} /s" : ""),

        // What the pipeline was built with, never what was asked for.
        // Printed in both directions: a row that vanished on "off" would leave two tiles of one HDR stream
        // indistinguishable.
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
    /// Path a frame took, stage by stage, in the order it crossed them.
    ///
    /// Which stages carry a figure is the backend's answer and not this side's:
    /// a viewer measures its own leg and its own pipeline,
    /// and sees the publishing machine's own work only where it is that machine too
    /// (<c>api/proto/screenshare/v1/events.proto</c>, DelayBudget).
    ///
    /// One stage the budget carries is drawn nowhere, the way between the two machines timed as a whole.
    /// The relay's row derives from it and the total counts it in place of the legs it covers, so a row of its own
    /// would print the same milliseconds a third time.
    ///
    /// One row is not a stage: the decode's worst single frame sits directly under the decode's mean,
    /// read against it and against the sink's deadline rather than from memory two blocks away.
    /// </summary>
    private static IReadOnlyList<StatLine> Delay(ReceiveStreamStats s)
    {
        if (s.Delay is not { } d)
        {
            return [];
        }

        return
        [
            Line("delay.publish", Ms(d.HasPublishMs, d.PublishMs)),
            Line("delay.publish_link", Ms(d.HasPublishLinkMs, d.PublishLinkMs)),
            Line("delay.relay", Ms(d.HasRelayMs, d.RelayMs)),
            Line("delay.watch_link", Ms(d.HasWatchLinkMs, d.WatchLinkMs)),
            Line("delay.receive", Ms(d.HasReceiveMs, d.ReceiveMs)),
            Line("delay.receive_peak", Ms(d.HasReceivePeakMs, d.ReceivePeakMs)),
            Line("delay.present", Ms(d.HasPresentMs, d.PresentMs)),
            Line("delay.total", Ms(d.HasTotalMs, d.TotalMs)),
        ];
    }

    /// <summary>
    /// One stage of the path in milliseconds, and nothing at all where nothing measured it.
    /// One decimal under ten milliseconds and whole ones above:
    /// a stage of a fifth of a millisecond and one of half a second sit in one column,
    /// read against each other.
    /// </summary>
    private static string Ms(bool measured, double value)
    {
        if (!measured)
        {
            return "";
        }

        return value < 10 ? $"{value:0.0} ms" : $"{value:0} ms";
    }

    /// <summary>
    /// Sound track, and no block at all where the pipeline built no audio branch.
    /// Emptiness is the test rather than a flag, the branch filling every field here:
    /// a stream published without a track and one whose branch has not come up read alike.
    /// Telling the two apart is the tile's.
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
    /// What this window got and drew, the counts being totals since the subscription opened.
    /// Drawn on every kind of tile, these being the frame channel's counters,
    /// independent of what produces the pictures.
    /// </summary>
    private static IReadOnlyList<StatLine> Window(TileReport report) =>
    [
        Line("window.size", Pixels(report.Width, report.Height)),
        Line("window.frames", Count(report.Frames)),
        Line("window.dropped", Count(report.Dropped)),
    ];

    /// <summary>One element of the transport and the counters it keeps, in the element's own order.</summary>
    private static StatSection Transport(ReceiveStatGroup group)
    {
        Assert.That(group.Values.Count > 0, "a reported element carries at least one counter", group.Element);

        var element = Counters.Element(group.Factory);

        // The element's own name beside what it is: what tells two jitterbuffers of one muxed stream apart,
        // and the name to look for in a pipeline dump.
        return new StatSection(
            $"{element.Label} · {group.Element}",
            element.Tip,
            group.Values.Select(Line));
    }

    /// <summary>
    /// One figure's row.
    /// An empty value is a figure nothing measured, printed as absent rather than as a zero: the pads negotiate
    /// after the pipeline is built, so every field here is empty for a moment on every decode.
    /// </summary>
    private static StatLine Line(string key, string value)
    {
        var counter = Counters.Field(key);
        return new StatLine(counter.Label, value.Length > 0 ? value : Figure.NoValue, counter.Tip);
    }

    /// <summary>One transport counter, keyed as the element names it.</summary>
    private static StatLine Line(ReceiveStatValue value) => Line(value.Key, Counter(value.Key, value.Value));

    /// <summary>
    /// One of an element's counters, the unit read off the key:
    /// "-ms" milliseconds, "-mbps" Mb/s, anything else a plain count.
    /// An element names its fields for what they report, so the key is where a unit is stated at all.
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

    /// <summary>A picture size in pixels, and nothing before the pads negotiate one.</summary>
    private static string Pixels(int width, int height) =>
        width > 0 && height > 0 ? $"{width}×{height}" : "";

    /// <summary>
    /// Frame rate the caps declare, per second, a claim rather than a measurement.
    /// A denominator of zero declares a variable rate, a different fact from declaring none.
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
    /// Latency window in milliseconds as one reading, the two ends being meaningful only together.
    /// Where no upper end was reported, the lower one stands alone rather than beside an invented bound.
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

    /// <summary>Two caps fields that read as one answer, either half possibly absent.</summary>
    private static string Joined(string first, string second) =>
        string.Join(" ", new[] { first, second }.Where(part => part.Length > 0));

    /// <summary>A running total, grouped, these reaching seven digits within the hour.</summary>
    private static string Count(ulong value) => value.ToString("N0");

    /// <summary>
    /// A byte total at a scale worth reading.
    /// Decimal multiples rather than binary, being read beside a bitrate.
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
    /// Seconds as a clock reads them.
    /// Position and uptime sit beside each other and are compared, a stalled position against an uptime that runs
    /// on, so both are spelled the same way.
    /// </summary>
    private static string Clock(double seconds)
    {
        var span = TimeSpan.FromSeconds(seconds);
        return span.TotalHours >= 1
            ? $"{(int)span.TotalHours}:{span.Minutes:00}:{span.Seconds:00}"
            : $"{span.Minutes}:{span.Seconds:00}";
    }
}
