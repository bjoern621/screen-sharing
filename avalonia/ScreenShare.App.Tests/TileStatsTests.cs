using System.Collections.ObjectModel;
using ScreenShare.Api.V1;
using ScreenShare.App.Features.Viewer.Tile.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Two defects the panel is guarded against.
/// An unmeasured figure printed as a zero reads as a decode receiving nothing, and every row spends the
/// seconds a stream is opening in that state.
/// A row with no tip is a number a reader cannot act on, and a key with no entry renders as the raw key with
/// nothing else to catch it.
/// </summary>
public sealed class TileStatsTests
{
    /// <summary>
    /// One sample off a pipeline that has negotiated everything and been measured at least twice.
    /// A test about an absence takes a field back off it, so the line that differs is what the test is
    /// about.
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

        // A tile on the machine that is publishing this stream, which is the one place every stage but the
        // relay's carries a figure.
        Delay = new DelayBudget
        {
            PublishMs = 8.4,
            PublishLinkMs = 300,
            WatchLinkMs = 120,
            ReceiveMs = 6.2,
            PresentMs = 13.8,
            TotalMs = 448.4,
        },
    };

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
            ["Arriving", "Picture", "Decode", "Render", "Timing", "Delay", "Audio", "This window"],
            panel.Select(section => section.Heading));
    }

    /// <summary>
    /// The delay block is the whole path in the order a frame crosses it, and it keeps the relay's stage even
    /// though nothing measures it.
    /// Dropping that row would present the total as the whole journey, and the total is short by exactly it.
    /// The decode's worst frame is the one row that is not a stage, and it sits under the mean it is read
    /// against rather than in a block of its own.
    /// </summary>
    [Fact]
    public void TheDelayBlockNamesEveryStageOfThePath()
    {
        var panel = TileStats.Of(Sample(), Report());

        Assert.Equal(
            [
                "Capture and encode", "Publisher to relay", "Through the relay", "Relay to here", "Decode",
                "Decode, worst", "Held for play time", "At least, end to end",
            ],
            Section(panel, "Delay").Lines.Select(line => line.Label));

        Assert.Equal("8.4 ms", Value(panel, "Delay", "Capture and encode"));
        Assert.Equal("300 ms", Value(panel, "Delay", "Publisher to relay"));
        Assert.Equal("448 ms", Value(panel, "Delay", "At least, end to end"));

        // The one row with no figure behind it, and the reason the total is a floor.
        Assert.Equal("…", Value(panel, "Delay", "Through the relay"));
    }

    /// <summary>
    /// A viewer watching somebody else's stream cannot see the publishing side at all: nothing carries those
    /// stages over the relay, and inventing them would put a number on the one part of the path this machine
    /// has no reading of.
    /// </summary>
    [Fact]
    public void AStreamFromAnotherMachineShowsNoPublishingStages()
    {
        var remote = Sample();
        remote.Delay = new DelayBudget { WatchLinkMs = 120, ReceiveMs = 6.2, PresentMs = 13.8, TotalMs = 140 };

        var panel = TileStats.Of(remote, Report());

        Assert.Equal("…", Value(panel, "Delay", "Capture and encode"));
        Assert.Equal("…", Value(panel, "Delay", "Publisher to relay"));
        Assert.Equal("140 ms", Value(panel, "Delay", "At least, end to end"));
    }

    /// <summary>
    /// Rows are keyed on identifiers the two sides own, and a missing entry renders as the identifier, which
    /// only a reader on that row would see.
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

    [Fact]
    public void TheIdentifiersTheBackendSendsArePrintedAsWords()
    {
        var panel = TileStats.Of(Sample(), Report());

        Assert.Equal("HDR (PQ)", Value(panel, "Picture", "Transfer"));
        Assert.Equal("the GPU, dmabuf", Value(panel, "Decode", "Decoded into"));
        Assert.Equal("the GPU, OpenGL", Value(panel, "Render", "Handed over in"));
        Assert.Contains("OpenGL", Value(panel, "Render", "Render chain"));
    }

    /// <summary>
    /// A decode on its first tick has counters, no rates and nothing the pads have not negotiated.
    /// None of that is a measurement of zero.
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

        // A counter that has counted nothing is a reading of zero, not an absence.
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
    /// Two tiles of one HDR stream are told apart by this row, so it cannot vanish when the answer is off.
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
    /// The evidence separating a stream this machine decodes too slowly from one the network is not
    /// delivering.
    /// Which counters a decode reports follows from the leg it was opened on.
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
    /// The raw key is a row a reader can search for and report, where swallowing it would leave a diagnostic
    /// short of evidence.
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
    /// The window's counters are its own: one frame channel, whether pictures come off the relay, the
    /// publish's loopback copy or a screen this machine reads.
    /// </summary>
    [Fact]
    public void TheWindowsOwnBlockIsDrawnWithNoSampleAtAll()
    {
        var panel = TileStats.Of(null, Report());

        Assert.Equal(["This window"], panel.Select(section => section.Heading));
        Assert.Equal("3", Value(panel, "This window", "Dropped waiting for this window"));
    }

    /// <summary>
    /// What makes the tooltips usable: a panel rebuilt per sample takes the row out from under a resting
    /// pointer once a second, closing the tip the row exists to show.
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

    /// <summary>An audio branch coming up is a whole block that was not there a second ago.</summary>
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

    /// <summary>
    /// A second is short enough for a healthy decode to measure none of a per-interval figure, and a column
    /// alternating between a number and an ellipsis is one nobody can read.
    /// The row keeps the last measurement instead, which is what the reader was looking at.
    /// </summary>
    [Fact]
    public void ARowKeepsItsLastMeasurementThroughAPassThatMeasuredNone()
    {
        var panel = new ObservableCollection<StatSection>();
        TileStats.Merge(panel, TileStats.Of(Sample(), Report()));

        Assert.Equal("24.50 Mb/s", Value(panel, "Arriving", "Bitrate"));

        var quiet = Sample();
        quiet.ClearVideoMbps();
        TileStats.Merge(panel, TileStats.Of(quiet, Report()));

        Assert.Equal("24.50 Mb/s", Value(panel, "Arriving", "Bitrate"));
    }

    /// <summary>
    /// A decode that stopped is a different panel, so nothing it measured stands under the one that follows.
    /// </summary>
    [Fact]
    public void ADecodeThatStoppedKeepsNothing()
    {
        var panel = new ObservableCollection<StatSection>();
        TileStats.Merge(panel, TileStats.Of(Sample(), Report()));
        TileStats.Merge(panel, TileStats.Of(null, Report()));

        Assert.Equal(["This window"], panel.Select(section => section.Heading));
    }
}
