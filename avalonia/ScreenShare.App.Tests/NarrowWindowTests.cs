using Avalonia.Controls;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Insights.ViewModel;
using ScreenShare.App.Features.Shell.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Each destination against the width the window handed it.
/// A tiling desktop hands a window whatever the layout leaves, a quarter of the screen among them,
/// so the arrangement is read off that width on every pass rather than off the size the app opened at.
///
/// Defect locked out: a screen whose controls stand outside the window they were drawn in.
/// </summary>
public sealed class NarrowWindowTests
{
    private static readonly Action<Action> Inline = action => action();

    /// <summary>Quarter of a laptop screen, where no surface fits two columns.</summary>
    private const double Quarter = 640;

    private static InsightsViewModel Insights(IBackend backend)
    {
        var session = new Session(backend, Inline);
        return new InsightsViewModel(backend, new FormSession(backend, session, Inline), session, Inline);
    }

    [Fact]
    public void ANarrowWindowTakesTheSetupRailOffTheForm()
    {
        var setup = Flows.Setup(new SeededBackend("linux"));

        setup.SetWindowWidth(Quarter);

        Assert.False(setup.RailColumn.IsBeside);
        Assert.False(setup.RailColumn.IsShowing);
        Assert.True(setup.RailColumn.ShowsToggle);
    }

    [Fact]
    public void AWideWindowKeepsTheSetupRailBesideTheForm()
    {
        var setup = Flows.Setup(new SeededBackend("linux"));

        setup.SetWindowWidth(WindowSize.Opening);

        Assert.True(setup.RailColumn.IsBeside);
        Assert.True(setup.RailColumn.IsShowing);
    }

    [Fact]
    public void ANarrowWindowTakesTheInsightsCardsOffTheFigures()
    {
        var insights = Insights(new SeededBackend("linux"));

        insights.SetWindowWidth(Quarter);

        Assert.False(insights.CardsColumn.IsBeside);
        Assert.True(insights.CardsColumn.ShowsToggle);
    }

    /// <summary>
    /// The live actions take the row on a narrow window, so the figures beside them keep a row of their own.
    /// A header deeper than the screen it heads is what this prevents.
    /// </summary>
    [Fact]
    public void ANarrowWindowPutsTheLiveActionsUnderTheFigures()
    {
        var insights = Insights(new SeededBackend("linux"));

        insights.SetWindowWidth(WindowSize.Opening);
        Assert.Equal(Dock.Right, insights.ActionsDock);

        insights.SetWindowWidth(Quarter);
        Assert.Equal(Dock.Bottom, insights.ActionsDock);
    }

    [Fact]
    public void ANarrowWindowTakesTheWatchPanelOverTheGrid()
    {
        var backend = new SeededBackend("linux");
        var viewer = Flows.Viewer(backend, new Session(backend, Inline));

        viewer.SetWindowWidth(Quarter);
        viewer.WatchColumn.Toggle.Execute(null);

        Assert.True(viewer.WatchColumn.IsShowing);
        Assert.False(viewer.WatchColumn.IsBeside);
    }

    /// <summary>
    /// The rail keeps the names a window has the width for.
    /// Below that width the entries keep their dots and drop their names, which leaves the grid a tile's width.
    /// </summary>
    [Fact]
    public void AWindowTooNarrowForTheRailDrawsItAsDots()
    {
        var backend = new SeededBackend("linux");
        var viewer = Flows.Viewer(backend, new Session(backend, Inline));

        viewer.SetWindowWidth(WindowSize.Floor);

        Assert.False(viewer.ShowsRailNames);
        Assert.False(viewer.ShowsRailToggle);
        Assert.Equal(88, viewer.RailWidth);
    }

    /// <summary>The reader's own collapse survives a window wide enough for the names.</summary>
    [Fact]
    public void ACollapsedRailStaysCollapsedOnAWideWindow()
    {
        var backend = new SeededBackend("linux");
        var viewer = Flows.Viewer(backend, new Session(backend, Inline));

        viewer.SetWindowWidth(WindowSize.Opening);
        viewer.ToggleRail.Execute(null);

        Assert.False(viewer.ShowsRailNames);
        Assert.True(viewer.ShowsRailToggle);

        viewer.SetWindowWidth(WindowSize.Opening);

        Assert.False(viewer.ShowsRailNames);
    }

    /// <summary>A width the window states twice changes nothing the second time.</summary>
    [Fact]
    public void TheSameWidthTwiceIsOnePass()
    {
        var setup = Flows.Setup(new SeededBackend("linux"));

        setup.SetWindowWidth(Quarter);
        setup.RailColumn.Toggle.Execute(null);
        setup.SetWindowWidth(Quarter);

        Assert.True(setup.RailColumn.IsShowing);
    }
}
