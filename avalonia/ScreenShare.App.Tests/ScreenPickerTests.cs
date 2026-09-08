using ScreenShare.Api.V1;
using ScreenShare.App.Backend;
using ScreenShare.App.Features.Setup.Model;
using ScreenShare.App.Features.Setup.ViewModel;
using ScreenShare.App.Features.Viewer.Tile.Model;
using Xunit;

namespace ScreenShare.App.Tests;

/// <summary>
/// Grid is a second path to <c>publish.monitor</c>, so picking writes what the list writes and nothing else.
/// Each picture costs a backend capture,
/// so it is asked for once per screen, while the share question is on screen with the window in front.
/// </summary>
public sealed class ScreenPickerTests
{
    /// <summary>One view of the share question, standing in for the control that reports it is on screen.</summary>
    private static readonly object Watcher = new();

    /// <summary>
    /// Reads every state once and stops before the reconnect delay, so nothing is left dialling behind the assertions.
    /// </summary>
    private static void Load(Session session)
    {
        session.Start();
        session.Stop();
    }

    /// <summary>Flow on the share step with the window in front, the only state the grid draws pictures in.</summary>
    private static SetupViewModel OnShareStep(SeededBackend backend, bool showing = true)
    {
        var session = new Session(backend, action => action());
        Load(session);

        var flow = Flows.Setup(backend, session);
        flow.CurrentStep = ShareLayout.GroupKey;
        flow.Share.SetShowing(Watcher, showing);
        return flow;
    }

    [Fact]
    public void TheGridOffersEveryEnumeratedScreenAndMarksTheOneTheDraftNames()
    {
        var flow = OnShareStep(new SeededBackend("linux") { AsksWhatToShare = true });

        Assert.True(flow.Share.Screens.IsVisible);
        Assert.Equal([0, 1], flow.Share.Screens.Screens.Select(screen => screen.Monitor));
        Assert.All(flow.Share.Screens.Screens, screen => Assert.True(screen.IsEnabled));

        // Label is composed here from the catalog row, the backend sending no name for a screen.
        Assert.Contains("2560", flow.Share.Screens.Screens[0].Label);
        Assert.Contains("144", flow.Share.Screens.Screens[0].Label);

        Assert.True(flow.Share.Screens.Screens[0].IsSelected);
        Assert.False(flow.Share.Screens.Screens[1].IsSelected);
    }

    /// <summary>Mark follows the draft rather than the press, the form answering with the value being what moves it.</summary>
    [Fact]
    public void PickingAScreenWritesTheSettingAndMovesTheMark()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var flow = OnShareStep(backend);

        flow.Share.Screens.Screens[1].Select.Execute(null);

        Assert.False(flow.Share.Screens.Screens[0].IsSelected);
        Assert.True(flow.Share.Screens.Screens[1].IsSelected);

