using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What an entry is called, where its own value does not say enough.
///
/// The defect these lock out is one name printed twice. The backend identifies an option by
/// its value alone and leaves the naming here, and two families of entries share everything
/// this side used to name them by: the same screen is read by both publish engines, and one
/// family of encoders produces one format from several encoders. Both rendered as two or
/// three identical rows, one of them greyed, with nothing on the row to say which was which.
///
/// The separating fact is a column of the catalog in both cases, so the rule is the same:
/// name against the table rather than off the row.
/// </summary>
public sealed class VocabularyTests
{
    /// <summary>
    /// The catalog rows a name is composed from. Only the columns the naming reads are set,
    /// which is what keeps a test about naming from asserting anything about availability.
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
    /// The engine is what a refusal elsewhere tells the reader to change, so it is on every
    /// entry rather than only on the ones that would otherwise collide: a list where some
    /// rows name their engine and some do not answers "which of these runs ffmpeg" with a
    /// shrug.
    /// </summary>
    [Fact]
    public void ACaptureBackendNamesItsEngineEvenWhereNothingSharesItsSource()
    {
        var words = Words(Capture("kmsgrab", Engine.Ffmpeg));

        Assert.Contains("ffmpeg", words.Name("publish.capture", "kmsgrab"));
    }

    [Fact]
    public void EncodersOfOneFormatAndFamilyAreToldApartByTheirOwnNames()
    {
        var words = Words(
            Codec("libaom-av1", "av1", "software"),
            Codec("libsvtav1", "av1", "software"),
            Codec("librav1e", "av1", "software"));

        var names = new[] { "libaom-av1", "libsvtav1", "librav1e" }
            .Select(name => words.Name("publish.codec", name))
            .ToList();

        Assert.Equal(names.Count, names.Distinct().Count());
        Assert.All(names, name => Assert.Contains("AV1", name));
    }

    /// <summary>
    /// The format and the family are the two questions a codec answers, and where they
    /// identify the row on their own the encoder's name stays out of it.
    /// </summary>
    [Fact]
    public void AnEncoderNothingSharesAFormatAndFamilyWithKeepsTheShortName()
    {
        var words = Words(
            Codec("hevc_nvenc", "hevc", "nvenc"),
            Codec("h264_nvenc", "h264", "nvenc"));

        Assert.DoesNotContain("hevc_nvenc", words.Name("publish.codec", "hevc_nvenc"));
    }

    /// <summary>
    /// The catalog arrives after the first form does, so every entry has to be nameable
    /// without it. The answer is then what the backend called it, which is a value the
    /// reader can still pick, search for and report.
    /// </summary>
    [Fact]
    public void WithoutTheCatalogAnEntryIsNamedByWhatTheBackendCalledIt()
    {
        Assert.Equal("libsvtav1", Vocabulary.Empty.Name("publish.codec", "libsvtav1"));
        Assert.Equal("X11 screen", Vocabulary.Empty.Name("publish.capture", "x11grab"));
    }
}
