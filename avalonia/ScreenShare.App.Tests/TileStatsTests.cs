using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Features.Viewer.Tile.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The stats panel: what a tile prints when a reader turns it on to diagnose one stream.
///
/// Two defects are what these lock out.
/// A figure nothing has measured printing as a zero, which would read as a decode receiving nothing rather
/// than as a pipeline that has not negotiated - the whole panel is read during the seconds a stream is
/// opening, so every row spends time in that state.
/// And a row with no tip, which is a number a reader cannot act on: the keys are the contract's own field
/// names, so a row whose key is misspelt renders as the raw key with nothing said about it, and nothing else
/// would catch that.
/// </summary>
public sealed class TileStatsTests
{
    /// <summary>
    /// One decode's sample, as the backend takes it off a running pipeline that has negotiated everything and
    /// been measured at least twice.
    /// Tests that are about an absence take a field back off it rather than building a second fixture, so
    /// what each of them is about is the line that differs.
    /// </summary>
    private static ReceiveStreamStats Sample() => new()
    {
        Stream = new WatchKey { StreamName = "desk", Transport = "srt" },

        CodecDescription = "H.265 (Main 4:4:4 profile)",
        Profile = "main-444",
        Level = "5.1",
        VideoBytes = 2_400_000_000,
        VideoFrames = 216_000,
        Keyframes = 1_800,
        SinceKeyframeSec = 1.4,
        VideoMbps = 24.5,
        VideoFps = 59.9,

        Width = 2560,
        Height = 1440,
        PixelFormat = "Y444_10LE",
        Depth = 10,
        Subsampling = "4:4:4",
        Colorimetry = "1:1:5:1",
        Transfer = "smpte2084",
        ChromaSite = "none",
        PixelAspect = "1:1",
        Interlace = "progressive",
        FpsNum = 60,
        FpsDen = 1,

        Decoder = "vah265dec",
        Hardware = true,
        DecodeMemory = "memory:DMABuf",
        RenderMemory = "memory:GLMemory",
        Chain = "gl",
        ToneMap = true,
        RenderFormat = "RGBA",
        RenderColorimetry = "1:1:13:1",
        RenderWidth = 1280,
        RenderHeight = 720,
        Frames = 215_400,
        Rendered = 215_390,
        Dropped = 10,
        RenderFps = 59.8,

        Live = true,
        LatencyMinMs = 20,
        LatencyMaxMs = 220,
        PositionSec = 3_600,
        UptimeSec = 3_602,

        AudioCodecDescription = "Opus",
        AudioDecoder = "opusdec",
        AudioFormat = "F32LE",
        AudioRate = 48_000,
        AudioChannels = 2,
        AudioBytes = 27_000_000,
        AudioKbps = 96,
    };

    /// <summary>What one window reports about the frames it was handed and what it drew.</summary>
    private static TileReport Report() => new(1280, 720, Frames: 215_390, Dropped: 3, Notice: "");

    private static StatSection Section(IReadOnlyList<StatSection> panel, string heading) =>
        panel.Single(section => section.Heading == heading);

    private static string Value(IReadOnlyList<StatSection> panel, string heading, string label) =>
        Section(panel, heading).Lines.Single(line => line.Label == label).Value;

    [Fact]
    public void TheBlocksFollowTheFramesThroughThePipeline()
    {
        var panel = TileStats.Of(Sample(), Report());

        Assert.Equal(
            ["Arriving", "Picture", "Decode", "Render", "Timing", "Audio", "This window"],
            panel.Select(section => section.Heading));
    }

    /// <summary>
    /// The guard on the tables.
    /// Every row is keyed on an identifier one of the two sides owns, and an entry that is missing renders as
    /// that identifier with nothing said about it - which is visible on screen but only to somebody looking
    /// at that row.
    /// </summary>
    [Fact]
    public void EveryRowAndEveryHeadingSaysWhatItMeans()
    {
        var sample = Sample();
        sample.Groups.Add(SrtGroup());

        foreach (var section in TileStats.Of(sample, Report()))
        {
            Assert.False(string.IsNullOrWhiteSpace(section.Tip), $"heading '{section.Heading}' explains nothing");
            foreach (var line in section.Lines)
            {
                Assert.False(string.IsNullOrWhiteSpace(line.Tip), $"row '{line.Label}' explains nothing");
            }
        }
    }

