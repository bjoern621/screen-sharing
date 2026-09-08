using ScreenShare.Api.V1;
using ScreenShare.App.Copy;
using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.StatusBar.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The band's one release control: the build the window runs, and what a check found standing in its place.
///
/// The title bar is the app's on Windows and macOS alone.
/// On Linux the frame is the desktop's and nothing of the app is in it (<c>avalonia/README.md</c>),
/// so a build stated there reaches two platforms out of three.
///
/// Defects locked out: a bug report naming no build, a tester who cannot tell which one they run,
/// and a band drawing the build beside a second line about the same release.
/// </summary>
public sealed class StatusBarVersionTests
{
    private const string Build = "0.4.0";

    private static StatusBarViewModel Band(
        Destination destination, string version = Build, UpdateState? update = null)
    {
        var backend = new SeededBackend("linux");
        if (update is not null)
        {
            backend.Update = update;
        }

        var band = new StatusBarViewModel(Flows.Updates(backend));
        band.Show(destination, [], "", version);
        return band;
    }

    /// <summary>
    /// The build is a fact about the app rather than a figure of one destination,
    /// so the band states it wherever the reader is standing.
    /// </summary>
    [Fact]
    public void TheBandStatesTheBuildInEveryDestination()
    {
        foreach (var destination in Enum.GetValues<Destination>())
        {
            var band = Band(destination);

            Assert.True(band.ShowsRelease);
            Assert.Contains(Build, band.Release);
        }
    }

    /// <summary>
    /// A build nothing has answered yet is stated as nothing rather than as an empty version,
    /// the handshake being what carries it and a window opening before that.
    /// </summary>
    [Fact]
    public void ABuildNobodyAnsweredIsNotDrawn()
    {
        var band = Band(Destination.Viewer, version: "");

        Assert.False(band.ShowsRelease);
        Assert.Equal("", band.Release);
    }

    /// <summary>
    /// A version reads as one at a glance, beside figures that are measurements.
    /// </summary>
    [Fact]
    public void TheBuildIsMarkedAsAVersion()
    {
        Assert.Equal("v" + Build, Band(Destination.Setup).Release);
    }

    /// <summary>
    /// The band renders every output on every pass, so a build survives a destination it says
    /// nothing else in (<c>docs/development-principles.md</c>, "One render function").
    /// </summary>
    [Fact]
    public void TheBuildSurvivesAPassThatStatesNoFigures()
    {
        var band = Band(Destination.Viewer);
        band.Show(Destination.Viewer, ["12 Mbit/s"], "hint", Build);
        band.Show(Destination.Setup, [], "", Build);

        Assert.False(band.ShowsMetrics);
        Assert.True(band.ShowsRelease);
        Assert.Equal("v" + Build, band.Release);
    }

    /// <summary>
    /// A published release takes the build's place rather than standing beside it:
    /// one control in the band says one thing about the release.
    /// The build stays a hover away, that being what a bug report names.
    /// </summary>
    [Fact]
    public void AFoundReleaseTakesTheBuildsPlace()
    {
        var band = Band(Destination.Setup, update: new UpdateState
        {
            Stage = UpdateStage.Available,
            Running = Build,
            Latest = "v0.5.0",
        });

        Assert.True(band.ShowsRelease);
        Assert.Equal(Updates.Available, band.Release);
        Assert.Contains("v" + Build, band.ReleaseHint);
    }

    /// <summary>
    /// A check under way says so in the same control, so a press has an answer while it waits.
    /// </summary>
    [Fact]
    public void ACheckUnderWayTakesTheBuildsPlace()
    {
        var band = Band(Destination.Setup, update: new UpdateState
        {
            Stage = UpdateStage.Checking,
            Running = Build,
        });

        Assert.True(band.ShowsRelease);
        Assert.Equal(Updates.Checking, band.Release);
        Assert.True(band.Updates.IsChecking);
    }

    /// <summary>
    /// A build at the published release states that and nothing else.
    /// </summary>
    [Fact]
    public void ABuildAtThePublishedReleaseTakesTheBuildsPlace()
    {
        var band = Band(Destination.Setup, update: new UpdateState
        {
            Stage = UpdateStage.Current,
            Running = Build,
            Latest = "v" + Build,
        });

        Assert.Equal(Updates.Current, band.Release);
    }

    /// <summary>
    /// A failing check takes the slot as selectable text, that being the string a bug report carries
    /// (<c>CLAUDE.md</c>, "Every error message is selectable and copyable").
    /// The pressable control is gone while it stands, so the band still draws one thing.
    /// </summary>
    [Fact]
    public void AFailingCheckLeavesNoPressableControl()
    {
        var band = Band(Destination.Setup, update: new UpdateState
        {
            Stage = UpdateStage.Failed,
            Running = Build,
            Failure = new Text { Code = TextCode.UpdateServiceUnreadable },
        });

        Assert.False(band.ShowsRelease);
        Assert.True(band.Updates.ShowsPlainLine);
        Assert.True(band.Updates.IsFailure);
    }
}
