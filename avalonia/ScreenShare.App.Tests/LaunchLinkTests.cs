using ScreenShare.App.Features.Shell.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What a window makes of the arguments a desktop starts it with.
/// A launcher decides how many arguments ride in front of the link, so the scheme is what finds it.
/// </summary>
public class LaunchLinkTests
{
    [Fact]
    public void AWindowStartedWithNoArgumentsFollowsNothing()
    {
        Assert.Equal("", LaunchLink.In(null));
        Assert.Equal("", LaunchLink.In([]));
    }

    [Fact]
    public void ALinkIsFoundWhereverItSits()
    {
        var link = "mirrorme://watch/abc123/bob/monitor-0";

        Assert.Equal(link, LaunchLink.In([link]));
        Assert.Equal(link, LaunchLink.In(["--some-flag", link]));
    }

    [Fact]
    public void ArgumentsThatAreNotLinksAreLeftAlone()
    {
        Assert.Equal("", LaunchLink.In(["--dev", "/home/someone/a file.txt"]));
        Assert.Equal("", LaunchLink.In(["https://example.test/watch/abc123/bob"]));
    }

    /// <summary>A desktop is free to hand the scheme back in any case it likes.</summary>
    [Fact]
    public void ASchemeIsReadInAnyCase()
    {
        Assert.Equal("MirrorMe://watch/abc123/bob", LaunchLink.In(["MirrorMe://watch/abc123/bob"]));
    }
}