    [Fact]
    public void TheFiguresArePrintedInTheUnitTheyAreMeasuredIn()
    {
        var panel = TileStats.Of(Sample(), Report());

        Assert.Equal("24.50 Mb/s", Value(panel, "Arriving", "Bitrate"));
        Assert.Equal("59.9 /s", Value(panel, "Arriving", "Frames arriving"));
        Assert.Equal("60 /s", Value(panel, "Arriving", "Declared rate"));
        Assert.Equal("2.40 GB", Value(panel, "Arriving", "Video received"));
        Assert.Equal("2560×1440", Value(panel, "Picture", "Size"));
        Assert.Equal("1280×720", Value(panel, "Render", "Drawn at"));
        Assert.Equal("20 to 220 ms", Value(panel, "Timing", "Latency window"));
        Assert.Equal("1:00:00", Value(panel, "Timing", "Position"));
        Assert.Equal("96 kb/s", Value(panel, "Audio", "Bitrate"));
    }

    /// <summary>
    /// The identifiers the backend sends are named in this shell's own words, which is the same rule every
    /// other identifier on the contract follows: the decode reports "smpte2084" and "memory:DMABuf", and
    /// neither is a word for a reader.
    /// </summary>
    [Fact]
    public void TheIdentifiersTheBackendSendsArePrintedAsWords()
    {
        var panel = TileStats.Of(Sample(), Report());

        Assert.Equal("HDR (PQ)", Value(panel, "Picture", "Transfer"));
        Assert.Equal("the GPU, dmabuf", Value(panel, "Decode", "Decoded into"));
        Assert.Equal("the GPU, OpenGL", Value(panel, "Render", "Reached the sink in"));
        Assert.Contains("OpenGL", Value(panel, "Render", "Render chain"));
    }

    /// <summary>
    /// A decode on its first tick has counters and no rates, and every field the pads have not negotiated is
    /// empty.
    /// None of that is a measurement of zero, and a panel that printed it as one would say the stream is
    /// arriving at nothing.
    /// </summary>
    [Fact]
    public void AFigureNothingHasMeasuredPrintsAsAbsent()
    {
        var opening = new ReceiveStreamStats
        {
            Stream = new WatchKey { StreamName = "desk", Transport = "srt" },
            UptimeSec = 0.4,
        };

        var panel = TileStats.Of(opening, new TileReport(0, 0, 0, 0, ""));

        Assert.Equal("…", Value(panel, "Arriving", "Bitrate"));
        Assert.Equal("…", Value(panel, "Arriving", "Codec"));
        Assert.Equal("…", Value(panel, "Picture", "Size"));
        Assert.Equal("…", Value(panel, "Timing", "Latency window"));
        Assert.Equal("…", Value(panel, "This window", "Handed over at"));

        // A counter that has counted nothing has counted zero, which is a reading rather than an absence and
        // is drawn as one.
        Assert.Equal("0", Value(panel, "This window", "Frames taken"));
    }

    [Fact]
    public void AStreamWithNoSoundTrackHasNoAudioBlock()
    {
        var silent = Sample();
        silent.AudioCodecDescription = "";
        silent.AudioDecoder = "";
        silent.AudioFormat = "";
        silent.AudioRate = 0;
        silent.AudioChannels = 0;
        silent.AudioBytes = 0;
        silent.ClearAudioKbps();

        var panel = TileStats.Of(silent, Report());

        Assert.DoesNotContain(panel, section => section.Heading == "Audio");
    }

    /// <summary>
    /// Whether the range is being rolled down is drawn in both directions.
    /// A reader comparing two tiles of one HDR stream is comparing exactly this row, so a row that vanished
    /// when the answer was "off" would leave them unable to tell the tiles apart.
    /// </summary>
    [Fact]
    public void ToneMappingIsStatedWhicheverWayItWent()
    {
        var untouched = Sample();
        untouched.ToneMap = false;

        Assert.Equal("on", Value(TileStats.Of(Sample(), Report()), "Decode", "Tone mapping"));
        Assert.Equal("off", Value(TileStats.Of(untouched, Report()), "Decode", "Tone mapping"));
    }

