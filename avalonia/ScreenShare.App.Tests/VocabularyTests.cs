using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What an entry is called, where its own value does not say enough.
///
/// Defect locked out: one name printed twice.
/// The backend identifies an option by its value alone and leaves the naming to the shell,
/// and one screen is read by both publish engines, so the source name alone offers the same entry twice.
/// The separating fact is a column of the catalog, so a name is composed against the table and not off the row.
/// </summary>
public sealed class VocabularyTests
{
    /// <summary>
    /// Catalog rows a name is composed from.
    /// Only the columns the naming reads are set, so a test about naming asserts nothing about availability.
    /// </summary>
    private static Vocabulary Words(params object[] rows)
    {
        var catalog = new Catalog();
        foreach (var row in rows)
        {
            switch (row)
            {
                case VideoCodec codec:
                    catalog.Codecs.Add(codec);
                    break;
                case CaptureBackend capture:
                    catalog.Captures.Add(capture);
                    break;
            }
        }

        return new Vocabulary(catalog);
    }

    private static CaptureBackend Capture(string name, Engine engine) =>
        new() { Name = name, Engine = engine };

    private static VideoCodec Codec(string name, string format, string family) =>
        new() { Name = name, Format = format, Family = family, Implemented = true };

    [Fact]
    public void TheSameScreenOnTwoEnginesIsTwoNames()
    {
        var words = Words(
            Capture("x11grab", Engine.Ffmpeg),
            Capture("ximagesrc", Engine.Gstreamer));

        var ffmpeg = words.Name("publish.capture", "x11grab");
        var gstreamer = words.Name("publish.capture", "ximagesrc");

        Assert.NotEqual(ffmpeg, gstreamer);
        Assert.Contains("ffmpeg", ffmpeg);
        Assert.Contains("GStreamer", gstreamer);
    }

    /// <summary>
    /// Engine is what a refusal elsewhere tells the reader to change,
    /// so it is on every entry rather than only on the ones that would otherwise collide.
    /// </summary>
    [Fact]
    public void ACaptureBackendNamesItsEngineEvenWhereNothingSharesItsSource()
    {
        var words = Words(Capture("kmsgrab", Engine.Ffmpeg));

        Assert.Contains("ffmpeg", words.Name("publish.capture", "kmsgrab"));
    }

    /// <summary>
    /// CPU encoders are what the reader chooses between once a format is picked, so each names its own project:
    /// three of them code AV1, and "CPU" alone offers the same entry three times.
    /// </summary>
    [Fact]
    public void TheCpuEncodersOfOneFormatAreToldApart()
    {
        var names = new[] { "libaom", "svt-av1", "rav1e" }
            .Select(id => Vocabulary.Empty.Name("publish.encoder", id))
            .ToList();

        Assert.Equal(names.Count, names.Distinct().Count());
        Assert.All(names, name => Assert.Contains("CPU", name));
    }

    /// <summary>
    /// A hardware family is one encoder,
    /// so the entry is named for the thing a reader either has or does not, never for the runtime driving it.
    /// </summary>
    [Fact]
    public void AHardwareEncoderIsNamedByTheHardware()
    {
        Assert.Contains("NVIDIA", Vocabulary.Empty.Name("publish.encoder", "nvenc"));
    }

    /// <summary>
    /// Catalog arrives after the first form does, so an entry is nameable without it.
    /// Where nothing names a value the backend's own is left, which a reader can pick, search for and report.
    /// </summary>
    [Fact]
    public void WithoutTheCatalogAnEntryIsNamedByWhatTheBackendCalledIt()
    {
        Assert.Equal("newcodec", Vocabulary.Empty.Name("publish.format", "newcodec"));
        Assert.Equal("X11 screen", Vocabulary.Empty.Name("publish.capture", "x11grab"));
    }

    /// <summary>Quality group as the backend resolves it, with the ceiling control in the state it stated.</summary>
    private static FieldGroup QualityGroup(bool ceilingOffered, long ceilingMbps)
    {
        var group = new FieldGroup { Key = "quality" };
        group.Fields.Add(new Field
        {
            Key = "publish.maxrate_mbps",
            Enabled = ceilingOffered,
            Visible = true,
            Value = new FieldValue { Number = ceilingMbps },
        });
        return group;
    }

    private static Settings ConstantQuality(int cq) => new()
    {
        Publish = new PublishSettings { Format = "h264", Encoder = "x264", Mode = "crf", Cq = cq },
    };

    /// <summary>
    /// A quality target names no rate a reader can hold against their connection,
    /// so the line carries the rate the encode is held to where there is one.
    /// </summary>
    [Fact]
    public void AQualityTargetCarriesTheRateItIsHeldTo()
    {
        var words = Words(Codec("libx264", "h264", "software"));

        var summary = words.Shorthand(QualityGroup(ceilingOffered: true, 45), ConstantQuality(18));

        Assert.Contains("quality 18", summary);
        Assert.Contains("45 Mbit/s", summary);
    }

    /// <summary>
    /// An encoder that bounds nothing greys the control, and the line then names no bound:
    /// the stored figure is one the encode is not holding, and printing it describes a stream nobody is sending.
    /// </summary>
    [Fact]
    public void AnUnboundedQualityTargetNamesNoRate()
    {
        var words = Words(Codec("libx264", "h264", "software"));

        var summary = words.Shorthand(QualityGroup(ceilingOffered: false, 45), ConstantQuality(18));

        Assert.Contains("quality 18", summary);
        Assert.DoesNotContain("45", summary);
    }
}
