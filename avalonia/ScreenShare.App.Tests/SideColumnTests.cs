using ScreenShare.App.Features.Shell.Model;
using ScreenShare.App.Features.Shell.ViewModel;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// What a window too narrow for two columns does with the side one.
/// The window is the one owner of its width, and every surface reads the same table against it
/// (<c>docs/design-language.md</c>, "Narrow windows").
///
/// Defect locked out: a column that keeps its width on a window that has none to spare,
/// leaving the body a strip the form cannot be read in.
/// </summary>
public sealed class SideColumnTests
{
    /// <summary>Width both columns fit in, whatever the row states.</summary>
    private const double Wide = 2000;

    /// <summary>Quarter of a laptop screen, where every row's threshold is above the window.</summary>
    private const double Quarter = 500;

    private static SideColumnViewModel Rail()
        => new(SideColumns.SetupRail, "Show what these settings cost.", "Hide what these settings cost.");

    private static SideColumnViewModel Watch()
        => new(SideColumns.ViewerWatch, "Change how streams come back.", "Close the watch settings.");

    /// <summary>A window nothing has measured yet withholds nothing: every column draws where its surface states.</summary>
    [Fact]
    public void AnUnmeasuredWindowDrawsEveryColumnBesideTheBody()
    {
        Assert.True(Rail().IsBeside);
        Assert.True(Rail().IsShowing);
    }

    [Fact]
    public void AWideWindowDrawsTheColumnBesideTheBody()
    {
        var rail = Rail();
        rail.SetWindowWidth(Wide);

        Assert.True(rail.IsBeside);
        Assert.True(rail.IsShowing);
    }

    /// <summary>Nothing to press where the column already stands beside the body.</summary>
    [Fact]
    public void AColumnDrawnBesideTheBodyOffersNoToggle()
    {
        var rail = Rail();
        rail.SetWindowWidth(Wide);

        Assert.False(rail.ShowsToggle);
    }

    [Fact]
    public void ANarrowWindowTakesTheColumnOffTheBody()
    {
        var rail = Rail();
        rail.SetWindowWidth(Quarter);

        Assert.False(rail.IsBeside);
        Assert.False(rail.IsShowing);
        Assert.True(rail.ShowsToggle);
    }

    [Fact]
    public void TheToggleOpensThePanelOverTheBody()
    {
        var rail = Rail();
        rail.SetWindowWidth(Quarter);

        rail.Toggle.Execute(null);

        Assert.True(rail.IsShowing);
        Assert.False(rail.IsBeside);
    }

    [Fact]
    public void TheToggleClosesThePanelAgain()
    {
        var rail = Rail();
        rail.SetWindowWidth(Quarter);

        rail.Toggle.Execute(null);
        rail.Toggle.Execute(null);

        Assert.False(rail.IsShowing);
    }

    /// <summary>A window widened while the panel is open draws it as the column it is on a wide window.</summary>
    [Fact]
    public void AWidenedWindowPutsThePanelBackBesideTheBody()
    {
        var rail = Rail();
        rail.SetWindowWidth(Quarter);
        rail.Toggle.Execute(null);

        rail.SetWindowWidth(Wide);

        Assert.True(rail.IsBeside);
        Assert.True(rail.IsShowing);
        Assert.False(rail.ShowsToggle);
    }

    /// <summary>A column the reader asks for stays closed on a window with the room for it.</summary>
    [Fact]
    public void AColumnOpenedOnRequestDrawsOnlyOnceAsked()
    {
        var watch = Watch();
        watch.SetWindowWidth(Wide);

        Assert.False(watch.IsShowing);
        Assert.True(watch.ShowsToggle);

        watch.Toggle.Execute(null);

        Assert.True(watch.IsShowing);
        Assert.True(watch.IsBeside);
    }

    /// <summary>Closing names the state, so a second close is the state the caller asked for.</summary>
    [Fact]
    public void ClosingTwiceLeavesThePanelClosed()
    {
        var watch = Watch();
        watch.SetWindowWidth(Wide);
        watch.Toggle.Execute(null);

        watch.Close();
        watch.Close();

        Assert.False(watch.IsShowing);
    }

    /// <summary>The tip names the state the press leads to.</summary>
    [Fact]
    public void TheToggleTipNamesWhatThePressDoes()
    {
        var watch = Watch();
        watch.SetWindowWidth(Wide);

        Assert.Equal("Change how streams come back.", watch.ToggleTip);

        watch.Toggle.Execute(null);

        Assert.Equal("Close the watch settings.", watch.ToggleTip);
    }

    /// <summary>The threshold is the row's own two figures, and the window carrying both draws both.</summary>
    [Fact]
    public void TheThresholdIsTheColumnPlusTheBodyItNeeds()
    {
        var row = SideColumns.SetupRail;

        Assert.Equal(row.Width + row.Body, row.Beside);
        Assert.True(row.FitsBeside(row.Beside));
        Assert.False(row.FitsBeside(row.Beside - 1));
    }

    /// <summary>
    /// The viewer's watch panel measures against the stream rail as well as the grid,
    /// two side columns on one window otherwise leaving the tiles a strip between them.
    /// </summary>
    [Fact]
    public void TheWatchPanelNeedsTheRailsWidthOnTopOfTheGrids()
    {
        Assert.True(SideColumns.ViewerWatch.Beside > SideColumns.SetupRail.Beside);
    }

    /// <summary>Every row draws beside the body on the window the app opens at.</summary>
    [Fact]
    public void TheOpeningWindowCarriesEveryColumnBesideItsBody()
    {
        Assert.True(SideColumns.SetupRail.FitsBeside(WindowSize.Opening));
        Assert.True(SideColumns.BroadcastCards.FitsBeside(WindowSize.Opening));
        Assert.True(SideColumns.ViewerWatch.FitsBeside(WindowSize.Opening));
        Assert.True(SideColumns.ViewerRail.FitsBeside(WindowSize.Opening));
    }

    /// <summary>The floor is a window every surface draws one column in, so no row fits beside the body there.</summary>
    [Fact]
    public void TheFloorIsNarrowerThanEveryRowsThreshold()
    {
        Assert.False(SideColumns.SetupRail.FitsBeside(WindowSize.Floor));
        Assert.False(SideColumns.BroadcastCards.FitsBeside(WindowSize.Floor));
        Assert.False(SideColumns.ViewerWatch.FitsBeside(WindowSize.Floor));
    }
}
