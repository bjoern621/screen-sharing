using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.StatusBar.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// The build the window is running, in the one band every platform draws.
///
/// The title bar is the app's on Windows and macOS alone.
/// On Linux the frame is the desktop's and nothing of the app is in it (<c>avalonia/README.md</c>),
/// so a build stated there reaches two platforms out of three.
///
/// Defect locked out: a bug report naming no build, and a tester who cannot tell which one they run.
/// </summary>
public sealed class StatusBarVersionTests
{
    private const string Build = "0.4.0";

    private static StatusBarViewModel Band(Destination destination, string version = Build)
    {
        var band = new StatusBarViewModel();
        band.Show(destination, "", [], "", version);
        return band;
    }

    /// <summary>
    /// The build is a fact about the app rather than a figure of one destination,
    /// so the band states it wherever the reader is standing.
    /// The figures beside it speak for the viewer alone and stay that way.
    /// </summary>
    [Fact]
    public void TheBandStatesTheBuildInEveryDestination()
    {
        foreach (var destination in Enum.GetValues<Destination>())
        {
            var band = Band(destination);

            Assert.True(band.ShowsVersion);
            Assert.Contains(Build, band.Version);
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

        Assert.False(band.ShowsVersion);
        Assert.Equal("", band.Version);
    }

    /// <summary>
    /// A version reads as one at a glance, beside figures that are measurements.
    /// </summary>
    [Fact]
    public void TheBuildIsMarkedAsAVersion()
    {
        Assert.Equal("v" + Build, Band(Destination.Setup).Version);
    }

    /// <summary>
    /// The band renders every output on every pass, so a build survives a destination it says
    /// nothing else in (<c>docs/development-principles.md</c>, "One render function").
    /// </summary>
    [Fact]
    public void TheBuildSurvivesAPassThatStatesNoFigures()
    {
        var band = new StatusBarViewModel();
        band.Show(Destination.Viewer, "2 of 3 on screen", ["12 Mbit/s"], "hint", Build);
        band.Show(Destination.Setup, "", [], "", Build);

        Assert.False(band.ShowsMetrics);
        Assert.True(band.ShowsVersion);
        Assert.Equal("v" + Build, band.Version);
    }
}
