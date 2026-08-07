using ScreenShare.App.Relay;
using Xunit;

namespace ScreenShare.App.Tests;

public sealed class TrackFormatsTests
{
    [Theory]
    [InlineData("H264", "h264")]
    [InlineData("AVC", "h264")]
    [InlineData("H265", "hevc")]
    [InlineData("HEVC", "hevc")]
    [InlineData("VP8", "vp8")]
    [InlineData("VP9", "vp9")]
    [InlineData("AV1", "av1")]
    public void BothSpellingsOfAFormatResolveToTheSameKey(string track, string format)
        => Assert.Equal(format, TrackFormats.Of([track]));

    [Fact]
    public void TheTrackNameIsReadWithoutRegardToCase()
        => Assert.Equal("h264", TrackFormats.Of(["h264"]));

    [Fact]
    public void TheVideoTrackWinsOverTheAudioOneWhateverTheOrder()
        => Assert.Equal("av1", TrackFormats.Of(["Opus", "AV1"]));

    [Fact]
    public void APathNamingNoKnownFormatResolvesToNone()
        => Assert.Equal("", TrackFormats.Of(["Opus", "MPEG-4 Audio"]));

    [Fact]
    public void APathWithNoTracksResolvesToNone()
        => Assert.Equal("", TrackFormats.Of([]));
}