        // Grid and list are one value read twice, not two controls kept in step.
        var list = flow.Share.Group.Fields.Single(field => field.Key == ShareLayout.MonitorKey);
        Assert.Equal("1", list.Options.Single(option => option.IsSelected).Value);
    }

    /// <summary>
    /// Flow re-renders on every keystroke,
    /// so a converge asking again on each pass costs a call per screen per keystroke.
    /// </summary>
    [Fact]
    public void EveryScreenIsAskedForOnceWhileTheGridIsDrawn()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var flow = OnShareStep(backend);

        Assert.Equal([0, 1], backend.Previewed);

        flow.Share.Apply();
        flow.Share.Apply();

        Assert.Equal([0, 1], backend.PreviewStarts);
    }

    /// <summary>Wizard renders every step's model on every pass, so being rendered is not what opens a capture.</summary>
    [Fact]
    public void NoScreenIsReadFromAnotherStep()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var session = new Session(backend, action => action());
        Load(session);

        var flow = Flows.Setup(backend, session);
        flow.CurrentStep = "encoder";
        flow.Share.SetShowing(Watcher, true);

        Assert.False(flow.Share.Screens.IsVisible);
        Assert.Empty(flow.Share.Screens.Screens);
        Assert.Empty(backend.Previewed);
        Assert.Empty(backend.PreviewStarts);
    }

    /// <summary>
    /// A picture nobody is looking at goes on grabbing every screen five times a second
    /// for as long as the app is open.
    /// </summary>
    [Fact]
    public void LeavingTheStepStopsReadingTheScreens()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var flow = OnShareStep(backend);

        Assert.Equal([0, 1], backend.Previewed);

        flow.CurrentStep = "encoder";

        Assert.Empty(backend.Previewed);
        Assert.False(flow.Share.Screens.IsVisible);
    }

    /// <summary>Step and window are separate facts, so either one turning false is enough.</summary>
    [Fact]
    public void AWindowThatWentBehindStopsReadingTheScreens()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        var flow = OnShareStep(backend);

        flow.Share.SetShowing(Watcher, false);

        Assert.Empty(backend.Previewed);
        Assert.False(flow.Share.Screens.IsVisible);
    }

    /// <summary>
    /// Previews outlive the window that asked for them, as decodes do, so nothing else closes one.
    /// A connecting shell learns of the leftover by reading the state,
    /// so that read is on the contract rather than left to the event stream.
    /// </summary>
    [Fact]
    public void AScreenLeftBeingReadByAnEarlierShellIsClosed()
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        backend.Previewed.Add(1);

        var session = new Session(backend, action => action());
        Load(session);

        Assert.Equal([1], session.PreviewedMonitors.Select(previewed => previewed.Monitor));

        // Building the flow renders once with nothing being looked at, and that pass closes the leftover.
        Flows.Setup(backend, session);

        Assert.Empty(backend.Previewed);
    }

    /// <summary>Sentence is the backend's own, and nothing is opened where every capture is refused.</summary>
    [Fact]
    public void AMachineThatCannotShowOneScreenSaysSoAndOpensNothing()
    {
        var backend = new SeededBackend("linux")
        {
            AsksWhatToShare = true,
            NoMonitorPreview = new Text
            {
                Code = TextCode.NoMonitorPreview,
                Args =
                {
                    new TextArg { Name = TextArgName.Os, Id = "linux" },
                    new TextArg { Name = TextArgName.Display, Id = "wayland" },
                },
            },
        };

        var flow = OnShareStep(backend);

        Assert.False(flow.Share.Screens.IsVisible);
        Assert.Empty(backend.Previewed);
        Assert.True(flow.Share.Screens.HasNotice);
        Assert.Contains("Wayland", flow.Share.Screens.Notice);
    }

    /// <summary>
    /// A subscription naming a screen nothing is reading is refused once and never retried,
    /// so a tile made while the start is in flight sits dark for as long as the reader stays on the step.
    /// </summary>
    [Fact]
    public void AScreenThatIsNotBeingReadYetCarriesNoTile()
    {
        var flow = OnShareStep(new SeededBackend("linux") { AsksWhatToShare = true });

        Assert.All(flow.Share.Screens.Screens, screen => Assert.Null(screen.Tile));
        Assert.All(flow.Share.Screens.Screens, screen => Assert.True(screen.HasPlaceholder));
    }

    [Fact]
    public void AScreenTheBackendIsReadingCarriesATileNamingIt()
    {
        var flow = OnShareStep(Reading(0, 1));
        var tile = flow.Share.Screens.Screens[1].Tile;

        Assert.NotNull(tile);
        Assert.Equal(TileSourceKind.MonitorPreview, tile.Source.Kind);
        Assert.Equal(1, tile.Source.Monitor);
        Assert.False(flow.Share.Screens.Screens[1].HasPlaceholder);
    }

    /// <summary>Form resolves on every keystroke, so a pass rebuilding the tiles re-subscribes that often.</summary>
    [Fact]
    public void ARenderPassKeepsTheTilesItAlreadyHas()
    {
        var flow = OnShareStep(Reading(0, 1));
        var before = flow.Share.Screens.Screens.Select(screen => screen.Tile).ToList();

        Assert.All(before, Assert.NotNull);

        flow.Share.Apply();

        Assert.Equal(before, flow.Share.Screens.Screens.Select(screen => screen.Tile));
    }

    /// <summary>Fixture already reading these screens, which is what puts a tile on a row.</summary>
    private static SeededBackend Reading(params int[] monitors)
    {
        var backend = new SeededBackend("linux") { AsksWhatToShare = true };
        backend.Previewed.AddRange(monitors);
        return backend;
    }
}