    /// <summary>
    /// The transport's counters, which are the evidence that separates a stream this machine cannot decode
    /// fast enough from one the network is not delivering.
    /// Which of them a decode reports follows from the leg it was opened on.
    /// </summary>
    [Fact]
    public void TheTransportsOwnCountersBecomeABlockPerElement()
    {
        var sample = Sample();
        sample.Groups.Add(SrtGroup());

        var panel = TileStats.Of(sample, Report());
        var link = panel[^1];

        Assert.Contains("SRT link", link.Heading);
        Assert.Contains("srtsrc0", link.Heading);
        Assert.Equal("Lost", link.Lines[1].Label);
        Assert.Equal("41", link.Lines[1].Value);
        Assert.Equal("18.5 ms", link.Lines[2].Value);
        Assert.Equal("24.90 Mb/s", link.Lines[3].Value);
    }

    /// <summary>
    /// A counter this build has no words for still reaches the panel, under the element's own name for it.
    /// It is a row a reader can search for and report, where swallowing it would leave a diagnostic quietly
    /// missing evidence.
    /// </summary>
    [Fact]
    public void ACounterThisBuildHasNoWordsForIsPrintedUnderItsOwnKey()
    {
        var sample = Sample();
        var group = new ReceiveStatGroup { Factory = "srtsrc", Element = "srtsrc0" };
        group.Values.Add(new ReceiveStatValue { Key = "packets-sent-unique", Value = 7 });
        sample.Groups.Add(group);

        var panel = TileStats.Of(sample, Report());

        Assert.Equal("packets-sent-unique", panel[^1].Lines[0].Label);
        Assert.Equal("7", panel[^1].Lines[0].Value);
    }

    /// <summary>
    /// A tile with no sample still prints what this window did, because those counters are the window's own:
    /// the frame channel is the same whether the pictures come off the relay, off the publish's own loopback
    /// copy, or off a screen this machine is reading.
    /// </summary>
    [Fact]
    public void TheWindowsOwnBlockIsDrawnWithNoSampleAtAll()
    {
        var panel = TileStats.Of(null, Report());

        Assert.Equal(["This window"], panel.Select(section => section.Heading));
        Assert.Equal("3", Value(panel, "This window", "Dropped waiting for this window"));
    }

    /// <summary>
    /// A row survives the sample that lands under it, and takes the new reading.
    ///
    /// This is what makes the tooltips usable at all.
    /// A panel rebuilt on every sample takes the row out from under a resting pointer once a second, which
    /// closes the tip that row exists to show - and a tip nobody can finish reading is a tip nobody reads.
    /// </summary>
    [Fact]
    public void ARowSurvivesTheSampleThatLandsUnderIt()
    {
        var panel = new ObservableCollection<StatSection>();
        TileStats.Merge(panel, TileStats.Of(Sample(), Report()));

        var section = panel[0];
        var row = section.Lines[0];

        var moved = Sample();
        moved.VideoMbps = 31.5;
        TileStats.Merge(panel, TileStats.Of(moved, Report()));

        Assert.Same(section, panel[0]);
        Assert.Same(row, panel[0].Lines[0]);
        Assert.Equal("31.50 Mb/s", Value(panel, "Arriving", "Bitrate"));
    }

    /// <summary>
    /// A pipeline that negotiates something it had not rebuilds the block that gained rows, and nothing else
    /// about the panel.
    /// An audio branch coming up is the case: it is a whole block that was not there a second ago.
    /// </summary>
    [Fact]
    public void ABlockTheDecodeGainsIsBuiltWhenItArrives()
    {
        var silent = Sample();
        silent.AudioCodecDescription = "";
        silent.AudioDecoder = "";
        silent.AudioFormat = "";
        silent.AudioRate = 0;
        silent.AudioChannels = 0;
        silent.AudioBytes = 0;
        silent.ClearAudioKbps();

        var panel = new ObservableCollection<StatSection>();
        TileStats.Merge(panel, TileStats.Of(silent, Report()));
        Assert.DoesNotContain(panel, section => section.Heading == "Audio");

        TileStats.Merge(panel, TileStats.Of(Sample(), Report()));
        Assert.Equal("Opus", Value(panel, "Audio", "Codec"));
    }

    private static ReceiveStatGroup SrtGroup()
    {
        var group = new ReceiveStatGroup { Factory = "srtsrc", Element = "srtsrc0" };
        group.Values.Add(new ReceiveStatValue { Key = "packets-received", Value = 1_402_331 });
        group.Values.Add(new ReceiveStatValue { Key = "packets-received-lost", Value = 41 });
        group.Values.Add(new ReceiveStatValue { Key = "rtt-ms", Value = 18.53 });
        group.Values.Add(new ReceiveStatValue { Key = "receive-rate-mbps", Value = 24.9 });
        return group;
    }
}
